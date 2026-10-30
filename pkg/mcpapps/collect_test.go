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
		text                                bool
	}{
		{name: "missing UI", code: "missing_ui_resource"},
		{name: "non Apps URI", uri: "https://example.com/view", code: "invalid_ui_resource_uri"},
		{name: "malformed URI", uri: "ui://app/%zz", code: "invalid_ui_resource_uri"},
		{name: "empty Apps URI", uri: "ui://", code: "invalid_ui_resource_uri"},
		{name: "invalid dynamic URI overrides static", uri: uri, resultMeta: mcp.Meta{"ui": map[string]any{"resourceUri": "file:///view.html"}}, code: "invalid_ui_resource_uri"},
		{name: "read failure", uri: uri, readErr: errors.New("HTTP 403: permission denied\n<html>PRIVATE_UI_HTML</html>"), code: "ui_resource_read_failed", detail: "HTTP 403: permission denied [HTML omitted]"},
		{name: "long read failure", uri: uri, readErr: errors.New(strings.Repeat("x", 600)), code: "ui_resource_read_failed", detail: strings.Repeat("x", 509) + "..."},
		{name: "wrong MIME", uri: uri, mime: "text/html", html: "<body/>", code: "invalid_ui_mime_type"},
		{name: "empty resource", uri: uri, mime: MIMEType, code: "invalid_ui_resource_size"},
		{name: "whitespace text", uri: uri, mime: MIMEType, html: " \n\t", text: true, code: "invalid_ui_resource_size"},
		{name: "oversized resource", uri: uri, mime: MIMEType, html: strings.Repeat("x", (1<<20)+1), code: "invalid_ui_resource_size"},
		{name: "valid text", uri: uri, mime: MIMEType, html: "<body/>", text: true},
		{name: "valid blob", uri: uri, mime: MIMEType, html: "<body/>"},
		{name: "dynamic URI only", resultMeta: mcp.Meta{"ui": map[string]any{"resourceUri": uri}}, mime: MIMEType, html: "<body/>"},
		{name: "legacy dynamic URI overrides static", uri: "ui://stale", resultMeta: mcp.Meta{"ui/resourceUri": uri}, mime: MIMEType, html: "<body/>"},
	} {
		t.Run(test.name, func(t *testing.T) {
			collector := NewCollector([]Assertion{{Server: "fixture", Tool: "view"}})
			readCalled := false
			collector.Observe(t.Context(), "fixture", &mcp.Tool{Name: "view", Meta: mcp.Meta{"ui": map[string]any{"resourceUri": test.uri}}}, &mcp.CallToolResult{Meta: test.resultMeta}, nil, func(_ context.Context, params *mcp.ReadResourceParams, appCallID string) (*mcp.ReadResourceResult, error) {
				readCalled = true
				require.Equal(t, uri, params.URI)
				require.Empty(t, params.Meta)
				require.NotEmpty(t, appCallID)
				if test.readErr != nil {
					return nil, test.readErr
				}
				content := &mcp.ResourceContents{URI: uri, MIMEType: test.mime}
				if test.text {
					content.Text = test.html
				} else {
					content.Blob = []byte(test.html)
				}
				return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{content}}, nil
			})
			require.Equal(t, test.code != "missing_ui_resource" && test.code != "invalid_ui_resource_uri", readCalled)
			observations := collector.Observations()
			require.Len(t, observations, 1)
			require.Equal(t, test.code, observations[0].Code)
			require.Equal(t, test.detail, observations[0].Detail)
		})
	}
}

func TestCaptureResourceURILimit(t *testing.T) {
	for _, tc := range []struct {
		name, reference string
		static          bool
		invalid         bool
	}{
		{"at limit", "ui://app/" + strings.Repeat("x", 2048-len("ui://app/")), false, false},
		{"oversized result URI", "ui://app/" + strings.Repeat("é", 1020), false, true},
		{"oversized static URI", "ui://app/" + strings.Repeat("x", 2040), true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collector := NewCollector([]Assertion{{Server: "fixture", Tool: "view"}})
			tool := &mcp.Tool{Name: "view", Meta: mcp.Meta{"ui/resourceUri": "ui://fallback"}}
			result := &mcp.CallToolResult{Meta: mcp.Meta{"ui/resourceUri": tc.reference}}
			if tc.static {
				tool.Meta["ui/resourceUri"] = tc.reference
				result.Meta = nil
			}
			readCalled := false
			collector.Observe(t.Context(), "fixture", tool, result, nil, func(_ context.Context, params *mcp.ReadResourceParams, _ string) (*mcp.ReadResourceResult, error) {
				readCalled = true
				require.Equal(t, tc.reference, params.URI)
				return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: params.URI, MIMEType: MIMEType, Text: "<body/>"}}}, nil
			})
			o := collector.Observations()[0]
			require.Equal(t, !tc.invalid, readCalled)
			if tc.invalid {
				require.Equal(t, "invalid_ui_resource_uri", o.Code)
				require.Empty(t, o.URI)
			} else {
				require.Empty(t, o.Code)
				require.Equal(t, tc.reference, o.URI)
			}
		})
	}
}
