package agent

import (
	"os/exec"
	"testing"

	"github.com/mcpchecker/mcpchecker/pkg/acpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadWithBuiltins(t *testing.T) {
	tests := map[string]struct {
		file        string
		setupEnv    func()
		cleanupEnv  func()
		expectErr   bool
		errContains string
		validate    func(t *testing.T, spec *AgentSpec)
		shouldSkip  bool
	}{
		"claude-code builtin": {
			file: "builtin-claude-code.yaml",
			validate: func(t *testing.T, spec *AgentSpec) {
				assert.Equal(t, "claude-code", spec.Metadata.Name)
				require.NotNil(t, spec.AcpConfig)
				assert.Equal(t, "claude-agent-acp", spec.AcpConfig.Cmd)
			},
			shouldSkip: func() bool {
				_, err := exec.LookPath("claude-agent-acp")
				return err != nil
			}(),
		},
		"llm-agent builtin": {
			file: "builtin-openai-agent.yaml",
			validate: func(t *testing.T, spec *AgentSpec) {
				assert.Equal(t, "llm-agent-openai-gpt-4", spec.Metadata.Name)
				require.NotNil(t, spec.Builtin)
				assert.Equal(t, "llm-agent", spec.Builtin.Type)
				assert.Equal(t, "openai:gpt-4", spec.Builtin.Model)
			},
		},
		"builtin with overrides": {
			file: "builtin-with-overrides.yaml",
			validate: func(t *testing.T, spec *AgentSpec) {
				// Name should be overridden
				assert.Equal(t, "custom-llm", spec.Metadata.Name)
				// UseVirtualHome should be true as specified in the YAML override
				require.NotNil(t, spec.Commands.UseVirtualHome)
				assert.True(t, *spec.Commands.UseVirtualHome)
				// Builtin configuration should be present
				require.NotNil(t, spec.Builtin)
				assert.Equal(t, "llm-agent", spec.Builtin.Type)
			},
		},
		"non-builtin agent (no builtin field)": {
			file: "claude-agent.yaml",
			validate: func(t *testing.T, spec *AgentSpec) {
				// Should load normally without builtin processing
				assert.Equal(t, "claude", spec.Metadata.Name)
				assert.NotNil(t, spec.Metadata.Version)
				assert.Equal(t, "2.0.x", *spec.Metadata.Version)
			},
		},
		"invalid builtin type": {
			file:        "builtin-invalid-type.yaml",
			expectErr:   true,
			errContains: "unknown builtin type",
		},
		"builtin requires model but not provided": {
			file:        "builtin-openai-no-model.yaml",
			expectErr:   true,
			errContains: "requires a model",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.shouldSkip {
				t.Skipf("skipping test %s because shouldSkip=true", name)
			}

			if tc.setupEnv != nil {
				tc.setupEnv()
			}
			if tc.cleanupEnv != nil {
				defer tc.cleanupEnv()
			}

			spec, err := LoadWithBuiltins(basePath + "/" + tc.file)

			if tc.expectErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, spec)

			if tc.validate != nil {
				tc.validate(t, spec)
			}
		})
	}
}

func TestNewRunnerForSpec(t *testing.T) {
	tt := map[string]struct {
		spec        *AgentSpec
		expectErr   bool
		errContains string
		validate    func(t *testing.T, runner Runner)
	}{
		"nil spec returns error": {
			spec:        nil,
			expectErr:   true,
			errContains: "nil AgentSpec",
		},
		"acp config returns acpRunner": {
			spec: &AgentSpec{
				Metadata: AgentMetadata{Name: "acp-test"},
				AcpConfig: &acpclient.AcpConfig{
					Cmd:  "test-acp-cmd",
					Args: []string{"--arg1"},
				},
			},
			validate: func(t *testing.T, runner Runner) {
				assert.Equal(t, "acp-test", runner.AgentName())
				_, ok := runner.(*acpRunner)
				assert.True(t, ok, "expected runner to be *acpRunner")
			},
		},
		"acp config takes precedence over builtin": {
			spec: &AgentSpec{
				Metadata: AgentMetadata{Name: "acp-priority"},
				AcpConfig: &acpclient.AcpConfig{
					Cmd: "acp-cmd",
				},
				Builtin: &BuiltinRef{
					Type:  "openai-agent",
					Model: "gpt-4",
				},
			},
			validate: func(t *testing.T, runner Runner) {
				// AcpConfig should take precedence
				_, ok := runner.(*acpRunner)
				assert.True(t, ok, "expected acpRunner when both AcpConfig and Builtin are set")
				assert.Equal(t, "acp-priority", runner.AgentName())
			},
		},
		"spec without acp or builtin returns agentSpecRunner": {
			spec: &AgentSpec{
				Metadata: AgentMetadata{Name: "shell-agent"},
				Commands: AgentCommands{
					RunPrompt: "echo hello",
				},
			},
			validate: func(t *testing.T, runner Runner) {
				assert.Equal(t, "shell-agent", runner.AgentName())
				_, ok := runner.(*agentSpecRunner)
				assert.True(t, ok, "expected runner to be *agentSpecRunner")
			},
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			runner, err := NewRunnerForSpec(tc.spec)

			if tc.expectErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, runner)

			if tc.validate != nil {
				tc.validate(t, runner)
			}
		})
	}
}

func TestMergeAgentSpecs(t *testing.T) {
	t.Run("override metadata name", func(t *testing.T) {
		base := &AgentSpec{
			Metadata: AgentMetadata{Name: "base"},
		}
		override := &AgentSpec{
			Metadata: AgentMetadata{Name: "override"},
		}
		result := mergeAgentSpecs(base, override)
		assert.Equal(t, "override", result.Metadata.Name)
	})

	t.Run("override commands", func(t *testing.T) {
		baseUseVirtualHome := false
		overrideUseVirtualHome := true
		base := &AgentSpec{
			Commands: AgentCommands{
				UseVirtualHome:       &baseUseVirtualHome,
				ArgTemplateMcpServer: "{{ .File }}",
				RunPrompt:            "base command",
			},
		}
		override := &AgentSpec{
			Commands: AgentCommands{
				UseVirtualHome: &overrideUseVirtualHome,
				RunPrompt:      "override command",
			},
		}
		result := mergeAgentSpecs(base, override)

		// Overridden fields
		require.NotNil(t, result.Commands.UseVirtualHome)
		assert.True(t, *result.Commands.UseVirtualHome)
		assert.Equal(t, "override command", result.Commands.RunPrompt)

		// Non-overridden fields should keep base value
		assert.Equal(t, "{{ .File }}", result.Commands.ArgTemplateMcpServer)
	})

	t.Run("override preserves base when override is empty", func(t *testing.T) {
		base := &AgentSpec{
			Metadata: AgentMetadata{Name: "base"},
			Commands: AgentCommands{
				ArgTemplateMcpServer: "{{ .File }}",
				RunPrompt:            "base command",
			},
		}
		override := &AgentSpec{
			Metadata: AgentMetadata{Name: "override"},
			// Commands not specified
		}
		result := mergeAgentSpecs(base, override)

		assert.Equal(t, "override", result.Metadata.Name)
		assert.Equal(t, "{{ .File }}", result.Commands.ArgTemplateMcpServer)
		assert.Equal(t, "base command", result.Commands.RunPrompt)
	})
}
