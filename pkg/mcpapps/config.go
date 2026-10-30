// Package mcpapps validates tool-associated MCP Apps resources.
package mcpapps

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const MIMEType = "text/html;profile=mcp-app"

type Config struct {
	// Host opts into UI capability negotiation and the model-facing Apps tool surface.
	Host bool `json:"host"`
}

// Assertion requires a retrievable HTML UI resource from every invocation of
// an exact server/tool pair. It does not execute or inspect the app's JavaScript.
type Assertion struct {
	Server string `json:"server"`
	Tool   string `json:"tool"`
}

func (a Assertion) Validate() error {
	if strings.TrimSpace(a.Server) == "" || strings.TrimSpace(a.Tool) == "" {
		return fmt.Errorf("server and tool are required")
	}
	return nil
}

func Capabilities() *mcp.ClientCapabilities {
	return &mcp.ClientCapabilities{
		// Setting explicit capabilities replaces the SDK defaults, including roots.
		RootsV2:    &mcp.RootCapabilities{ListChanged: true},
		Extensions: map[string]any{"io.modelcontextprotocol/ui": map[string]any{"mimeTypes": []string{MIMEType}}},
	}
}
func ui(meta mcp.Meta) map[string]any { value, _ := meta["ui"].(map[string]any); return value }
func ResourceURI(tool *mcp.Tool) string {
	if tool == nil {
		return ""
	}
	return resourceURI(tool.Meta)
}
func resourceURI(meta mcp.Meta) string {
	if uri, _ := ui(meta)["resourceUri"].(string); uri != "" {
		return uri
	}
	uri, _ := meta["ui/resourceUri"].(string)
	return uri
}
func ModelVisible(tool *mcp.Tool) bool {
	if tool == nil {
		return false
	}
	value, ok := ui(tool.Meta)["visibility"]
	if !ok {
		value, ok = tool.Meta["ui/visibility"]
	}
	if !ok {
		return true
	}
	switch data := value.(type) {
	case string:
		return data == "model"
	case []any:
		for _, v := range data {
			if v == "model" {
				return true
			}
		}
	case []string:
		for _, v := range data {
			if v == "model" {
				return true
			}
		}
	}
	return false
}

type contextKey int

const (
	hostKey contextKey = iota
	collectorKey
)

func WithHost(ctx context.Context) context.Context {
	return context.WithValue(ctx, hostKey, true)
}
func HostEnabled(ctx context.Context) bool { v, _ := ctx.Value(hostKey).(bool); return v }
func WithCollector(ctx context.Context, c *Collector) context.Context {
	return context.WithValue(ctx, collectorKey, c)
}
func FromContext(ctx context.Context) *Collector {
	c, _ := ctx.Value(collectorKey).(*Collector)
	return c
}
