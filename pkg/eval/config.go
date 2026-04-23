package eval

import (
	"fmt"
	"os"
	"path/filepath"

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

	// Sources are external task repositories
	Sources map[string]*SourceSpec `json:"sources,omitempty"`

	// Extensions configuration
	Extensions map[string]*extension.ExtensionSpec `json:"extensions"`

	// MCP configuration
	McpConfigFile string                       `json:"mcpConfigFile"`
	LLMJudge      *llmjudge.LLMJudgeEvalConfig `json:"llmJudge"`

	// DefaultTaskLimits sets default timeout limits for all tasks in this eval.
	// Individual tasks can override these via spec.limits.
	DefaultTaskLimits *util.Limits `json:"defaultTaskLimits,omitempty"`

	// Advanced mode: different assertion sets
	TaskSets []TaskSet `json:"taskSets,omitempty"`
}

type TaskSet struct {
	// Source references a named source from config.sources.
	// When set, Glob/Path are resolved relative to the fetched source directory.
	Source string `json:"source,omitempty"`

	// Exactly one of Glob or Path must be set
	Glob string `json:"glob,omitempty"`
	Path string `json:"path,omitempty"`

	// Optional label selector - filters tasks by labels
	// All specified labels must match (AND logic)
	LabelSelector map[string]string `json:"labelSelector,omitempty"`

	Assertions *TaskAssertions `json:"assertions,omitempty"`

	// ServerMapping rewrites task requires.mcpServer names when loading tasks
	// from this set. Populated automatically during source expansion; not set
	// directly in eval.yaml.
	ServerMapping map[string]string `json:"serverMapping,omitempty"`
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

	// Resolve task set paths and globs
	for i := range spec.Config.TaskSets {
		if spec.Config.TaskSets[i].Path != "" {
			if err := resolveFilePath(&spec.Config.TaskSets[i].Path, basePath); err != nil {
				return nil, fmt.Errorf("failed to resolve task set path at index %d: %w", i, err)
			}
		} else if spec.Config.TaskSets[i].Glob != "" {
			if err := resolveFilePath(&spec.Config.TaskSets[i].Glob, basePath); err != nil {
				return nil, fmt.Errorf("failed to resolve task set glob at index %d: %w", i, err)
			}
		}
	}

	return spec, nil
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
