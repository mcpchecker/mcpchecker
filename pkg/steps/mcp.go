package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type McpStep struct {
	serverName string
	toolName   string
	Args       map[string]any `json:"args,omitempty"`
	Expect     *McpExpect     `json:"expect,omitempty"`
}

type McpExpect struct {
	IsError *bool       `json:"isError,omitempty"`
	Content *ExpectBody `json:"content,omitempty"`
}

var _ StepRunner = &McpStep{}

func NewMcpServerParser(ctx context.Context, serverName string) PrefixParser {
	return func(toolName string, raw json.RawMessage) (StepRunner, error) {
		manager, ok := mcpclient.ManagerFromContext(ctx)
		if !ok {
			return nil, fmt.Errorf("unable to get mcpclient.Manager from context")
		}

		client, ok := manager.Get(serverName)
		if !ok {
			return nil, fmt.Errorf("no mcp server registered that matches name %q", serverName)
		}

		found := false
		allowedTools, err := client.GetAllowedTools(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to discover allowed tools for MCP server %q: %w", serverName, err)
		}
		for _, t := range allowedTools {
			if t.Name == toolName {
				found = true
			}
		}

		if !found {
			return nil, fmt.Errorf("no tool named %q registered on mcp server %q", toolName, serverName)
		}

		step := &McpStep{serverName: serverName, toolName: toolName}

		if err := json.Unmarshal(raw, &step); err != nil {
			return nil, fmt.Errorf("failed to unmarshal mcp step config: %w", err)
		}

		return step, nil
	}
}

func (s *McpStep) Execute(ctx context.Context, input *StepInput) (*StepOutput, error) {
	manager, ok := mcpclient.ManagerFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("unable to get mcpclient.Manager from context")
	}

	client, ok := manager.Get(s.serverName)
	if !ok {
		return nil, fmt.Errorf("no mcp server registered that matches name %q", s.serverName)
	}

	res, err := callScriptedTool(ctx, client, &mcp.CallToolParams{
		Name:      s.toolName,
		Arguments: s.Args,
	})

	out := &StepOutput{
		Success: true,
		Type:    fmt.Sprintf("%s.%s", s.serverName, s.toolName),
		Outputs: make(map[string]string),
	}

	if err != nil {
		out.Success = false
		out.Error = err.Error()
		if s.Expect != nil && s.Expect.IsError != nil && *s.Expect.IsError {
			out.Success = true
		}
		return out, nil
	}

	if res.NeedsInput() {
		out.Success = false
		out.Error = "tool requires input, which scripted mcp steps cannot provide"
		return out, nil
	}

	// TODO: improve output handling to store more detailed output (not just string type)
	serializedOut, marshalErr := json.Marshal(res.Content)
	if marshalErr == nil {
		out.Outputs["content"] = string(serializedOut)
	}

	if res.IsError {
		if s.Expect != nil && s.Expect.IsError != nil && *s.Expect.IsError {
			out.Success = true
		} else {
			out.Success = false
		}
	}

	if s.Expect != nil && s.Expect.Content != nil {
		errors := s.Expect.Content.Validate(serializedOut)
		if len(errors) > 0 {
			out.Success = false
			out.Message = fmt.Sprintf("response failed content validation: %s", strings.Join(errors, ";"))
		}
	}

	return out, nil
}

// callScriptedTool retries empty input requests (load shedding) on passthrough
// connections. Match the SDK's limit of three load-shedding responses.
func callScriptedTool(ctx context.Context, client *mcpclient.Client, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	const maxAttempts = 3
	for range maxAttempts {
		res, err := client.CallTool(ctx, params)
		if err != nil || !res.NeedsInput() || len(res.InputRequests) > 0 {
			return res, err
		}
		params.RequestState = res.RequestState
		params.InputResponses = mcp.InputResponseMap{}
	}
	return nil, fmt.Errorf("multi-round-trip: exceeded maximum load-shedding retries (%d)", maxAttempts)
}
