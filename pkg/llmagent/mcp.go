package llmagent

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type McpClient interface {
	GetTools() []mcpsdk.Tool
	CallTool(ctx context.Context, name string, arguments map[string]any) (string, error)
	Close() error
}

type mcpClient struct {
	session atomic.Pointer[mcpsdk.ClientSession]
	mu      sync.RWMutex
	tools   []mcpsdk.Tool
}

func NewMcpClient(ctx context.Context, serverURL string, headers http.Header) (McpClient, error) {
	mc := &mcpClient{}

	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "mcpchecker-agent",
		Version: "1.0.0",
	}, &mcpsdk.ClientOptions{
		ToolListChangedHandler: func(ctx context.Context, tlcr *mcpsdk.ToolListChangedRequest) {
			_ = mc.reloadTools(ctx)
		},
	})

	transport := &mcpsdk.StreamableClientTransport{
		Endpoint: serverURL,
	}

	if len(headers) > 0 {
		transport.HTTPClient = &http.Client{
			Transport: mcpclient.NewHeaderRoundTripper(headers, nil),
		}
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MCP server: %w", err)
	}

	mc.session.Store(session)
	err = mc.reloadTools(ctx)
	return mc, err
}

func (c *mcpClient) GetTools() []mcpsdk.Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]mcpsdk.Tool, len(c.tools))
	copy(out, c.tools)

	return out
}

func (c *mcpClient) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	session := c.session.Load()
	if session == nil {
		return "", fmt.Errorf("mcp client session not initialized")
	}
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return "", fmt.Errorf("failed to call tool %s: %w", name, err)
	}

	// UI metadata belongs to the client UI, not the model's tool response.
	// Keep the session result intact for other consumers.
	modelResult := *result
	modelResult.Meta = withoutUIMetadata(result.Meta)
	modelResult.Content = withoutUIContentMetadata(result.Content)
	// TODO: we should probably only marshal either the content or the structured content (not everything)
	resultBytes, err := json.Marshal(&modelResult)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tool result: %w", err)
	}

	return string(resultBytes), nil
}

func withoutUIMetadata(meta mcpsdk.Meta) mcpsdk.Meta {
	copyMeta := maps.Clone(meta)
	for key := range copyMeta {
		if key == "ui" || strings.HasPrefix(key, "ui/") {
			delete(copyMeta, key)
		}
	}
	return copyMeta
}

// Copy each block and every metadata map we filter. Payloads and annotations
// remain shared because they are not modified.
func withoutUIContentMetadata(content []mcpsdk.Content) []mcpsdk.Content {
	if content == nil {
		return nil
	}
	out := make([]mcpsdk.Content, len(content))
	for i, block := range content {
		switch block := block.(type) {
		case *mcpsdk.TextContent:
			copyBlock := *block
			copyBlock.Meta = withoutUIMetadata(block.Meta)
			out[i] = &copyBlock
		case *mcpsdk.ImageContent:
			copyBlock := *block
			copyBlock.Meta = withoutUIMetadata(block.Meta)
			out[i] = &copyBlock
		case *mcpsdk.AudioContent:
			copyBlock := *block
			copyBlock.Meta = withoutUIMetadata(block.Meta)
			out[i] = &copyBlock
		case *mcpsdk.ResourceLink:
			copyBlock := *block
			copyBlock.Meta = withoutUIMetadata(block.Meta)
			out[i] = &copyBlock
		case *mcpsdk.EmbeddedResource:
			copyBlock := *block
			copyBlock.Meta = withoutUIMetadata(block.Meta)
			if block.Resource != nil {
				resource := *block.Resource
				resource.Meta = withoutUIMetadata(resource.Meta)
				copyBlock.Resource = &resource
			}
			out[i] = &copyBlock
		default:
			out[i] = block
		}
	}
	return out
}

func (c *mcpClient) Close() error {
	session := c.session.Load()
	if session == nil {
		return nil
	}
	return session.Close()
}

func (c *mcpClient) reloadTools(ctx context.Context) error {
	session := c.session.Load()
	if session == nil {
		return nil
	}
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to list tools: %w", err)
	}

	tools := make([]mcpsdk.Tool, 0, len(result.Tools))
	for _, tool := range result.Tools {
		if tool != nil {
			tools = append(tools, *tool)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.tools = tools

	return nil
}
