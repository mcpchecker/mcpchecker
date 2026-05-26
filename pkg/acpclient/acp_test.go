package acpclient

import (
	"context"
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_RequestPermission(t *testing.T) {
	tt := map[string]struct {
		sessions     map[acp.SessionId]*session
		params       acp.RequestPermissionRequest
		expectErr    bool
		errContains  string
		expectedOpt  string
	}{
		"allowed tool call selects allow always option": {
			sessions: map[acp.SessionId]*session{
				"session-1": newTestSession([]*mcp.Tool{{Name: "read_file", Title: "Read File"}}),
			},
			params: acp.RequestPermissionRequest{
				SessionId: "session-1",
				ToolCall: acp.ToolCallUpdate{
					ToolCallId: "call-1",
					Title:      ptr("Read File"),
				},
				Options: []acp.PermissionOption{
					{OptionId: "opt-once", Kind: acp.PermissionOptionKindAllowOnce},
					{OptionId: "opt-always", Kind: acp.PermissionOptionKindAllowAlways},
				},
			},
			expectedOpt: "opt-always",
		},
		"allowed tool call falls back to allow once if no always": {
			sessions: map[acp.SessionId]*session{
				"session-1": newTestSession([]*mcp.Tool{{Name: "read_file", Title: "Read File"}}),
			},
			params: acp.RequestPermissionRequest{
				SessionId: "session-1",
				ToolCall: acp.ToolCallUpdate{
					ToolCallId: "call-1",
					Title:      ptr("Read File"),
				},
				Options: []acp.PermissionOption{
					{OptionId: "opt-once", Kind: acp.PermissionOptionKindAllowOnce},
					{OptionId: "opt-reject", Kind: acp.PermissionOptionKindRejectOnce},
				},
			},
			expectedOpt: "opt-once",
		},
		"not allowed tool call selects reject always option": {
			sessions: map[acp.SessionId]*session{
				"session-1": newTestSession([]*mcp.Tool{{Name: "read_file", Title: "Read File"}}),
			},
			params: acp.RequestPermissionRequest{
				SessionId: "session-1",
				ToolCall: acp.ToolCallUpdate{
					ToolCallId: "call-1",
					Title:      ptr("Delete File"),
				},
				Options: []acp.PermissionOption{
					{OptionId: "opt-allow", Kind: acp.PermissionOptionKindAllowOnce},
					{OptionId: "opt-reject-once", Kind: acp.PermissionOptionKindRejectOnce},
					{OptionId: "opt-reject-always", Kind: acp.PermissionOptionKindRejectAlways},
				},
			},
			expectedOpt: "opt-reject-always",
		},
		"session not found returns error": {
			sessions: map[acp.SessionId]*session{},
			params: acp.RequestPermissionRequest{
				SessionId: "nonexistent",
				ToolCall: acp.ToolCallUpdate{
					ToolCallId: "call-1",
					Title:      ptr("Read File"),
				},
				Options: []acp.PermissionOption{
					{OptionId: "opt-1", Kind: acp.PermissionOptionKindAllowOnce},
				},
			},
			expectErr:   true,
			errContains: "no matching session",
		},
		"no options returns error": {
			sessions: map[acp.SessionId]*session{
				"session-1": newTestSession([]*mcp.Tool{}),
			},
			params: acp.RequestPermissionRequest{
				SessionId: "session-1",
				ToolCall: acp.ToolCallUpdate{
					ToolCallId: "call-1",
					Title:      ptr("Read File"),
				},
				Options: []acp.PermissionOption{},
			},
			expectErr:   true,
			errContains: "at least one option",
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			c := &client{
				sessions: tc.sessions,
			}

			resp, err := c.RequestPermission(context.Background(), tc.params)

			if tc.expectErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp.Outcome.Selected, "expected outcome to be Selected")
			assert.Equal(t, acp.PermissionOptionId(tc.expectedOpt), resp.Outcome.Selected.OptionId)
		})
	}
}

func TestClient_SessionUpdate(t *testing.T) {
	tt := map[string]struct {
		sessions    map[acp.SessionId]*session
		params      acp.SessionNotification
		expectErr   bool
		errContains string
	}{
		"updates existing session": {
			sessions: map[acp.SessionId]*session{
				"session-1": newTestSession([]*mcp.Tool{}),
			},
			params: acp.SessionNotification{
				SessionId: "session-1",
				Update:    acp.UpdateAgentMessageText("Hello"),
			},
			expectErr: false,
		},
		"session not found returns error": {
			sessions: map[acp.SessionId]*session{},
			params: acp.SessionNotification{
				SessionId: "nonexistent",
				Update:    acp.SessionUpdate{},
			},
			expectErr:   true,
			errContains: "no matching session",
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			c := &client{
				sessions: tc.sessions,
			}

			err := c.SessionUpdate(context.Background(), tc.params)

			if tc.expectErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			require.NoError(t, err)
			// Verify the update was stored
			session := tc.sessions[tc.params.SessionId]
			assert.Len(t, session.updates, 1)
		})
	}
}

// newTestSession creates a session with the given allowed tools for testing
func newTestSession(allowedTools []*mcp.Tool) *session {
	mgr := &mockServerManager{
		servers: []mcpproxy.Server{
			&mockServer{name: "test-server", allowedTools: allowedTools},
		},
	}
	return NewSession(mgr, "")
}
