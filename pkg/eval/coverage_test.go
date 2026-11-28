package eval

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type coverageManager map[string]*mcpclient.Client

func (m coverageManager) Get(name string) (*mcpclient.Client, bool) { c, ok := m[name]; return c, ok }
func (m coverageManager) GetAll() map[string]*mcpclient.Client      { return m }
func (m coverageManager) Close(context.Context) error               { return nil }

func TestDiscoverToolCoverage(t *testing.T) {
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "server", Version: "1"}, nil)
	for _, name := range []string{"allowed", "not-allowed"} {
		server.AddTool(&mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})
	}
	serverCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(serverCtx, serverTransport) }()
	t.Cleanup(func() { cancel(); <-done })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1"}, nil).Connect(context.Background(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	config := &mcpclient.MCPConfig{MCPServers: map[string]*mcpclient.ServerConfig{"server": {AlwaysAllow: []string{"allowed"}}, "disabled": {Disabled: true}}}
	ctx := mcpclient.ManagerToContext(context.Background(), coverageManager{"server": {ClientSession: session}})
	got := discoverToolCoverage(ctx, config)
	require.Len(t, got, 1)
	assert.True(t, got[0].DiscoveryOK)
	assert.Equal(t, []ToolCoverage{{Name: "allowed"}, {Name: "not-allowed"}}, got[0].Tools)

	canceled, stop := context.WithCancel(ctx)
	stop()
	failed := discoverToolCoverage(canceled, config)
	require.Len(t, failed, 1)
	assert.False(t, failed[0].DiscoveryOK)
	assert.NotEmpty(t, failed[0].DiscoveryError)
	assert.Empty(t, failed[0].Tools)
}

func TestDiscoverServerWithoutTools(t *testing.T) {
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "empty", Version: "1"}, nil)
	serverCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(serverCtx, serverTransport) }()
	t.Cleanup(func() { cancel(); <-done })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1"}, nil).Connect(context.Background(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	ctx := mcpclient.ManagerToContext(context.Background(), coverageManager{"empty": {ClientSession: session}})
	config := &mcpclient.MCPConfig{MCPServers: map[string]*mcpclient.ServerConfig{"empty": {}}}
	got := discoverToolCoverage(ctx, config)
	require.Equal(t, []ServerToolCoverage{{ServerName: "empty", DiscoveryOK: true, Tools: []ToolCoverage{}}}, got)
}

func TestDiscoverToolCoverageTimeoutContinuesToNextServer(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	newClient := func(stall bool) *mcpclient.Client {
		clientTransport, serverTransport := mcp.NewInMemoryTransports()
		server := mcp.NewServer(&mcp.Implementation{Name: "server", Version: "1"}, nil)
		server.AddTool(&mcp.Tool{Name: "tool", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})
		if stall {
			server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
				return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
					if method == "tools/list" {
						select {
						case <-ctx.Done():
							return nil, ctx.Err()
						case <-release:
						}
					}
					return next(ctx, method, req)
				}
			})
		}
		serverCtx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- server.Run(serverCtx, serverTransport) }()
		t.Cleanup(func() { cancel(); <-done })
		session, err := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1"}, nil).Connect(context.Background(), clientTransport, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = session.Close() })
		return &mcpclient.Client{ClientSession: session}
	}
	manager := coverageManager{"a-stalled": newClient(true), "b-healthy": newClient(false)}
	config := &mcpclient.MCPConfig{MCPServers: map[string]*mcpclient.ServerConfig{"a-stalled": {}, "b-healthy": {}}}
	ctx := mcpclient.ManagerToContext(context.Background(), manager)
	got := discoverToolCoverageWithTimeout(ctx, config, 100*time.Millisecond)
	require.Len(t, got, 2)
	assert.False(t, got[0].DiscoveryOK)
	assert.Contains(t, got[0].DiscoveryError, "context deadline exceeded")
	assert.Empty(t, got[0].Tools)
	assert.True(t, got[1].DiscoveryOK)
	assert.Equal(t, []ToolCoverage{{Name: "tool"}}, got[1].Tools)
}

func TestAggregateToolCoverage(t *testing.T) {
	call := func(server, name string, success, isError bool) *mcpproxy.ToolCall {
		return &mcpproxy.ToolCall{CallRecord: mcpproxy.CallRecord{ServerName: server, Success: success}, ToolName: name, Result: &mcp.CallToolResult{IsError: isError}}
	}
	coverage := []ServerToolCoverage{
		{ServerName: "a", DiscoveryOK: true, Tools: []ToolCoverage{{Name: "unused"}, {Name: "shared"}}},
		{ServerName: "b", DiscoveryOK: true, Tools: []ToolCoverage{{Name: "shared"}}},
		{ServerName: "empty", DiscoveryOK: true, Tools: []ToolCoverage{}},
		{ServerName: "failed", DiscoveryError: "unavailable", Tools: []ToolCoverage{}},
	}
	results := []*EvalResult{
		{CallHistory: &mcpproxy.CallHistory{ToolCalls: []*mcpproxy.ToolCall{call("a", "shared", true, false), call("a", "shared", false, false), call("b", "shared", true, true), call("failed", "unknown", true, false)}}},
		{CallHistory: &mcpproxy.CallHistory{ToolCalls: []*mcpproxy.ToolCall{call("a", "shared", true, false), call("a", "new", true, false)}}},
		{CallHistory: nil},
	}
	got := aggregateToolCoverage(coverage, results)
	require.Len(t, got, 4)
	assert.Equal(t, ToolCoverage{Name: "unused"}, got[0].Tools[0])
	assert.Equal(t, ToolCoverage{Name: "shared", Called: true, SuccessCount: 2, FailureCount: 1}, got[0].Tools[1])
	assert.Equal(t, ToolCoverage{Name: "shared", Called: true, FailureCount: 1}, got[1].Tools[0])
	assert.Empty(t, got[2].Tools)
	assert.Equal(t, "unavailable", got[3].DiscoveryError)
	assert.Empty(t, got[3].Tools)

	data, err := json.Marshal(EvalOutput{Summary: &EvalSummary{}, Results: results, ToolCoverage: got})
	require.NoError(t, err)
	var output map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &output))
	assert.Contains(t, output, "summary")
	assert.Contains(t, output, "results")
	assert.Contains(t, output, "toolCoverage")
}
