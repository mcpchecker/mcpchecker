package agent

import (
	"fmt"
	"os"

	"github.com/mcpchecker/mcpchecker/pkg/acpclient"
	"github.com/mcpchecker/mcpchecker/pkg/util"
	"sigs.k8s.io/yaml"
)

const (
	KindAgent = "Agent"
)

type AgentSpec struct {
	util.TypeMeta `json:",inline"`
	Metadata      AgentMetadata        `json:"metadata"`
	Builtin       *BuiltinRef          `json:"builtin,omitempty"`
	AcpConfig     *acpclient.AcpConfig `json:"acp,omitempty"` // if builtin and acp are both set, default to acp
	Commands      AgentCommands        `json:"commands"`
	Skills        *AgentSkillsConfig   `json:"skills,omitempty"`
}

// AgentSkillsConfig defines agent-specific skill loading behavior
type AgentSkillsConfig struct {
	// MountPath is the relative path within the agent's working directory
	// where skill files should be placed (e.g., ".claude/skills" for Claude Code)
	MountPath string `json:"mountPath"`

	// ToolName is the agent-specific tool name used to load skills
	// (e.g., "Skill" for Claude Code)
	ToolName string `json:"toolName"`
}

// AgentRef specifies how to configure the agent
// Use "type: builtin.X" for built-in agents or "type: file" for custom agent files
type AgentRef struct {
	// Type specifies the agent type:
	// - "builtin.claude-code" for Claude Code
	// - "builtin.llm-agent" for LLM agents (supports openai, anthropic, gemini, etc.)
	// - "file" for custom agent configuration files
	Type string `json:"type"`

	// Path to agent configuration file (required when type is "file")
	Path string `json:"path,omitempty"`

	// Model in "provider:model-id" format (required for builtin.llm-agent)
	Model string `json:"model,omitempty"`
}

// BuiltinRef references a built-in agent type with optional model
type BuiltinRef struct {
	// Type is the built-in agent type (e.g., "llm-agent", "claude-code")
	Type string `json:"type"`

	// Model is the model to use in "provider:model-id" format (e.g. "openai:gpt-4o").
	// Required for "llm-agent" type.
	Model string `json:"model,omitempty"`

	// BaseURL is the API base URL (deprecated: used for backwards compat with openai-agent/openai-acp configs)
	BaseURL string `json:"baseUrl,omitempty"`

	// APIKey is the API key (deprecated: used for backwards compat with openai-agent/openai-acp configs)
	APIKey string `json:"apiKey,omitempty"`
}

type AgentMetadata struct {
	// Name of the agent
	Name string `json:"name"`

	// Version of the agent - used if Commands.GetVersion is not set
	Version *string `json:"version,omitempty"`
}

type AgentCommands struct {
	// Whether or not to create a virtual $HOME for executing the agent without existing config
	UseVirtualHome *bool `json:"useVirtualHome,omitempty"`

	// A template for how the mcp servers config files should be provided to the prompt
	// the server file will be in {{ .File }}
	// the server URL will be in {{ .URL }}
	ArgTemplateMcpServer string `json:"argTemplateMcpServer"`

	// A template for how the mcp agents allowed tools should be provided to the prompt
	// the server name will be in {{ .ServerName }}
	// the tool name will be in {{ .ToolName }}
	ArgTemplateAllowedTools string `json:"argTemplateAllowedTools"`

	// The separator to use when joining allowed tools together
	// Defaults to " " (space) if not specified
	AllowedToolsJoinSeparator *string `json:"allowedToolsJoinSeparator,omitempty"`

	// A template command to run the agent with a prompt and some mcp servers
	// the prompt will be in {{ .Prompt }}
	// the servers will be in {{ .McpServerFileArgs }}
	// the allowed tools will be in {{ .AllowedToolArgs }}
	RunPrompt string `json:"runPrompt"`

	// An optional command to get the version of the agent
	// useful for generic agents such as claude code that may autoupdate/have different versions on different machines
	GetVersion *string `json:"getVersion,omitempty"`
}

func Read(data []byte) (*AgentSpec, error) {
	spec := &AgentSpec{}

	err := yaml.Unmarshal(data, spec)
	if err != nil {
		return nil, err
	}

	if err := spec.TypeMeta.Validate(KindAgent); err != nil {
		return nil, err
	}

	if spec.Skills != nil {
		if spec.Skills.MountPath == "" {
			return nil, fmt.Errorf("skills.mountPath is required when skills are configured")
		}
		if spec.Skills.ToolName == "" {
			return nil, fmt.Errorf("skills.toolName is required when skills are configured")
		}
	}

	return spec, nil
}

func FromFile(path string) (*AgentSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s' for agentspec: %w", path, err)
	}

	return Read(data)
}
