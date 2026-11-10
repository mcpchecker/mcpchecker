package llmagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestMcpToolResponseExcludesUIMetadata(t *testing.T) {
	uiMeta := mcp.Meta{"ui": map[string]any{"resourceUri": "ui://UI_SENTINEL"}, "ui/legacy": "UI_SENTINEL"}
	result := &mcp.CallToolResult{
		Meta: mcp.Meta{"ui": uiMeta["ui"], "example.com/trace": "preserved"},
		Content: []mcp.Content{
			&mcp.TextContent{Text: "namespace data", Meta: uiMeta},
			&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: "test://embedded", Text: "embedded data", Meta: uiMeta}, Meta: uiMeta},
		},
		StructuredContent: map[string]any{"ui": "ordinary payload"},
	}
	upstream := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	upstream.AddTool(&mcp.Tool{Name: "view", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return result, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return upstream }, &mcp.StreamableHTTPOptions{Stateless: true}))
	defer httpServer.Close()
	client, err := NewMcpClient(t.Context(), httpServer.URL, nil)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()
	tools := ToolsFromMcpClients([]McpClient{client}, nil)
	response, err := tools[0].Run(t.Context(), fantasy.ToolCall{Name: "view", Input: "{}"})
	require.NoError(t, err)
	require.NotContains(t, response.Content, "UI_SENTINEL")

	var got mcp.CallToolResult
	require.NoError(t, json.Unmarshal([]byte(response.Content), &got))
	require.Equal(t, "preserved", got.Meta["example.com/trace"])
	require.Equal(t, result.StructuredContent, got.StructuredContent)
	require.Equal(t, []mcp.Content{
		&mcp.TextContent{Text: "namespace data"},
		&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: "test://embedded", Text: "embedded data"}},
	}, got.Content)
}

// TestMcpClientRaceCondition reproduces the race between client initialization
// and the ToolListChangedHandler callback firing before session is assigned.
func TestMcpClientRaceCondition(t *testing.T) {
	mc := &mcpClient{}
	ctx := t.Context()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = mc.reloadTools(ctx)
	}()
	wg.Wait()
}

func TestMcpClientConcurrentAccess(t *testing.T) {
	mc := &mcpClient{}
	ctx := t.Context()

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		_ = mc.reloadTools(ctx)
	}()
	go func() {
		defer wg.Done()
		_, _ = mc.CallTool(ctx, "test", nil)
	}()
	go func() {
		defer wg.Done()
		_ = mc.Close()
	}()
	wg.Wait()
}
