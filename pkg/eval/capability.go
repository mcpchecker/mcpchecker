package eval

// CapabilityInventory lists the visible tools exposed by each server.
// A server is present when it has an entry in the inventory, even if it has no tools.
type CapabilityInventory map[string][]string

// CapabilityResolution contains the assertions that can be evaluated and any
// assertions that could not be resolved against the live capability inventory.
type CapabilityResolution struct {
	Assertions        TaskAssertions
	SkippedAssertions []AssertionPresenceResult
	PresenceFailures  []AssertionPresenceResult
}

// ResolveCapabilities filters assertions against currently visible server and
// tool capabilities without modifying the input assertions or inventory.
func ResolveCapabilities(assertions TaskAssertions, inventory CapabilityInventory) CapabilityResolution {
	result := CapabilityResolution{Assertions: cloneTaskAssertions(assertions)}

	addMissing := func(record AssertionPresenceResult) {
		if assertions.RequirePresent {
			result.PresenceFailures = append(result.PresenceFailures, record)
		} else {
			result.SkippedAssertions = append(result.SkippedAssertions, record)
		}
	}

	filterTools := func(assertionType string, configured []ToolAssertion) []ToolAssertion {
		filtered := make([]ToolAssertion, 0, len(configured))
		for _, assertion := range configured {
			if reason, target := toolPresence(assertion, inventory); reason != "" {
				addMissing(AssertionPresenceResult{
					Type: assertionType, Server: assertion.Server, Target: target, Reason: reason,
				})
				continue
			}
			filtered = append(filtered, assertion)
		}
		return filtered
	}

	result.Assertions.ToolsUsed = filterTools(assertionTypeToolsUsed, assertions.ToolsUsed)
	result.Assertions.ToolsNotUsed = filterTools(assertionTypeToolsNotUsed, assertions.ToolsNotUsed)
	result.Assertions.Apps = make([]AppAssertion, 0, len(assertions.Apps))
	for _, app := range assertions.Apps {
		if reason, target := toolPresence(ToolAssertion{Server: app.Server, Tool: app.Tool}, inventory); reason != "" {
			addMissing(AssertionPresenceResult{Type: assertionTypeApps, Server: app.Server, Target: target, Reason: reason})
			continue
		}
		result.Assertions.Apps = append(result.Assertions.Apps, app)
	}

	result.Assertions.RequireAny = make([]ToolAssertion, 0, len(assertions.RequireAny))
	var firstMissing *AssertionPresenceResult
	for _, candidate := range assertions.RequireAny {
		if reason, target := toolPresence(candidate, inventory); reason != "" {
			if firstMissing == nil {
				firstMissing = &AssertionPresenceResult{
					Type: assertionTypeRequireAny, Server: candidate.Server,
					Target: target, Reason: reason,
				}
			}
			continue
		}
		result.Assertions.RequireAny = append(result.Assertions.RequireAny, candidate)
	}
	if len(assertions.RequireAny) > 0 && len(result.Assertions.RequireAny) == 0 && firstMissing != nil {
		addMissing(*firstMissing)
	}

	result.Assertions.ResourcesRead = filterResources(assertions.ResourcesRead, inventory, assertionTypeResourcesRead, addMissing)
	result.Assertions.ResourcesNotRead = filterResources(assertions.ResourcesNotRead, inventory, assertionTypeResourcesNotRead, addMissing)
	result.Assertions.PromptsUsed = filterPrompts(assertions.PromptsUsed, inventory, assertionTypePromptsUsed, addMissing)
	result.Assertions.PromptsNotUsed = filterPrompts(assertions.PromptsNotUsed, inventory, assertionTypePromptsNotUsed, addMissing)

	result.Assertions.CallOrder = make([]CallOrderAssertion, 0, len(assertions.CallOrder))
	for _, entry := range assertions.CallOrder {
		missing := AssertionPresenceResult{Type: assertionTypeCallOrder, Server: entry.Server}
		if entry.Type == callTypeTool {
			missing.Target = entry.Name
		}
		if _, serverPresent := inventory[entry.Server]; !serverPresent {
			missing.Reason = "server not present"
		} else if entry.Type == callTypeTool && !hasVisibleTool(entry.Name, inventory[entry.Server]) {
			missing.Reason = "tool not present"
		}
		if missing.Reason != "" {
			addMissing(missing)
			// Call order is one assertion: discard the full sequence on any missing entry.
			result.Assertions.CallOrder = nil
			break
		}
		result.Assertions.CallOrder = append(result.Assertions.CallOrder, entry)
	}

	return result
}

func cloneTaskAssertions(assertions TaskAssertions) TaskAssertions {
	copyAssertions := assertions
	copyAssertions.Apps = append(assertions.Apps[:0:0], assertions.Apps...)
	copyAssertions.ToolsUsed = append([]ToolAssertion(nil), assertions.ToolsUsed...)
	copyAssertions.RequireAny = append([]ToolAssertion(nil), assertions.RequireAny...)
	copyAssertions.ToolsNotUsed = append([]ToolAssertion(nil), assertions.ToolsNotUsed...)
	copyAssertions.ResourcesRead = append([]ResourceAssertion(nil), assertions.ResourcesRead...)
	copyAssertions.ResourcesNotRead = append([]ResourceAssertion(nil), assertions.ResourcesNotRead...)
	copyAssertions.PromptsUsed = append([]PromptAssertion(nil), assertions.PromptsUsed...)
	copyAssertions.PromptsNotUsed = append([]PromptAssertion(nil), assertions.PromptsNotUsed...)
	copyAssertions.CallOrder = append([]CallOrderAssertion(nil), assertions.CallOrder...)
	copyAssertions.SkillsLoaded = append([]SkillAssertion(nil), assertions.SkillsLoaded...)
	copyAssertions.SkillsNotLoaded = append([]SkillAssertion(nil), assertions.SkillsNotLoaded...)
	if assertions.MinToolCalls != nil {
		min := *assertions.MinToolCalls
		copyAssertions.MinToolCalls = &min
	}
	if assertions.MaxToolCalls != nil {
		max := *assertions.MaxToolCalls
		copyAssertions.MaxToolCalls = &max
	}
	return copyAssertions
}

func toolPresence(assertion ToolAssertion, inventory CapabilityInventory) (reason, target string) {
	tools, serverPresent := inventory[assertion.Server]
	if !serverPresent {
		return "server not present", toolAssertionTarget(assertion)
	}
	if assertion.Tool == "" && assertion.ToolPattern == "" {
		return "", ""
	}
	for _, tool := range tools {
		if matchesToolName(assertion.Server, tool, assertion) {
			return "", ""
		}
	}
	return "tool not present", toolAssertionTarget(assertion)
}

func toolAssertionTarget(assertion ToolAssertion) string {
	if assertion.Tool != "" {
		return assertion.Tool
	}
	return assertion.ToolPattern
}

func hasVisibleTool(name string, tools []string) bool {
	for _, tool := range tools {
		if tool == name {
			return true
		}
	}
	return false
}

func filterResources(configured []ResourceAssertion, inventory CapabilityInventory, assertionType string, addMissing func(AssertionPresenceResult)) []ResourceAssertion {
	filtered := make([]ResourceAssertion, 0, len(configured))
	for _, assertion := range configured {
		if _, present := inventory[assertion.Server]; !present {
			addMissing(AssertionPresenceResult{Type: assertionType, Server: assertion.Server, Reason: "server not present"})
			continue
		}
		filtered = append(filtered, assertion)
	}
	return filtered
}

func filterPrompts(configured []PromptAssertion, inventory CapabilityInventory, assertionType string, addMissing func(AssertionPresenceResult)) []PromptAssertion {
	filtered := make([]PromptAssertion, 0, len(configured))
	for _, assertion := range configured {
		if _, present := inventory[assertion.Server]; !present {
			addMissing(AssertionPresenceResult{Type: assertionType, Server: assertion.Server, Reason: "server not present"})
			continue
		}
		filtered = append(filtered, assertion)
	}
	return filtered
}
