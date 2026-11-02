package mcpapps

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestModelVisible(t *testing.T) {
	for _, test := range []struct {
		name string
		tool *mcp.Tool
		want bool
	}{
		{"nil tool", nil, false},
		{"default", &mcp.Tool{}, true},
		{"nested app list", &mcp.Tool{Meta: mcp.Meta{"ui": map[string]any{"visibility": []any{"app"}}}}, false},
		{"nested model list", &mcp.Tool{Meta: mcp.Meta{"ui": map[string]any{"visibility": []string{"app", "model"}}}}, true},
		{"legacy app list", &mcp.Tool{Meta: mcp.Meta{"ui/visibility": []any{"app"}}}, false},
		{"legacy model list", &mcp.Tool{Meta: mcp.Meta{"ui/visibility": []any{"model"}}}, true},
		{"scalar app", &mcp.Tool{Meta: mcp.Meta{"ui": map[string]any{"visibility": "app"}}}, false},
		{"scalar model", &mcp.Tool{Meta: mcp.Meta{"ui/visibility": "model"}}, true},
		{"nested takes precedence", &mcp.Tool{Meta: mcp.Meta{"ui": map[string]any{"visibility": []any{"app"}}, "ui/visibility": []any{"model"}}}, false},
		{"invalid visibility", &mcp.Tool{Meta: mcp.Meta{"ui/visibility": true}}, false},
	} {
		t.Run(test.name, func(t *testing.T) { require.Equal(t, test.want, ModelVisible(test.tool)) })
	}
}
