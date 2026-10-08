package mcpproxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	newProtocolVersion = "2026-07-28"
	customMetaKey      = "example.com/trace"
	staticResourceURI  = "test://static"
	templateURI        = "test://items/{id}"
	templateItemURI    = "test://items/42"
)

type echoArgs struct {
	Message string `json:"message"`
}

// upstreamMetas records the request _meta that the upstream server received,
// by method.
type upstreamMetas struct {
	mu    sync.Mutex
	metas map[string][]mcp.Meta
}

func (u *upstreamMetas) record(method string, meta mcp.Meta) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.metas == nil {
		u.metas = map[string][]mcp.Meta{}
	}
	u.metas[method] = append(u.metas[method], maps.Clone(meta))
}

func (u *upstreamMetas) get(method string) []mcp.Meta {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.metas[method]
}

// assertForwardedMeta checks that the upstream server received exactly one
// request for method, that the agent's protocol version was not forwarded,
// and that the non-protocol keys in want were.
func assertForwardedMeta(t *testing.T, seen *upstreamMetas, method string, want map[string]any) {
	t.Helper()

	metas := seen.get(method)
	require.Len(t, metas, 1, "upstream requests for %s", method)
	meta := metas[0]
	assert.NotEqual(t, newProtocolVersion, meta[mcp.MetaKeyProtocolVersion],
		"agent's protocol version must not be forwarded upstream for %s", method)
	if info, ok := meta[mcp.MetaKeyClientInfo].(map[string]any); ok {
		assert.NotEqual(t, "agent", info["name"], "agent's client info must not be forwarded upstream for %s", method)
	}
	for k, v := range want {
		assert.Equal(t, v, meta[k], "_meta key %q forwarded upstream for %s", k, method)
	}
}

// startProxy starts an upstream MCP server with an "echo" tool, a "greet"
// prompt, a static resource, and a resource template on a stateful
// streamable HTTP handler (go-sdk's default, and what most MCP servers under
// test run). It connects to it the way mcpchecker does and runs a proxy in
// front of it. It returns the proxy once it is ready, and the _meta that the
// upstream server receives.
func startProxy(t *testing.T, ctx context.Context) (Server, *upstreamMetas) {
	t.Helper()

	seen := &upstreamMetas{}
	upstream := mcp.NewServer(&mcp.Implementation{Name: "upstream", Version: "1.0.0"}, nil)
	mcp.AddTool(upstream, &mcp.Tool{Name: "echo", Description: "echoes the message"},
		func(_ context.Context, req *mcp.CallToolRequest, args echoArgs) (*mcp.CallToolResult, any, error) {
			seen.record("tools/call", req.Params.Meta)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: args.Message}},
			}, nil, nil
		})
	upstream.AddPrompt(&mcp.Prompt{Name: "greet", Description: "a greeting"},
		func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			seen.record("prompts/get", req.Params.Meta)
			return &mcp.GetPromptResult{
				Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: "hello"}}},
			}, nil
		})
	upstream.AddResource(&mcp.Resource{Name: "static", URI: staticResourceURI},
		func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			seen.record("resources/read", req.Params.Meta)
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: "static content"}},
			}, nil
		})
	upstream.AddResourceTemplate(&mcp.ResourceTemplate{Name: "item", URITemplate: templateURI},
		func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			seen.record("resources/read (template)", req.Params.Meta)
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: "item content"}},
			}, nil
		})

	upstreamHTTP := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return upstream
	}, &mcp.StreamableHTTPOptions{}))
	t.Cleanup(upstreamHTTP.Close)

	client, err := mcpclient.Connect(ctx, &mcpclient.ServerConfig{
		Type: mcpclient.TransportTypeHttp,
		URL:  upstreamHTTP.URL,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	proxy, err := NewProxyServerForClient(ctx, "upstream", client)
	require.NoError(t, err)

	go func() { _ = proxy.Run(ctx) }()
	require.NoError(t, proxy.WaitReady(ctx))
	t.Cleanup(func() { _ = proxy.Close() })

	return proxy, seen
}

// connectAgent connects a go-sdk client to the proxy at protocolVersion.
func connectAgent(t *testing.T, ctx context.Context, proxy Server, protocolVersion string) *mcp.ClientSession {
	t.Helper()

	cfg, err := proxy.GetConfig()
	require.NoError(t, err)

	agent := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1.0.0"}, nil)
	session, err := agent.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: cfg.URL},
		&mcp.ClientSessionOptions{ProtocolVersion: protocolVersion})
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	require.Equal(t, protocolVersion, session.InitializeResult().ProtocolVersion)
	return session
}

// TestProxyServerProtocolVersions checks that agents can list and call tools
// through the proxy regardless of the MCP protocol version they negotiate.
// Protocol version 2026-07-28 is only accepted by go-sdk's streamable HTTP
// handler in stateless mode, see
// https://github.com/mcpchecker/mcpchecker/issues/345.
func TestProxyServerProtocolVersions(t *testing.T) {
	tests := map[string]struct {
		protocolVersion string
	}{
		"protocol version 2026-07-28": {
			protocolVersion: newProtocolVersion,
		},
		"protocol version 2025-11-25": {
			protocolVersion: "2025-11-25",
		},
		"protocol version 2025-06-18": {
			protocolVersion: "2025-06-18",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			proxy, seen := startProxy(t, ctx)
			session := connectAgent(t, ctx, proxy, tc.protocolVersion)

			tools, err := session.ListTools(ctx, &mcp.ListToolsParams{})
			require.NoError(t, err)
			require.Len(t, tools.Tools, 1)
			assert.Equal(t, "echo", tools.Tools[0].Name)

			res, err := session.CallTool(ctx, &mcp.CallToolParams{
				Meta:      mcp.Meta{customMetaKey: "abc"},
				Name:      "echo",
				Arguments: map[string]any{"message": "hello"},
			})
			require.NoError(t, err)
			require.False(t, res.IsError)
			require.Len(t, res.Content, 1)
			text, ok := res.Content[0].(*mcp.TextContent)
			require.True(t, ok)
			assert.Equal(t, "hello", text.Text)

			assertForwardedMeta(t, seen, "tools/call", map[string]any{customMetaKey: "abc"})

			history := proxy.GetCallHistory()
			require.Len(t, history.ToolCalls, 1)
			assert.Equal(t, "echo", history.ToolCalls[0].ToolName)
			assert.True(t, history.ToolCalls[0].Success)
		})
	}
}

// TestProxyServerForwardsPromptsAndResources checks prompt gets, resource
// reads, and resource template reads through the proxy at protocol version
// 2026-07-28 against a stateful upstream server.
func TestProxyServerForwardsPromptsAndResources(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	proxy, seen := startProxy(t, ctx)
	session := connectAgent(t, ctx, proxy, newProtocolVersion)

	prompt, err := session.GetPrompt(ctx, &mcp.GetPromptParams{
		Meta: mcp.Meta{customMetaKey: "prompt"},
		Name: "greet",
	})
	require.NoError(t, err)
	require.Len(t, prompt.Messages, 1)
	assert.Equal(t, "hello", prompt.Messages[0].Content.(*mcp.TextContent).Text)
	assertForwardedMeta(t, seen, "prompts/get", map[string]any{customMetaKey: "prompt"})

	static, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
		Meta: mcp.Meta{customMetaKey: "resource"},
		URI:  staticResourceURI,
	})
	require.NoError(t, err)
	require.Len(t, static.Contents, 1)
	assert.Equal(t, "static content", static.Contents[0].Text)
	assertForwardedMeta(t, seen, "resources/read", map[string]any{customMetaKey: "resource"})

	item, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
		Meta: mcp.Meta{customMetaKey: "template"},
		URI:  templateItemURI,
	})
	require.NoError(t, err)
	require.Len(t, item.Contents, 1)
	assert.Equal(t, "item content", item.Contents[0].Text)
	assertForwardedMeta(t, seen, "resources/read (template)", map[string]any{customMetaKey: "template"})

	history := proxy.GetCallHistory()
	require.Len(t, history.PromptGets, 1)
	assert.Equal(t, "greet", history.PromptGets[0].Name)
	assert.True(t, history.PromptGets[0].Success)
	require.Len(t, history.ResourceReads, 2)
	assert.Equal(t, staticResourceURI, history.ResourceReads[0].URI)
	assert.True(t, history.ResourceReads[0].Success)
	assert.Equal(t, templateItemURI, history.ResourceReads[1].URI)
	assert.True(t, history.ResourceReads[1].Success)
}

// postNewProtocolRequest sends a single JSON-RPC request the way a 2026-07-28
// client does: no initialize handshake, the version in the Mcp-Protocol-Version
// header, the matching _meta.protocolVersion on the request, and the
// Mcp-Method and Mcp-Name routing headers. extraMeta is added to _meta.
func postNewProtocolRequest(t *testing.T, ctx context.Context, url, method, name string, params, extraMeta map[string]any) map[string]any {
	t.Helper()

	meta := map[string]any{
		mcp.MetaKeyProtocolVersion:    newProtocolVersion,
		mcp.MetaKeyClientInfo:         map[string]any{"name": "agent", "version": "1.0.0"},
		mcp.MetaKeyClientCapabilities: map[string]any{},
	}
	maps.Copy(meta, extraMeta)
	params["_meta"] = meta
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", newProtocolVersion)
	req.Header.Set("Mcp-Method", method)
	if name != "" {
		req.Header.Set("Mcp-Name", name)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		require.FailNow(t, "unexpected HTTP status", "%s returned HTTP %d: %s", method, resp.StatusCode, raw)
	}

	// The response is either a JSON body or a single SSE "data:" event.
	var payload []byte
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if data, ok := strings.CutPrefix(scanner.Text(), "data:"); ok {
				payload = []byte(strings.TrimSpace(data))
				break
			}
		}
		require.NoError(t, scanner.Err())
	} else {
		payload, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
	}

	var msg map[string]any
	require.NoError(t, json.Unmarshal(payload, &msg), "response body: %s", payload)
	require.Nil(t, msg["error"], "unexpected JSON-RPC error: %v", msg["error"])

	result, ok := msg["result"].(map[string]any)
	require.True(t, ok, "missing result in response: %s", payload)
	return result
}

// TestProxyServerAcceptsNewProtocolRequests sends 2026-07-28 requests without
// first negotiating a version, as clients that do not fall back to an older
// protocol do. A stateful handler rejects these with
// CodeUnsupportedProtocolVersion (-32022).
func TestProxyServerAcceptsNewProtocolRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	proxy, seen := startProxy(t, ctx)
	cfg, err := proxy.GetConfig()
	require.NoError(t, err)

	listResult := postNewProtocolRequest(t, ctx, cfg.URL, "tools/list", "", map[string]any{}, nil)
	tools, ok := listResult["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	assert.Equal(t, "echo", tools[0].(map[string]any)["name"])

	callResult := postNewProtocolRequest(t, ctx, cfg.URL, "tools/call", "echo", map[string]any{
		"name":      "echo",
		"arguments": map[string]any{"message": "hello"},
	}, map[string]any{"progressToken": "tok-1", customMetaKey: "abc"})
	content, ok := callResult["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	assert.Equal(t, "hello", content[0].(map[string]any)["text"])

	assertForwardedMeta(t, seen, "tools/call", map[string]any{"progressToken": "tok-1", customMetaKey: "abc"})

	history := proxy.GetCallHistory()
	require.Len(t, history.ToolCalls, 1)
	assert.Equal(t, "echo", history.ToolCalls[0].ToolName)
}

func TestUpstreamMeta(t *testing.T) {
	tests := map[string]struct {
		input    mcp.Meta
		expected mcp.Meta
	}{
		"nil meta returns nil": {
			input:    nil,
			expected: nil,
		},
		"empty meta returns nil": {
			input:    mcp.Meta{},
			expected: nil,
		},
		"meta without protocol keys is forwarded unchanged": {
			input:    mcp.Meta{"progressToken": "abc", customMetaKey: "123"},
			expected: mcp.Meta{"progressToken": "abc", customMetaKey: "123"},
		},
		"agent protocol keys are removed": {
			input: mcp.Meta{
				mcp.MetaKeyProtocolVersion:    newProtocolVersion,
				mcp.MetaKeyClientInfo:         map[string]any{"name": "agent"},
				mcp.MetaKeyClientCapabilities: map[string]any{},
				"progressToken":               "abc",
			},
			expected: mcp.Meta{"progressToken": "abc"},
		},
		"meta with only protocol keys returns nil": {
			input:    mcp.Meta{mcp.MetaKeyProtocolVersion: newProtocolVersion},
			expected: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			original := maps.Clone(tc.input)
			assert.Equal(t, tc.expected, upstreamMeta(tc.input))
			assert.Equal(t, original, tc.input, "input must not be modified")
		})
	}
}
