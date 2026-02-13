package agent

import (
	"context"
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/mcpchecker/mcpchecker/pkg/acpclient"
	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAcpRunner(t *testing.T) {
	cfg := &acpclient.AcpConfig{
		Cmd:  "test-cmd",
		Args: []string{"--arg1", "--arg2"},
	}

	runner := NewAcpRunner(cfg, "test-agent")

	require.NotNil(t, runner)
	assert.Equal(t, "test-agent", runner.AgentName())
}

func TestAcpRunner_AgentName(t *testing.T) {
	tt := map[string]struct {
		name     string
		expected string
	}{
		"simple name": {
			name:     "my-agent",
			expected: "my-agent",
		},
		"empty name": {
			name:     "",
			expected: "",
		},
		"name with special chars": {
			name:     "agent-v1.0",
			expected: "agent-v1.0",
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			cfg := &acpclient.AcpConfig{Cmd: "test"}
			runner := NewAcpRunner(cfg, tc.name)
			assert.Equal(t, tc.expected, runner.AgentName())
		})
	}
}

func TestAcpRunner_WithMcpServerInfo(t *testing.T) {
	cfg := &acpclient.AcpConfig{
		Cmd:  "test-cmd",
		Args: []string{"--arg1"},
	}
	originalRunner := NewAcpRunner(cfg, "original-agent")

	mgr := &mockServerManager{
		servers: []mcpproxy.Server{
			&mockServer{name: "test-server"},
		},
	}

	newRunner := originalRunner.WithMcpServerInfo(mgr)

	// Verify new runner is returned
	require.NotNil(t, newRunner)
	assert.NotSame(t, originalRunner, newRunner)

	// Verify new runner has same name
	assert.Equal(t, "original-agent", newRunner.AgentName())

	// Verify original runner is unchanged (mcpServers should still be nil)
	acpOriginal, ok := originalRunner.(*acpRunner)
	require.True(t, ok)
	assert.Nil(t, acpOriginal.mcpServers)

	// Verify new runner has the server manager set
	acpNew, ok := newRunner.(*acpRunner)
	require.True(t, ok)
	assert.NotNil(t, acpNew.mcpServers)
	assert.Equal(t, mgr, acpNew.mcpServers)
}

func TestAcpRunnerResult_GetOutput(t *testing.T) {
	tt := map[string]struct {
		updates       []acp.SessionUpdate
		checkContains string
		checkEmpty    bool
	}{
		"empty updates returns no output message": {
			updates:       []acp.SessionUpdate{},
			checkContains: "got no output from acp agent",
		},
		"nil updates returns no output message": {
			updates:       nil,
			checkContains: "got no output from acp agent",
		},
		"valid updates marshal to JSON": {
			updates: []acp.SessionUpdate{
				acp.UpdateAgentMessageText("Hello world"),
			},
			checkContains: "Hello world",
		},
		"multiple updates marshal to JSON array": {
			updates: []acp.SessionUpdate{
				acp.UpdateAgentMessageText("First"),
				acp.UpdateAgentMessageText("Second"),
			},
			checkContains: "Second",
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			result := &acpResult{
				updates: tc.updates,
			}

			output := result.GetOutput()
			if tc.checkContains != "" {
				assert.Contains(t, output, tc.checkContains)
			}
		})
	}
}

func TestAcpRunnerResult_GetOutput_WithAgentMessageChunk(t *testing.T) {
	// Test that updates with AgentMessageChunk marshal correctly
	result := &acpResult{
		updates: []acp.SessionUpdate{
			acp.UpdateAgentMessageText("Final message"),
		},
	}

	output := result.GetOutput()
	assert.Contains(t, output, "Final message")
	// Should be valid JSON
	assert.True(t, len(output) > 0)
}

// mockServer implements mcpproxy.Server for testing
type mockServer struct {
	name         string
	allowedTools []*mcp.Tool
}

func (m *mockServer) Run(_ context.Context) error                   { return nil }
func (m *mockServer) GetConfig() (*mcpclient.ServerConfig, error)   { return nil, nil }
func (m *mockServer) GetName() string                               { return m.name }
func (m *mockServer) GetAllowedTools(_ context.Context) []*mcp.Tool { return m.allowedTools }
func (m *mockServer) Close() error                                  { return nil }
func (m *mockServer) GetCallHistory() mcpproxy.CallHistory          { return mcpproxy.CallHistory{} }
func (m *mockServer) WaitReady(_ context.Context) error             { return nil }

// mockServerManager implements mcpproxy.ServerManager for testing
type mockServerManager struct {
	servers []mcpproxy.Server
}

func (m *mockServerManager) GetMcpServerFiles() ([]string, error)     { return nil, nil }
func (m *mockServerManager) GetMcpServers() []mcpproxy.Server         { return m.servers }
func (m *mockServerManager) Start(_ context.Context) error            { return nil }
func (m *mockServerManager) Close() error                             { return nil }
func (m *mockServerManager) GetAllCallHistory() *mcpproxy.CallHistory { return nil }
func (m *mockServerManager) GetCallHistoryForServer(_ string) (mcpproxy.CallHistory, bool) {
	return mcpproxy.CallHistory{}, false
}
