package eval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveCapabilitiesToolPresence(t *testing.T) {
	tests := []struct {
		name      string
		assertion ToolAssertion
		inventory CapabilityInventory
		present   bool
		reason    string
		target    string
	}{
		{name: "exact tool present", assertion: ToolAssertion{Server: "s", Tool: "one"}, inventory: CapabilityInventory{"s": {"one"}}, present: true},
		{name: "exact tool absent", assertion: ToolAssertion{Server: "s", Tool: "one"}, inventory: CapabilityInventory{"s": {"two"}}, reason: "tool not present", target: "one"},
		{name: "server absent", assertion: ToolAssertion{Server: "s", Tool: "one"}, inventory: CapabilityInventory{}, reason: "server not present", target: "one"},
		{name: "pattern matches", assertion: ToolAssertion{Server: "s", ToolPattern: `^one-[0-9]+$`}, inventory: CapabilityInventory{"s": {"other", "one-2"}}, present: true},
		{name: "pattern does not match", assertion: ToolAssertion{Server: "s", ToolPattern: `^one-[0-9]+$`}, inventory: CapabilityInventory{"s": {"other"}}, reason: "tool not present", target: `^one-[0-9]+$`},
		{name: "invalid pattern is a non-match", assertion: ToolAssertion{Server: "s", ToolPattern: "["}, inventory: CapabilityInventory{"s": {"one"}}, reason: "tool not present", target: "["},
		{name: "server-only assertion", assertion: ToolAssertion{Server: "s"}, inventory: CapabilityInventory{"s": {}}, present: true},
		{name: "server-only assertion with missing server", assertion: ToolAssertion{Server: "s"}, inventory: CapabilityInventory{}, reason: "server not present"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, requirePresent := range []bool{false, true} {
				result := ResolveCapabilities(TaskAssertions{
					RequirePresent: requirePresent,
					ToolsUsed:      []ToolAssertion{tt.assertion},
				}, tt.inventory)
				if tt.present {
					assert.Equal(t, []ToolAssertion{tt.assertion}, result.Assertions.ToolsUsed)
					assert.Empty(t, result.SkippedAssertions)
					assert.Empty(t, result.PresenceFailures)
					continue
				}

				assert.Empty(t, result.Assertions.ToolsUsed)
				record := []AssertionPresenceResult{{
					Type: assertionTypeToolsUsed, Server: tt.assertion.Server,
					Target: tt.target, Reason: tt.reason,
				}}
				if requirePresent {
					assert.Equal(t, record, result.PresenceFailures)
					assert.Empty(t, result.SkippedAssertions)
				} else {
					assert.Equal(t, record, result.SkippedAssertions)
					assert.Empty(t, result.PresenceFailures)
				}
			}
		})
	}
}

func TestResolveCapabilitiesAppsPresence(t *testing.T) {
	app := AppAssertion{Server: "s", Tool: "view"}
	for _, test := range []struct {
		name      string
		inventory CapabilityInventory
		reason    string
	}{
		{"present", CapabilityInventory{"s": {"view"}}, ""},
		{"tool absent", CapabilityInventory{"s": {"other"}}, "tool not present"},
		{"server absent", CapabilityInventory{}, "server not present"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, requirePresent := range []bool{false, true} {
				configured := TaskAssertions{Apps: []AppAssertion{app}, RequirePresent: requirePresent}
				resolved := ResolveCapabilities(configured, test.inventory)
				assert.Equal(t, []AppAssertion{app}, configured.Apps, "resolution must not modify configured assertions")
				result := &CompositeAssertionResult{Apps: evaluateApps(resolved.Assertions.Apps, nil), SkippedAssertions: resolved.SkippedAssertions, PresenceFailures: resolved.PresenceFailures}
				if test.reason == "" {
					assert.Equal(t, []AppAssertion{app}, resolved.Assertions.Apps)
					assert.Empty(t, resolved.SkippedAssertions)
					assert.Empty(t, resolved.PresenceFailures)
					assert.Contains(t, result.Apps.Reason, "tool_not_invoked")
					continue
				}
				assert.Empty(t, resolved.Assertions.Apps)
				assert.Nil(t, result.Apps, "absent targets must not produce tool_not_invoked")
				record := []AssertionPresenceResult{{Type: assertionTypeApps, Server: app.Server, Target: app.Tool, Reason: test.reason}}
				if requirePresent {
					assert.Equal(t, record, resolved.PresenceFailures)
					assert.Empty(t, resolved.SkippedAssertions)
					assert.False(t, result.Succeeded())
					assert.Equal(t, 1, result.TotalAssertions())
				} else {
					assert.Equal(t, record, resolved.SkippedAssertions)
					assert.Empty(t, resolved.PresenceFailures)
					assert.True(t, result.Succeeded())
					assert.Zero(t, result.TotalAssertions())
				}
			}
		})
	}
}

func TestResolveCapabilitiesToolsNotUsedMissingTargetRequirePresent(t *testing.T) {
	tests := []struct {
		name      string
		assertion ToolAssertion
		inventory CapabilityInventory
		reason    string
		target    string
	}{
		{name: "missing tool", assertion: ToolAssertion{Server: "s", Tool: "missing"}, inventory: CapabilityInventory{"s": {"visible"}}, reason: "tool not present", target: "missing"},
		{name: "missing server", assertion: ToolAssertion{Server: "s", Tool: "missing"}, inventory: CapabilityInventory{}, reason: "server not present", target: "missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, requirePresent := range []bool{false, true} {
				result := ResolveCapabilities(TaskAssertions{
					RequirePresent: requirePresent,
					ToolsNotUsed:   []ToolAssertion{tt.assertion},
				}, tt.inventory)
				record := AssertionPresenceResult{Type: assertionTypeToolsNotUsed, Server: "s", Target: tt.target, Reason: tt.reason}
				assert.Empty(t, result.Assertions.ToolsNotUsed)
				if requirePresent {
					assert.Equal(t, []AssertionPresenceResult{record}, result.PresenceFailures)
					assert.Empty(t, result.SkippedAssertions)
				} else {
					assert.Equal(t, []AssertionPresenceResult{record}, result.SkippedAssertions)
					assert.Empty(t, result.PresenceFailures)
				}
			}
		})
	}
}

func TestResolveCapabilitiesRequireAny(t *testing.T) {
	tests := []struct {
		name          string
		candidates    []ToolAssertion
		inventory     CapabilityInventory
		wantRemaining []ToolAssertion
		wantMissing   bool
	}{
		{
			name:          "all present",
			candidates:    []ToolAssertion{{Server: "s", Tool: "one"}, {Server: "s", Tool: "two"}},
			inventory:     CapabilityInventory{"s": {"one", "two"}},
			wantRemaining: []ToolAssertion{{Server: "s", Tool: "one"}, {Server: "s", Tool: "two"}},
		},
		{
			name:          "some present removes absent candidates",
			candidates:    []ToolAssertion{{Server: "s", Tool: "one"}, {Server: "s", Tool: "two"}},
			inventory:     CapabilityInventory{"s": {"one"}},
			wantRemaining: []ToolAssertion{{Server: "s", Tool: "one"}},
		},
		{
			name:        "none present",
			candidates:  []ToolAssertion{{Server: "s", Tool: "one"}, {Server: "other", Tool: "two"}},
			inventory:   CapabilityInventory{"s": {"visible"}},
			wantMissing: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveCapabilities(TaskAssertions{RequireAny: tt.candidates}, tt.inventory)
			if tt.wantRemaining == nil {
				assert.Empty(t, result.Assertions.RequireAny)
			} else {
				assert.Equal(t, tt.wantRemaining, result.Assertions.RequireAny)
			}
			if !tt.wantMissing {
				assert.Empty(t, result.SkippedAssertions)
				assert.Empty(t, result.PresenceFailures)
				return
			}
			assert.Equal(t, []AssertionPresenceResult{{
				Type: assertionTypeRequireAny, Server: "s", Target: "one", Reason: "tool not present",
			}}, result.SkippedAssertions)

			strict := ResolveCapabilities(TaskAssertions{RequirePresent: true, RequireAny: tt.candidates}, tt.inventory)
			assert.Empty(t, strict.SkippedAssertions)
			assert.Equal(t, result.SkippedAssertions, strict.PresenceFailures)
		})
	}
}

func TestResolveCapabilitiesCallOrderMissingTool(t *testing.T) {
	callOrder := []CallOrderAssertion{
		{Type: "tool", Server: "s", Name: "visible"},
		{Type: "tool", Server: "s", Name: "missing"},
	}
	for _, requirePresent := range []bool{false, true} {
		t.Run(map[bool]string{false: "skip", true: "fail"}[requirePresent], func(t *testing.T) {
			result := ResolveCapabilities(TaskAssertions{RequirePresent: requirePresent, CallOrder: callOrder}, CapabilityInventory{"s": {"visible"}})
			assert.Empty(t, result.Assertions.CallOrder, "the complete order assertion must be removed")
			record := AssertionPresenceResult{Type: assertionTypeCallOrder, Server: "s", Target: "missing", Reason: "tool not present"}
			if requirePresent {
				assert.Equal(t, []AssertionPresenceResult{record}, result.PresenceFailures)
			} else {
				assert.Equal(t, []AssertionPresenceResult{record}, result.SkippedAssertions)
			}
		})
	}
}

func TestResolveCapabilitiesCallOrderMissingServer(t *testing.T) {
	for _, requirePresent := range []bool{false, true} {
		t.Run(map[bool]string{false: "skip", true: "fail"}[requirePresent], func(t *testing.T) {
			result := ResolveCapabilities(TaskAssertions{
				RequirePresent: requirePresent,
				CallOrder:      []CallOrderAssertion{{Type: callTypeTool, Server: "missing", Name: "tool"}},
			}, CapabilityInventory{})
			assert.Empty(t, result.Assertions.CallOrder)
			record := AssertionPresenceResult{Type: assertionTypeCallOrder, Server: "missing", Target: "tool", Reason: "server not present"}
			if requirePresent {
				assert.Equal(t, []AssertionPresenceResult{record}, result.PresenceFailures)
			} else {
				assert.Equal(t, []AssertionPresenceResult{record}, result.SkippedAssertions)
			}
		})
	}
}

func TestResolveCapabilitiesResourcesAndPromptsRequirePresent(t *testing.T) {
	for _, requirePresent := range []bool{false, true} {
		t.Run(map[bool]string{false: "skip", true: "fail"}[requirePresent], func(t *testing.T) {
			assertions := TaskAssertions{
				RequirePresent:   requirePresent,
				ResourcesRead:    []ResourceAssertion{{Server: "resource-read", URI: "r1"}},
				ResourcesNotRead: []ResourceAssertion{{Server: "resource-not-read", URI: "r2"}},
				PromptsUsed:      []PromptAssertion{{Server: "prompt-used", Prompt: "p1"}},
				PromptsNotUsed:   []PromptAssertion{{Server: "prompt-not-used", Prompt: "p2"}},
			}
			result := ResolveCapabilities(assertions, CapabilityInventory{})
			assert.Empty(t, result.Assertions.ResourcesRead)
			assert.Empty(t, result.Assertions.ResourcesNotRead)
			assert.Empty(t, result.Assertions.PromptsUsed)
			assert.Empty(t, result.Assertions.PromptsNotUsed)
			records := []AssertionPresenceResult{
				{Type: assertionTypeResourcesRead, Server: "resource-read", Reason: "server not present"},
				{Type: assertionTypeResourcesNotRead, Server: "resource-not-read", Reason: "server not present"},
				{Type: assertionTypePromptsUsed, Server: "prompt-used", Reason: "server not present"},
				{Type: assertionTypePromptsNotUsed, Server: "prompt-not-used", Reason: "server not present"},
			}
			if requirePresent {
				assert.Equal(t, records, result.PresenceFailures)
				assert.Empty(t, result.SkippedAssertions)
			} else {
				assert.Equal(t, records, result.SkippedAssertions)
				assert.Empty(t, result.PresenceFailures)
			}
		})
	}
}

func TestResolveCapabilitiesDoesNotMutateAssertions(t *testing.T) {
	minCalls, maxCalls := 1, 3
	assertions := TaskAssertions{
		RequirePresent:   false,
		ToolsUsed:        []ToolAssertion{{Server: "s", Tool: "missing"}},
		RequireAny:       []ToolAssertion{{Server: "s", Tool: "visible"}, {Server: "s", Tool: "missing"}},
		ToolsNotUsed:     []ToolAssertion{{Server: "s", Tool: "missing"}},
		MinToolCalls:     &minCalls,
		MaxToolCalls:     &maxCalls,
		ResourcesRead:    []ResourceAssertion{{Server: "s", URI: "r"}},
		ResourcesNotRead: []ResourceAssertion{{Server: "s", URI: "r2"}},
		PromptsUsed:      []PromptAssertion{{Server: "s", Prompt: "p"}},
		PromptsNotUsed:   []PromptAssertion{{Server: "s", Prompt: "p2"}},
		CallOrder:        []CallOrderAssertion{{Type: "tool", Server: "s", Name: "missing"}},
		NoDuplicateCalls: true,
		SkillsLoaded:     []SkillAssertion{{Skill: "skill"}},
		SkillsNotLoaded:  []SkillAssertion{{Skill: "other"}},
	}
	original := cloneTaskAssertions(assertions)

	result := ResolveCapabilities(assertions, CapabilityInventory{"s": {"visible"}})
	assert.Equal(t, original, assertions)
	assert.NotSame(t, assertions.MinToolCalls, result.Assertions.MinToolCalls)
	assert.NotSame(t, assertions.MaxToolCalls, result.Assertions.MaxToolCalls)
	assert.Equal(t, []ToolAssertion{{Server: "s", Tool: "visible"}}, result.Assertions.RequireAny)
	assert.Equal(t, original.RequireAny, assertions.RequireAny)
	assert.Empty(t, result.Assertions.ToolsUsed)
	assert.Len(t, result.SkippedAssertions, 3)
}

func TestResolveCapabilitiesAlwaysKeepsUnconditionalAssertions(t *testing.T) {
	minCalls, maxCalls := 1, 4
	result := ResolveCapabilities(TaskAssertions{
		MinToolCalls:     &minCalls,
		MaxToolCalls:     &maxCalls,
		NoDuplicateCalls: true,
		SkillsLoaded:     []SkillAssertion{{Skill: "loaded"}},
		SkillsNotLoaded:  []SkillAssertion{{Skill: "not-loaded"}},
	}, nil)
	assert.Equal(t, &minCalls, result.Assertions.MinToolCalls)
	assert.Equal(t, &maxCalls, result.Assertions.MaxToolCalls)
	assert.True(t, result.Assertions.NoDuplicateCalls)
	assert.Equal(t, []SkillAssertion{{Skill: "loaded"}}, result.Assertions.SkillsLoaded)
	assert.Equal(t, []SkillAssertion{{Skill: "not-loaded"}}, result.Assertions.SkillsNotLoaded)
}
