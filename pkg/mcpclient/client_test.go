package mcpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newClientWithTools(t *testing.T, cfg *ServerConfig) *Client {
	t.Helper()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	for _, name := range []string{"allowed-tool", "other-tool"} {
		server.AddTool(&mcp.Tool{
			Name:        name,
			Description: "A test tool",
			InputSchema: map[string]any{"type": "object"},
		}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})
	}

	serverCtx, stopServer := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Run(serverCtx, serverTransport)
	}()
	t.Cleanup(func() {
		stopServer()
		<-serverDone
	})

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := mcpClient.Connect(context.Background(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	return &Client{ClientSession: session, cfg: cfg}
}

func TestGetAllowedToolsEnableAllTools(t *testing.T) {
	client := newClientWithTools(t, &ServerConfig{EnableAllTools: true})

	tools, err := client.GetAllowedTools(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"allowed-tool", "other-tool"}, toolNames(tools))
}

func TestGetAllowedToolsAlwaysAllow(t *testing.T) {
	client := newClientWithTools(t, &ServerConfig{AlwaysAllow: []string{"other-tool"}})

	tools, err := client.GetAllowedTools(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"other-tool"}, toolNames(tools))
}

func TestGetAllowedToolsReturnsListError(t *testing.T) {
	client := newClientWithTools(t, &ServerConfig{EnableAllTools: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tools, err := client.GetAllowedTools(ctx)
	require.Error(t, err)
	assert.Nil(t, tools)
}

func TestConnectDefaultInputHandling(t *testing.T) {
	upstream := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	upstream.AddTool(&mcp.Tool{Name: "confirm", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				InputRequests: mcp.InputRequestMap{
					"confirmation": &mcp.ElicitParams{
						Message:         "Confirm?",
						RequestedSchema: &jsonschema.Schema{Type: "object"},
					},
				},
				RequestState: "opaque-state",
			}, nil
		})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return upstream
	}, &mcp.StreamableHTTPOptions{Stateless: true}))
	t.Cleanup(httpServer.Close)

	client, err := Connect(t.Context(), &ServerConfig{Type: TransportTypeHttp, URL: httpServer.URL})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	// The default SDK client tries to fulfill the request without an elicitation handler.
	_, err = client.CallTool(t.Context(), &mcp.CallToolParams{Name: "confirm"})
	require.Error(t, err)
}

func toolNames(tools []*mcp.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}
