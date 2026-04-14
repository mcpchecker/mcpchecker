package agent

import (
	"testing"

	"github.com/mcpchecker/mcpchecker/pkg/acpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetBuiltinType(t *testing.T) {
	tests := map[string]struct {
		agentType    string
		shouldExist  bool
		expectedName string
	}{
		"llm-agent exists": {
			agentType:    "llm-agent",
			shouldExist:  true,
			expectedName: "llm-agent",
		},
		"openai-agent deprecated alias": {
			agentType:    "openai-agent",
			shouldExist:  true,
			expectedName: "llm-agent",
		},
		"openai-acp deprecated alias": {
			agentType:    "openai-acp",
			shouldExist:  true,
			expectedName: "llm-agent",
		},
		"claude-code exists": {
			agentType:    "claude-code",
			shouldExist:  true,
			expectedName: "claude-code",
		},
		"non-existent agent": {
			agentType:   "non-existent",
			shouldExist: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			agent, ok := GetBuiltinType(tc.agentType)
			if tc.shouldExist {
				require.True(t, ok)
				require.NotNil(t, agent)
				assert.Equal(t, tc.expectedName, agent.Name())
			} else {
				assert.False(t, ok)
				assert.Nil(t, agent)
			}
		})
	}
}

func TestListBuiltinTypes(t *testing.T) {
	agents := ListBuiltinTypes()

	// Should have at least 2 builtin agents
	assert.GreaterOrEqual(t, len(agents), 2)

	// Check that expected agents are present
	expectedAgents := map[string]bool{
		"llm-agent":   false,
		"claude-code": false,
	}

	for _, agent := range agents {
		if _, ok := expectedAgents[agent.Name()]; ok {
			expectedAgents[agent.Name()] = true
		}
	}

	for name, found := range expectedAgents {
		assert.True(t, found, "Expected builtin agent %s not found", name)
	}
}

func TestLLMAgent(t *testing.T) {
	agent := &LLMAgent{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "llm-agent", agent.Name())
	})

	t.Run("Description", func(t *testing.T) {
		desc := agent.Description()
		assert.NotEmpty(t, desc)
	})

	t.Run("RequiresModel", func(t *testing.T) {
		assert.True(t, agent.RequiresModel())
	})

	t.Run("ValidateEnvironment", func(t *testing.T) {
		err := agent.ValidateEnvironment()
		assert.NoError(t, err)
	})

	t.Run("GetDefaults requires model", func(t *testing.T) {
		spec, err := agent.GetDefaults("")
		assert.Error(t, err)
		assert.Nil(t, spec)
		assert.Contains(t, err.Error(), "model is required")
	})

	t.Run("GetDefaults with valid model", func(t *testing.T) {
		spec, err := agent.GetDefaults("openai:gpt-4")
		require.NoError(t, err)
		require.NotNil(t, spec)

		assert.Equal(t, "llm-agent-openai:gpt-4", spec.Metadata.Name)

		require.NotNil(t, spec.Builtin)
		assert.Equal(t, "llm-agent", spec.Builtin.Type)
		assert.Equal(t, "openai:gpt-4", spec.Builtin.Model)
	})
}

func TestClaudeCodeAgent(t *testing.T) {
	agent := &ClaudeCodeAgent{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "claude-code", agent.Name())
	})

	t.Run("Description", func(t *testing.T) {
		desc := agent.Description()
		assert.NotEmpty(t, desc)
		assert.Contains(t, desc, "Claude")
	})

	t.Run("RequiresModel", func(t *testing.T) {
		assert.False(t, agent.RequiresModel())
	})

	t.Run("GetDefaults without model", func(t *testing.T) {
		spec, err := agent.GetDefaults("")
		require.NoError(t, err)
		require.NotNil(t, spec)

		assert.Equal(t, "claude-code", spec.Metadata.Name)
		require.NotNil(t, spec.AcpConfig)
		assert.Equal(t, &acpclient.AcpConfig{Cmd: "claude-agent-acp"}, spec.AcpConfig)
	})

	t.Run("GetDefaults with model (ignored)", func(t *testing.T) {
		spec, err := agent.GetDefaults("some-model")
		require.NoError(t, err)
		require.NotNil(t, spec)

		assert.Equal(t, "claude-code", spec.Metadata.Name)
		require.NotNil(t, spec.AcpConfig)
	})
}
