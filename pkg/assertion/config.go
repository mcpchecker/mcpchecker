package assertion

import (
	"fmt"
	"regexp"

	"github.com/mcpchecker/mcpchecker/pkg/mcpapps"
)

// TODO: add a custom Verify script for another form of assertion
type TaskAssertions struct {
	// Apps validates tool-associated HTML UI resources.
	Apps []AppAssertion `json:"apps,omitempty"`

	// RequirePresent fails assertions whose targeted server or tool is not present.
	RequirePresent bool `json:"requirePresent,omitempty"`

	// Tool assertions
	ToolsUsed    []ToolAssertion `json:"toolsUsed,omitempty"`
	RequireAny   []ToolAssertion `json:"requireAny,omitempty"`
	ToolsNotUsed []ToolAssertion `json:"toolsNotUsed,omitempty"`
	MinToolCalls *int            `json:"minToolCalls,omitempty"`
	MaxToolCalls *int            `json:"maxToolCalls,omitempty"`

	// Resource assertions
	ResourcesRead    []ResourceAssertion `json:"resourcesRead,omitempty"`
	ResourcesNotRead []ResourceAssertion `json:"resourcesNotRead,omitempty"`

	// Prompt assertions
	PromptsUsed    []PromptAssertion `json:"promptsUsed,omitempty"`
	PromptsNotUsed []PromptAssertion `json:"promptsNotUsed,omitempty"`

	// Order assertions
	CallOrder []CallOrderAssertion `json:"callOrder,omitempty"`

	// Efficiency assertions
	NoDuplicateCalls bool `json:"noDuplicateCalls,omitempty"`

	// Skill assertions - evaluated against agent tool calls
	SkillsLoaded    []SkillAssertion `json:"skillsLoaded,omitempty"`
	SkillsNotLoaded []SkillAssertion `json:"skillsNotLoaded,omitempty"`
}

// IsEmpty reports whether the assertion set contains no configured assertions.
func (a *TaskAssertions) IsEmpty() bool {
	return a == nil || (len(a.Apps) == 0 && len(a.ToolsUsed) == 0 &&
		len(a.RequireAny) == 0 &&
		len(a.ToolsNotUsed) == 0 &&
		a.MinToolCalls == nil &&
		a.MaxToolCalls == nil &&
		len(a.ResourcesRead) == 0 &&
		len(a.ResourcesNotRead) == 0 &&
		len(a.PromptsUsed) == 0 &&
		len(a.PromptsNotUsed) == 0 &&
		len(a.CallOrder) == 0 &&
		!a.NoDuplicateCalls &&
		len(a.SkillsLoaded) == 0 &&
		len(a.SkillsNotLoaded) == 0)
}

// Validate checks selectors and UI expectations shared by task-level and eval-level assertions.
func (a *TaskAssertions) Validate() error {
	if a == nil {
		return nil
	}

	for i, app := range a.Apps {
		if err := app.Validate(); err != nil {
			return fmt.Errorf("apps[%d]: %w", i, err)
		}
	}

	toolGroups := map[string][]ToolAssertion{
		"toolsUsed": a.ToolsUsed, "requireAny": a.RequireAny, "toolsNotUsed": a.ToolsNotUsed,
	}
	for name, assertions := range toolGroups {
		for i, assertion := range assertions {
			if err := validatePattern(fmt.Sprintf("%s[%d].toolPattern", name, i), assertion.ToolPattern); err != nil {
				return err
			}
		}
	}

	resourceGroups := map[string][]ResourceAssertion{
		"resourcesRead": a.ResourcesRead, "resourcesNotRead": a.ResourcesNotRead,
	}
	for name, assertions := range resourceGroups {
		for i, assertion := range assertions {
			if err := validatePattern(fmt.Sprintf("%s[%d].uriPattern", name, i), assertion.URIPattern); err != nil {
				return err
			}
		}
	}

	promptGroups := map[string][]PromptAssertion{
		"promptsUsed": a.PromptsUsed, "promptsNotUsed": a.PromptsNotUsed,
	}
	for name, assertions := range promptGroups {
		for i, assertion := range assertions {
			if err := validatePattern(fmt.Sprintf("%s[%d].promptPattern", name, i), assertion.PromptPattern); err != nil {
				return err
			}
		}
	}

	skillGroups := map[string][]SkillAssertion{
		"skillsLoaded": a.SkillsLoaded, "skillsNotLoaded": a.SkillsNotLoaded,
	}
	for name, assertions := range skillGroups {
		for i, assertion := range assertions {
			if err := validatePattern(fmt.Sprintf("%s[%d].skillPattern", name, i), assertion.SkillPattern); err != nil {
				return err
			}
		}
	}

	return nil
}

func validatePattern(field, pattern string) error {
	if pattern == "" {
		return nil
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return fmt.Errorf("%s is invalid: %w", field, err)
	}
	return nil
}

// SkillAssertion identifies a skill by name or pattern for assertion matching.
// Matching is done by searching the serialized RawInput of agent tool calls
// whose Title matches the configured skill tool name.
type SkillAssertion struct {
	// Skill is the exact skill name to match (quoted string match in serialized tool call input)
	Skill string `json:"skill,omitempty"`
	// SkillPattern is a regex pattern to match against tool call input
	SkillPattern string `json:"skillPattern,omitempty"`
}

type ToolAssertion struct {
	Server string `json:"server"`

	// Exactly one of Tool or ToolPattern should be set
	// If neither is set, matches any tool from the server
	Tool        string `json:"tool,omitempty"`
	ToolPattern string `json:"toolPattern,omitempty"` // regex pattern
}

type ResourceAssertion struct {
	Server string `json:"server"`

	// Exactly one of URI or URIPattern should be set
	// If neither is set, matches any resource from the server
	URI        string `json:"uri,omitempty"`
	URIPattern string `json:"uriPattern,omitempty"` // regex pattern
}

type PromptAssertion struct {
	Server string `json:"server"`

	// Exactly one of Prompt or PromptPattern should be set
	// If neither is set, matches any prompt from the server
	Prompt        string `json:"prompt,omitempty"`
	PromptPattern string `json:"promptPattern,omitempty"`
}

type CallOrderAssertion struct {
	Type   string `json:"type"` // "tool", "resource", "prompt"
	Server string `json:"server"`
	Name   string `json:"name"`
}

// AppAssertion identifies a tool whose UI resource must be retrievable.
type AppAssertion = mcpapps.Assertion
