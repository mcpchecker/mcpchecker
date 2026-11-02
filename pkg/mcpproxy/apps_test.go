package mcpproxy_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcpchecker/mcpchecker/pkg/mcpapps"
	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Covers negotiation, an unlisted UI resource, metadata separation, and
// redacted recording with a plain MCP client that has no Apps support.
func TestMCPApps(t *testing.T) {
	collector := mcpapps.NewCollector([]mcpapps.Assertion{{Server: "kubernetes", Tool: "namespaces_list"}})
	ctx := mcpapps.WithCollector(mcpapps.WithHost(t.Context()), collector)
	const html = "<html><body>PRIVATE_UI_HTML</body></html>"
	const uri = "ui://namespaces/view.html"
	const resultURI = "ui://namespaces/this-invocation.html"
	contentMeta := mcp.Meta{"ui": map[string]any{"resourceUri": resultURI}, "ui/legacy": "content-ui", "example.com/trace": "content-trace"}
	content := []mcp.Content{
		&mcp.TextContent{Text: "Namespaces retrieved", Meta: contentMeta},
		&mcp.ImageContent{MIMEType: "image/png", Data: []byte("image"), Meta: contentMeta},
		&mcp.AudioContent{MIMEType: "audio/wav", Data: []byte("audio"), Meta: contentMeta},
		&mcp.ResourceLink{URI: "test://static", Name: "static", Meta: contentMeta},
		&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: "test://embedded", Text: "embedded data", Meta: contentMeta}, Meta: contentMeta},
		&mcp.ToolUseContent{ID: "nested-call", Name: "view", Meta: contentMeta},
		&mcp.ToolResultContent{ToolUseID: "nested-call", Content: []mcp.Content{&mcp.TextContent{Text: "nested data", Meta: contentMeta}}, Meta: contentMeta},
	}
	// Compare complete content to catch payload loss as well as filtering or mutation.
	checkContent := func(t *testing.T, got []mcp.Content, filtered bool) {
		t.Helper()
		original, err := json.Marshal(content)
		require.NoError(t, err)
		var expected any
		require.NoError(t, json.Unmarshal(original, &expected))
		if filtered {
			var clean func(any)
			clean = func(value any) {
				switch value := value.(type) {
				case []any:
					for _, item := range value {
						clean(item)
					}
				case map[string]any:
					if _, ok := value["_meta"]; ok {
						value["_meta"] = map[string]any{"example.com/trace": "content-trace"}
					}
					for _, item := range value {
						clean(item)
					}
				}
			}
			clean(expected)
		}
		want, err := json.Marshal(expected)
		require.NoError(t, err)
		actual, err := json.Marshal(got)
		require.NoError(t, err)
		require.JSONEq(t, string(want), string(actual))
	}
	var calls, reads atomic.Int32
	var negotiated atomic.Bool
	upstream := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{
		Resources:  &mcp.ResourceCapabilities{},
		Extensions: map[string]any{"example.com/custom": map[string]any{}},
	}})
	upstream.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/list" {
				caps := req.(*mcp.ListToolsRequest).ClientCapabilities()
				if caps != nil {
					assert.NotNil(t, caps.RootsV2, "Apps must preserve the SDK's default roots capability")
					if caps.RootsV2 != nil {
						assert.True(t, caps.RootsV2.ListChanged)
					}
					_, ok := caps.Extensions["io.modelcontextprotocol/ui"]
					negotiated.Store(ok)
				}
			}
			if method == "resources/read" {
				rr := req.(*mcp.ReadResourceRequest)
				assert.Nil(t, rr.Params.Meta["mcpchecker/host-ui-read"])
				if rr.Params.URI == resultURI {
					reads.Add(1)
					return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: resultURI, MIMEType: mcpapps.MIMEType, Text: html}}}, nil
				}
			}
			return next(ctx, method, req)
		}
	})
	upstream.AddTool(&mcp.Tool{Name: "namespaces_list", InputSchema: map[string]any{"type": "object"}, Meta: mcp.Meta{"ui": map[string]any{"resourceUri": uri}}}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls.Add(1)
		return &mcp.CallToolResult{Content: content, StructuredContent: map[string]any{"items": []any{map[string]any{"name": "payments", "status": "Active"}}}, Meta: mcp.Meta{
			"ui": map[string]any{"resourceUri": resultURI, "private": "for-the-ui"}, "ui/legacy": "for-the-ui", "example.com/trace": "server-trace", "progressToken": "progress-1",
		}}, nil
	})
	upstream.AddTool(&mcp.Tool{Name: "app_only", InputSchema: map[string]any{"type": "object"}, Meta: mcp.Meta{"ui": map[string]any{"visibility": []string{"app"}}}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	readAgentResource := func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: "agent resource data"}}}, nil
	}
	upstream.AddResource(&mcp.Resource{Name: "static", URI: "test://static"}, readAgentResource)
	upstream.AddResourceTemplate(&mcp.ResourceTemplate{Name: "template", URITemplate: "test://items/{id}"}, readAgentResource)
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return upstream }, &mcp.StreamableHTTPOptions{Stateless: true}))
	defer httpServer.Close()
	client, err := mcpclient.Connect(ctx, &mcpclient.ServerConfig{Type: mcpclient.TransportTypeHttp, URL: httpServer.URL, EnableAllTools: true})
	require.NoError(t, err)
	defer client.Close()
	proxy, err := mcpproxy.NewProxyServerForClient(ctx, "kubernetes", client)
	require.NoError(t, err)
	go func() { _ = proxy.Run(ctx) }()
	require.NoError(t, proxy.WaitReady(ctx))
	require.True(t, negotiated.Load(), "upstream discovery must see Apps capability")
	defer proxy.Close()
	config, err := proxy.GetConfig()
	require.NoError(t, err)
	// No UI extension or collector is supplied to the agent client.
	agent, err := mcp.NewClient(&mcp.Implementation{Name: "plain-agent", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: config.URL}, nil)
	require.NoError(t, err)
	defer agent.Close()
	require.Empty(t, agent.InitializeResult().Capabilities.Extensions, "the proxy must not advertise unimplemented upstream extensions")
	tools, err := agent.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, tools.Tools, 1, "app-only tools must not be offered to the model")
	result, err := agent.CallTool(ctx, &mcp.CallToolParams{Name: "namespaces_list", Arguments: map[string]any{"cluster": "test"}})
	require.NoError(t, err)
	require.NotContains(t, result.Meta, "ui")
	require.NotContains(t, result.Meta, "ui/legacy")
	require.Equal(t, "server-trace", result.Meta["example.com/trace"])
	require.Equal(t, "progress-1", result.Meta["progressToken"])
	checkContent(t, result.Content, true)
	visible, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(visible), "<html>")
	observations := collector.Observations()
	require.Len(t, observations, 1)
	require.Empty(t, observations[0].Code)
	require.Equal(t, resultURI, observations[0].URI)
	require.Equal(t, mcpapps.MIMEType, observations[0].MIMEType)
	require.EqualValues(t, 1, calls.Load(), "the tool must never be replayed for UI validation")
	require.EqualValues(t, 1, reads.Load())
	history := proxy.GetCallHistory()
	require.Len(t, history.ToolCalls, 1)
	require.Equal(t, "for-the-ui", history.ToolCalls[0].Result.Meta["ui/legacy"], "filtering must not mutate recorded metadata")
	checkContent(t, history.ToolCalls[0].Result.Content, false)
	require.Len(t, history.ResourceReads, 1)
	read := history.ResourceReads[0]
	require.True(t, read.HostUIRead)
	require.Equal(t, observations[0].CallID, read.AppCallID)
	require.Nil(t, read.Result)
	require.Equal(t, len(html), read.SizeBytes)
	saved, err := json.Marshal(history)
	require.NoError(t, err)
	require.NotContains(t, string(saved), "PRIVATE_UI_HTML")
	require.Empty(t, mcpproxy.ComputeCallHistoryTokens(&history))
	require.Nil(t, read.Tokens)

	// Agent-supplied metadata cannot hide reads through either resource handler.
	for _, agentURI := range []string{"test://static", "test://items/42"} {
		_, err := agent.ReadResource(ctx, &mcp.ReadResourceParams{URI: agentURI, Meta: mcp.Meta{"mcpchecker/host-ui-read": "spoofed"}})
		require.NoError(t, err)
	}
	history = proxy.GetCallHistory()
	require.Len(t, history.ResourceReads, 3)
	require.Empty(t, mcpproxy.ComputeCallHistoryTokens(&history))
	for _, agentRead := range history.ResourceReads[1:] {
		require.False(t, agentRead.HostUIRead)
		require.Empty(t, agentRead.AppCallID)
		require.NotNil(t, agentRead.Result)
		require.NotNil(t, agentRead.Tokens)
		require.Positive(t, agentRead.Tokens.OutputTokens)
	}

	// Cover the other combinations independently of the host + assertions case above.
	for _, mode := range []struct {
		name             string
		host, assertions bool
	}{
		{name: "host only", host: true},
		{name: "assertions only", assertions: true},
		{name: "neither"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			ctx := t.Context()
			if mode.host {
				ctx = mcpapps.WithHost(ctx)
			}
			var collector *mcpapps.Collector
			if mode.assertions {
				collector = mcpapps.NewCollector([]mcpapps.Assertion{{Server: "kubernetes", Tool: "namespaces_list"}})
				ctx = mcpapps.WithCollector(ctx, collector)
			}
			beforeReads := reads.Load()
			client, err := mcpclient.Connect(ctx, &mcpclient.ServerConfig{Type: mcpclient.TransportTypeHttp, URL: httpServer.URL, EnableAllTools: true})
			require.NoError(t, err)
			defer client.Close()
			proxy, err := mcpproxy.NewProxyServerForClient(ctx, "kubernetes", client)
			require.NoError(t, err)
			go func() { _ = proxy.Run(ctx) }()
			require.NoError(t, proxy.WaitReady(ctx))
			defer proxy.Close()
			require.Equal(t, mode.host, negotiated.Load())
			allowed, err := client.GetAllowedTools(ctx)
			require.NoError(t, err)
			config, err := proxy.GetConfig()
			require.NoError(t, err)
			agent, err := mcp.NewClient(&mcp.Implementation{Name: "plain-agent", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: config.URL}, nil)
			require.NoError(t, err)
			defer agent.Close()
			require.Empty(t, agent.InitializeResult().Capabilities.Extensions)
			tools, err := agent.ListTools(ctx, nil)
			require.NoError(t, err)
			wantTools := 2
			if mode.host {
				wantTools = 1
			}
			require.Len(t, tools.Tools, wantTools)
			require.Len(t, allowed, wantTools, "capability inventory must match the proxy tool surface")
			result, err := agent.CallTool(ctx, &mcp.CallToolParams{Name: "namespaces_list", Arguments: map[string]any{}})
			require.NoError(t, err)
			_, hasUI := result.Meta["ui"]
			_, hasLegacyUI := result.Meta["ui/legacy"]
			require.Equal(t, !mode.host, hasUI)
			require.Equal(t, !mode.host, hasLegacyUI)
			require.Equal(t, "server-trace", result.Meta["example.com/trace"])
			checkContent(t, result.Content, mode.host)
			checkContent(t, proxy.GetCallHistory().ToolCalls[0].Result.Content, false)
			if mode.assertions {
				observations := collector.Finish()
				require.Len(t, observations, 1)
				require.Empty(t, observations[0].Code)
				require.Equal(t, resultURI, observations[0].URI)
				require.Equal(t, beforeReads+1, reads.Load())
			} else {
				require.Equal(t, beforeReads, reads.Load(), "host emulation alone must not read UI resources")
				require.Empty(t, proxy.GetCallHistory().ResourceReads)
			}
		})
	}
}

func TestMCPAppsCancellation(t *testing.T) {
	for _, taskCancellation := range []bool{true, false} {
		t.Run(map[bool]string{true: "task cancellation", false: "finish waits for validation"}[taskCancellation], func(t *testing.T) {
			collector := mcpapps.NewCollector([]mcpapps.Assertion{{Server: "fixture", Tool: "view"}})
			ctx, cancel := context.WithCancel(mcpapps.WithCollector(t.Context(), collector))
			defer cancel()
			started, canceled := make(chan struct{}), make(chan struct{})
			upstream := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
			upstream.AddTool(&mcp.Tool{Name: "view", InputSchema: map[string]any{"type": "object"}, Meta: mcp.Meta{"ui": map[string]any{"resourceUri": "ui://blocked"}}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{}, nil
			})
			upstream.AddResource(&mcp.Resource{Name: "blocked", URI: "ui://blocked"}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				close(started)
				<-ctx.Done()
				close(canceled)
				return nil, ctx.Err()
			})
			httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return upstream }, &mcp.StreamableHTTPOptions{Stateless: true, PropagateRequestCancellation: true}))
			defer httpServer.Close()
			// Keep the shared upstream connection and agent alive when only the task is canceled.
			client, err := mcpclient.Connect(t.Context(), &mcpclient.ServerConfig{Type: mcpclient.TransportTypeHttp, URL: httpServer.URL})
			require.NoError(t, err)
			defer client.Close()
			proxy, err := mcpproxy.NewProxyServerForClient(ctx, "fixture", client)
			require.NoError(t, err)
			go func() { _ = proxy.Run(ctx) }()
			require.NoError(t, proxy.WaitReady(ctx))
			defer proxy.Close()
			config, err := proxy.GetConfig()
			require.NoError(t, err)
			agent, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil).Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: config.URL}, nil)
			require.NoError(t, err)
			defer agent.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = agent.CallTool(t.Context(), &mcp.CallToolParams{Name: "view", Arguments: map[string]any{}})
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("UI read did not start")
			}
			if taskCancellation {
				cancel()
				select {
				case <-canceled:
				case <-time.After(3 * time.Second):
					t.Fatal("task cancellation did not reach the UI read")
				}
			}
			observations := collector.Finish()
			require.Len(t, observations, 1, "final snapshot must include the interrupted invocation")
			require.Equal(t, "ui_resource_read_failed", observations[0].Code)
			require.Contains(t, observations[0].Detail, "context canceled")
			history := proxy.GetCallHistory()
			require.Len(t, history.ResourceReads, 1)
			require.Contains(t, history.ResourceReads[0].Error, "context canceled")
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("agent call did not finish")
			}
		})
	}
}
