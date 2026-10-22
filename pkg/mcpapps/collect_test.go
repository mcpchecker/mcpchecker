package mcpapps

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestCaptureResourceValidation(t *testing.T) {
	const uri = "ui://app/view"
	for _, test := range []struct {
		name, uri, mime, html, code, detail string
		readErr                             error
		resultMeta                          mcp.Meta
	}{
		{name: "missing UI", code: "missing_ui_resource"},
		{name: "read failure", uri: uri, readErr: errors.New("HTTP 403: permission denied\n<html>PRIVATE_UI_HTML</html>"), code: "ui_resource_read_failed", detail: "HTTP 403: permission denied [HTML omitted]"},
		{name: "long read failure", uri: uri, readErr: errors.New(strings.Repeat("x", 600)), code: "ui_resource_read_failed", detail: strings.Repeat("x", 509) + "..."},
		{name: "wrong MIME", uri: uri, mime: "text/html", html: "<body/>", code: "invalid_ui_mime_type"},
		{name: "empty resource", uri: uri, mime: MIMEType, code: "invalid_ui_resource_size"},
		{name: "valid blob", uri: uri, mime: MIMEType, html: "<body/>"},
		{name: "dynamic URI only", resultMeta: mcp.Meta{"ui": map[string]any{"resourceUri": uri}}, mime: MIMEType, html: "<body/>"},
		{name: "legacy dynamic URI overrides static", uri: "ui://stale", resultMeta: mcp.Meta{"ui/resourceUri": uri}, mime: MIMEType, html: "<body/>"},
	} {
		t.Run(test.name, func(t *testing.T) {
			collector := NewCollector([]Assertion{{Server: "fixture", Tool: "view"}})
			collector.Observe(t.Context(), "fixture", &mcp.Tool{Name: "view", Meta: mcp.Meta{"ui": map[string]any{"resourceUri": test.uri}}}, &mcp.CallToolResult{Meta: test.resultMeta}, nil, func(_ context.Context, params *mcp.ReadResourceParams, appCallID string) (*mcp.ReadResourceResult, error) {
				require.Equal(t, uri, params.URI)
				require.Empty(t, params.Meta)
				require.NotEmpty(t, appCallID)
				if test.readErr != nil {
					return nil, test.readErr
				}
				return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: test.mime, Blob: []byte(test.html)}}}, nil
			})
			observations := collector.Observations()
			require.Len(t, observations, 1)
			require.Equal(t, test.code, observations[0].Code)
			require.Equal(t, test.detail, observations[0].Detail)
		})
	}
}
