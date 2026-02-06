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
		for _, t := range client.GetAllowedTools(ctx) {
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

	res, err := client.CallTool(ctx, &mcp.CallToolParams{
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
