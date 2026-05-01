package acpclient

import (
	"context"
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
)

// mockServer implements mcpproxy.Server for testing
type mockServer struct {
	name         string
	allowedTools []*mcp.Tool
}

func (m *mockServer) Run(_ context.Context) error                   { return nil }
func (m *mockServer) GetConfig() (*mcpclient.ServerConfig, error)   { return nil, nil }
func (m *mockServer) GetName() string                               { return m.name }
func (m *mockServer) GetAllowedTools(_ context.Context) []*mcp.Tool { return m.allowedTools }
func (m *mockServer) GetInstructions() string                       { return "" }
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

func TestSession_IsAllowedToolCall(t *testing.T) {
	tt := map[string]struct {
		allowedTools []*mcp.Tool
		call         acp.ToolCallUpdate
		expected     bool
	}{
		"allowed when tool title matches": {
			allowedTools: []*mcp.Tool{{Name: "read_file", Title: "Read File"}},
			call: acp.ToolCallUpdate{
				ToolCallId: "call-1",
				Title:      ptr("Read File"),
			},
			expected: true,
		},
		"not allowed when tool title does not match": {
			allowedTools: []*mcp.Tool{{Name: "read_file", Title: "Read File"}},
			call: acp.ToolCallUpdate{
				ToolCallId: "call-1",
				Title:      ptr("Delete File"),
			},
			expected: false,
		},
		"not allowed when no tools configured": {
			allowedTools: []*mcp.Tool{},
			call: acp.ToolCallUpdate{
				ToolCallId: "call-1",
				Title:      ptr("Read File"),
			},
			expected: false,
		},
		"not allowed when title is nil and no prior update": {
			allowedTools: []*mcp.Tool{{Name: "read_file", Title: "Read File"}},
			call: acp.ToolCallUpdate{
				ToolCallId: "call-1",
				Title:      nil,
			},
			expected: false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			mgr := &mockServerManager{
				servers: []mcpproxy.Server{
					&mockServer{name: "test-server", allowedTools: tc.allowedTools},
				},
			}
			s := NewSession(mgr, "")

			result := s.isAllowedToolCall(context.Background(), tc.call)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSession_IsAllowedToolCall_WithPriorUpdate(t *testing.T) {
	// Test that IsAllowedToolCall uses title from prior update when call.Title is nil
	mgr := &mockServerManager{
		servers: []mcpproxy.Server{
			&mockServer{name: "test-server", allowedTools: []*mcp.Tool{{Name: "read_file", Title: "Read File"}}},
		},
	}
	s := NewSession(mgr, "")

	// First, store a tool call with a title
	s.toolCallStatuses["call-1"] = &acp.SessionToolCallUpdate{
		ToolCallId: "call-1",
		Title:      ptr("Read File"),
	}

	// Now check with a call that has no title - should use stored title
	call := acp.ToolCallUpdate{
		ToolCallId: "call-1",
		Title:      nil,
	}

	result := s.isAllowedToolCall(context.Background(), call)
	assert.True(t, result)
}

func TestSession_ToolCallStatusUpdateLocked(t *testing.T) {
	tt := map[string]struct {
		initial  *acp.SessionToolCallUpdate
		update   *acp.SessionToolCallUpdate
		validate func(t *testing.T, result *acp.SessionToolCallUpdate)
	}{
		"new tool call is stored": {
			initial: nil,
			update: &acp.SessionToolCallUpdate{
				ToolCallId: "call-1",
				Title:      ptr("Read File"),
			},
			validate: func(t *testing.T, result *acp.SessionToolCallUpdate) {
				assert.Equal(t, acp.ToolCallId("call-1"), result.ToolCallId)
				assert.Equal(t, "Read File", *result.Title)
			},
		},
		"update merges with existing": {
			initial: &acp.SessionToolCallUpdate{
				ToolCallId: "call-1",
				Title:      ptr("Read File"),
				RawInput:   "input data",
			},
			update: &acp.SessionToolCallUpdate{
				ToolCallId: "call-1",
				RawOutput:  "output data",
			},
			validate: func(t *testing.T, result *acp.SessionToolCallUpdate) {
				assert.Equal(t, "Read File", *result.Title)
				assert.Equal(t, "input data", result.RawInput)
				assert.Equal(t, "output data", result.RawOutput)
			},
		},
		"update overwrites existing fields": {
			initial: &acp.SessionToolCallUpdate{
				ToolCallId: "call-1",
				Title:      ptr("Read File"),
			},
			update: &acp.SessionToolCallUpdate{
				ToolCallId: "call-1",
				Title:      ptr("Write File"),
			},
			validate: func(t *testing.T, result *acp.SessionToolCallUpdate) {
				assert.Equal(t, "Write File", *result.Title)
			},
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			mgr := &mockServerManager{}
			s := NewSession(mgr, "")

			if tc.initial != nil {
				s.toolCallStatuses[tc.initial.ToolCallId] = tc.initial
			}

			s.toolCallStatusUpdateLocked(tc.update)

			result := s.toolCallStatuses[tc.update.ToolCallId]
			tc.validate(t, result)
		})
	}
}

func TestToolTitleProbablyMatches(t *testing.T) {
	tt := map[string]struct {
		acpToolTitle  string
		mcpToolTitle  string
		mcpToolName   string
		mcpServerName string
		expected      bool
	}{
		"exact title match": {
			acpToolTitle:  "Read File",
			mcpToolTitle:  "Read File",
			mcpToolName:   "read_file",
			mcpServerName: "filesystem",
			expected:      true,
		},
		"matches tool name exactly": {
			acpToolTitle:  "read_file",
			mcpToolTitle:  "Read File",
			mcpToolName:   "read_file",
			mcpServerName: "filesystem",
			expected:      true,
		},
		"contains mcp and tool name": {
			acpToolTitle:  "mcp__filesystem__read_file",
			mcpToolTitle:  "Read File",
			mcpToolName:   "read_file",
			mcpServerName: "filesystem",
			expected:      true,
		},
		"contains server name and tool name": {
			acpToolTitle:  "filesystem:read_file",
			mcpToolTitle:  "Read File",
			mcpToolName:   "read_file",
			mcpServerName: "filesystem",
			expected:      true,
		},
		"contains mcp prefix with tool name": {
			acpToolTitle:  "mcp_read_file",
			mcpToolTitle:  "Read File",
			mcpToolName:   "read_file",
			mcpServerName: "other_server",
			expected:      true,
		},
		"no match - different title": {
			acpToolTitle:  "Delete File",
			mcpToolTitle:  "Read File",
			mcpToolName:   "read_file",
			mcpServerName: "filesystem",
			expected:      false,
		},
		"no match - partial tool name without mcp or server": {
			acpToolTitle:  "some_read_file_action",
			mcpToolTitle:  "Read File",
			mcpToolName:   "read_file",
			mcpServerName: "filesystem",
			expected:      false,
		},
		"no match - empty acp title": {
			acpToolTitle:  "",
			mcpToolTitle:  "Read File",
			mcpToolName:   "read_file",
			mcpServerName: "filesystem",
			expected:      false,
		},
		"server name contains tool name": {
			acpToolTitle:  "kubernetes_kubectl_apply",
			mcpToolTitle:  "Apply Manifest",
			mcpToolName:   "kubectl_apply",
			mcpServerName: "kubernetes",
			expected:      true,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			result := toolTitleProbablyMatches(tc.acpToolTitle, tc.mcpToolTitle, tc.mcpToolName, tc.mcpServerName)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSession_IsAllowedToolCall_FuzzyMatching(t *testing.T) {
	tt := map[string]struct {
		serverName   string
		allowedTools []*mcp.Tool
		callTitle    string
		expected     bool
	}{
		"matches via tool name": {
			serverName:   "filesystem",
			allowedTools: []*mcp.Tool{{Name: "read_file", Title: "Read File"}},
			callTitle:    "read_file",
			expected:     true,
		},
		"matches via mcp prefix pattern": {
			serverName:   "filesystem",
			allowedTools: []*mcp.Tool{{Name: "read_file", Title: "Read File"}},
			callTitle:    "mcp__filesystem__read_file",
			expected:     true,
		},
		"matches via server name pattern": {
			serverName:   "kubernetes",
			allowedTools: []*mcp.Tool{{Name: "kubectl_apply", Title: "Apply Manifest"}},
			callTitle:    "kubernetes:kubectl_apply",
			expected:     true,
		},
		"no match for unrelated tool": {
			serverName:   "filesystem",
			allowedTools: []*mcp.Tool{{Name: "read_file", Title: "Read File"}},
			callTitle:    "mcp__other__delete_file",
			expected:     false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			mgr := &mockServerManager{
				servers: []mcpproxy.Server{
					&mockServer{name: tc.serverName, allowedTools: tc.allowedTools},
				},
			}
			s := NewSession(mgr, "")

			call := acp.ToolCallUpdate{
				ToolCallId: "call-1",
				Title:      ptr(tc.callTitle),
			}

			result := s.isAllowedToolCall(context.Background(), call)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func ptr[T any](v T) *T {
	return &v
}
