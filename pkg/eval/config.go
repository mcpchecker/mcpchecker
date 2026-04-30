package eval

import (
	"fmt"
	"os"
	gopath "path"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/mcpchecker/mcpchecker/pkg/agent"
	"github.com/mcpchecker/mcpchecker/pkg/extension"
	"github.com/mcpchecker/mcpchecker/pkg/llmjudge"
	"github.com/mcpchecker/mcpchecker/pkg/util"
)

const (
	KindEval = "Eval"
)

type EvalSpec struct {
	util.TypeMeta `json:",inline"`
	Metadata      EvalMetadata `json:"metadata"`
	Config        EvalConfig   `json:"config"`

	// basePath is the directory containing the eval file, used for resolving relative paths
	basePath string
}

// BasePath returns the directory containing the eval file
func (s *EvalSpec) BasePath() string {
	return s.basePath
}

type EvalMetadata struct {
	Name string `json:"name"`
}

type EvalConfig struct {
	// Agent configuration
	Agent *agent.AgentRef `json:"agent"`

	// Extensions configuration
	Extensions map[string]*extension.ExtensionSpec `json:"extensions"`

	// Sources defines cross-repo eval sources keyed by name
	Sources map[string]*SourceSpec `json:"sources,omitempty"`

	// MCP configuration
	McpConfigFile string                       `json:"mcpConfigFile"`
	LLMJudge      *llmjudge.LLMJudgeEvalConfig `json:"llmJudge"`

	// Skills configuration - defines skill sources to mount for the agent
	Skills *SkillsConfig `json:"skills,omitempty"`

	// DefaultTaskLimits sets default timeout limits for all tasks in this eval.
	// Individual tasks can override these via spec.limits.
	DefaultTaskLimits *util.Limits `json:"defaultTaskLimits,omitempty"`

	// Advanced mode: different assertion sets
	TaskSets []TaskSet `json:"taskSets,omitempty"`
}

// SkillsConfig defines skill sources to mount for agent evaluation
type SkillsConfig struct {
	// Sources is a list of skill sources to mount
	Sources []SkillSource `json:"sources"`
}

// SkillSource defines where to find skill files
type SkillSource struct {
	// Type is the source type ("path" for now)
	Type string `json:"type"`

	// Path to a local directory containing skill files (used when type is "path")
	Path string `json:"path,omitempty"`
}

// SourceSpec defines a cross-repo eval source
type SourceSpec struct {
	Repo          string            `json:"repo"`
	Ref           string            `json:"ref"`
	ServerMapping map[string]string `json:"serverMapping,omitempty"`
}

type TaskSet struct {
	// Exactly one of Glob or Path must be set
	Glob string `json:"glob,omitempty"`
	Path string `json:"path,omitempty"`

	// Source references a key in EvalConfig.Sources.
	// Mutually exclusive with absolute local paths.
	Source string `json:"source,omitempty"`

	// Optional label selector - filters tasks by labels
	// All specified labels must match (AND logic)
	LabelSelector map[string]string `json:"labelSelector,omitempty"`

	Assertions *TaskAssertions `json:"assertions,omitempty"`
}

// TODO: add a custom Verify script for another form of assertion
type TaskAssertions struct {
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

func Read(data []byte, basePath string) (*EvalSpec, error) {
	spec := &EvalSpec{}

	err := yaml.Unmarshal(data, spec)
	if err != nil {
		return nil, err
	}

	if err := spec.TypeMeta.Validate(KindEval); err != nil {
		return nil, err
	}

	// Store the base path for later use (e.g., resolving extension paths)
	spec.basePath = basePath

	// Convert all relative file paths to absolute paths
	if spec.Config.Agent != nil && spec.Config.Agent.Type == "file" {
		if err := resolveFilePath(&spec.Config.Agent.Path, basePath); err != nil {
			return nil, fmt.Errorf("failed to resolve agent file path: %w", err)
		}
	}
	if err := resolveFilePath(&spec.Config.McpConfigFile, basePath); err != nil {
		return nil, fmt.Errorf("failed to resolve mcp config file path: %w", err)
	}

	// Validate source specs
	for name, src := range spec.Config.Sources {
		if err := validateSourceSpec(name, src); err != nil {
			return nil, err
		}
	}

	// Resolve and validate skill source paths
	if spec.Config.Skills != nil {
		for i := range spec.Config.Skills.Sources {
			src := &spec.Config.Skills.Sources[i]
			if src.Type == "" {
				return nil, fmt.Errorf("skills.sources[%d]: type is required", i)
			}
			if src.Type != "path" {
				return nil, fmt.Errorf("skills.sources[%d]: unsupported type %q (must be \"path\")", i, src.Type)
			}
			if src.Path == "" {
				return nil, fmt.Errorf("skills.sources[%d]: path is required for type \"path\"", i)
			}
			if err := resolveFilePath(&src.Path, basePath); err != nil {
				return nil, fmt.Errorf("failed to resolve skill source path at index %d: %w", i, err)
			}
		}
	}

	// Resolve task set paths/globs and validate source references
	for i := range spec.Config.TaskSets {
		ts := &spec.Config.TaskSets[i]
		if ts.Source != "" {
			if err := ts.validateSource(spec.Config.Sources); err != nil {
				return nil, fmt.Errorf("taskSet[%d]: %w", i, err)
			}
		} else if ts.Path != "" {
			if err := resolveFilePath(&ts.Path, basePath); err != nil {
				return nil, fmt.Errorf("failed to resolve task set path at index %d: %w", i, err)
			}
		} else if ts.Glob != "" {
			if err := resolveFilePath(&ts.Glob, basePath); err != nil {
				return nil, fmt.Errorf("failed to resolve task set glob at index %d: %w", i, err)
			}
		}
	}

	return spec, nil
}

// taskPath returns the set path value and its kind ("path" or "glob").
func (ts *TaskSet) taskPath() (string, string) {
	if ts.Path != "" {
		return ts.Path, "path"
	}

	return ts.Glob, "glob"
}

func (ts *TaskSet) validateSource(sources map[string]*SourceSpec) error {
	if sources == nil {
		return fmt.Errorf("references source %q but no sources are defined", ts.Source)
	}

	if _, ok := sources[ts.Source]; !ok {
		return fmt.Errorf("references undefined source %q", ts.Source)
	}

	taskPath, pathKind := ts.taskPath()
	if taskPath != "" {
		if gopath.IsAbs(taskPath) || filepath.IsAbs(taskPath) {
			return fmt.Errorf(
				"has source %q but %s %q is absolute; sourced task sets must use relative paths",
				ts.Source,
				pathKind,
				taskPath,
			)
		}

		if err := validateNoPathEscape(taskPath); err != nil {
			return fmt.Errorf("%s escapes source repo root: %w", pathKind, err)
		}
	}

	return nil
}

func validateSourceSpec(name string, src *SourceSpec) error {
	if src == nil {
		return fmt.Errorf("source %q is nil", name)
	}

	if src.Repo == "" {
		return fmt.Errorf("source %q requires a repo field", name)
	}

	if src.Ref == "" {
		return fmt.Errorf("source %q requires a ref field", name)
	}

	return nil
}

// validateNoPathEscape checks that a relative path does not escape above the root via "../"
func validateNoPathEscape(p string) error {
	cleaned := gopath.Clean(p)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("path %q escapes the repo root", p)
	}

	return nil
}

func resolveFilePath(filePath *string, basePath string) error {
	if filePath == nil || *filePath == "" {
		return nil
	}

	// If the path is already absolute, leave it as-is
	if filepath.IsAbs(*filePath) {
		return nil
	}

	// Convert relative path to absolute path based on the YAML file's directory
	absPath := filepath.Join(basePath, *filePath)
	*filePath = absPath

	return nil
}

func FromFile(path string) (*EvalSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s' for evalspec: %w", path, err)
	}

	// Convert to absolute path to ensure basePath is absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for '%s': %w", path, err)
	}

	basePath := filepath.Dir(absPath)

	return Read(data, basePath)
}
