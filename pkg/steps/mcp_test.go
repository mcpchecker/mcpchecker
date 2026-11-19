package steps

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMcpStepExecuteInputRequired(t *testing.T) {
	expectError := true
	const needsInput = "tool requires input, which scripted mcp steps cannot provide"
	for _, tc := range []struct {
		name         string
		busyRounds   int32
		requestInput bool
		wantCalls    int32
		wantError    string
	}{
		{name: "completed tool succeeds", wantCalls: 1},
		{name: "input required fails even with expected error", requestInput: true, wantCalls: 1, wantError: needsInput},
		{name: "temporary load shedding succeeds", busyRounds: 2, wantCalls: 3},
		{name: "persistent load shedding stops", busyRounds: 3, wantCalls: 3, wantError: "exceeded maximum load-shedding retries"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
			server.AddTool(&mcp.Tool{Name: "check", InputSchema: map[string]any{"type": "object"}},
				func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					call := calls.Add(1)
					if call > 1 {
						assert.Equal(t, fmt.Sprintf("state-%d", call-1), req.Params.RequestState)
					}
					if call <= tc.busyRounds {
						return &mcp.CallToolResult{InputRequests: mcp.InputRequestMap{}, RequestState: fmt.Sprintf("state-%d", call)}, nil
					}
					if tc.requestInput {
						return &mcp.CallToolResult{InputRequests: mcp.InputRequestMap{
							"confirm": &mcp.ElicitParams{Message: "Confirm?", RequestedSchema: &jsonschema.Schema{Type: "object"}},
						}}, nil
					}
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "done"}}}, nil
				})
			httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
				return server
			}, &mcp.StreamableHTTPOptions{Stateless: true}))
			t.Cleanup(httpServer.Close)

			ctx := t.Context()
			manager, err := mcpclient.NewManager(ctx, &mcpclient.MCPConfig{
				MCPServers: map[string]*mcpclient.ServerConfig{
					"test": {Type: mcpclient.TransportTypeHttp, URL: httpServer.URL},
				},
			}, mcpclient.WithInputRequiredPassthrough())
			require.NoError(t, err)
			t.Cleanup(func() { _ = manager.Close(context.Background()) })
			ctx = mcpclient.ManagerToContext(ctx, manager)

			step := &McpStep{serverName: "test", toolName: "check"}
			if tc.requestInput {
				step.Expect = &McpExpect{IsError: &expectError}
			}
			out, err := step.Execute(ctx, &StepInput{})
			require.NoError(t, err)
			assert.Equal(t, tc.wantCalls, calls.Load())
			assert.Equal(t, tc.wantError == "", out.Success)
			if tc.wantError != "" {
				assert.Contains(t, out.Error, tc.wantError)
				assert.NotContains(t, out.Outputs, "content")
			} else {
				assert.Empty(t, out.Error)
				assert.Contains(t, out.Outputs["content"], "done")
			}
		})
	}
}
