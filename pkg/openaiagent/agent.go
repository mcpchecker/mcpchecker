package openaiagent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/shared"

	"github.com/mcpchecker/mcpchecker/pkg/usage"
)

// AIAgent is an AI agent that connects to MCP servers and uses an OpenAI-compatible API.
type AIAgent struct {
	client       *openai.Client
	mcpClients   []*McpClient
	model        shared.ChatModel
	systemPrompt string
	usage        *usage.TokenUsage
}

type runOpts struct {
	prompt              string
	mcpClients          []*McpClient
	onNewMessage        func(ctx context.Context, msg openai.ChatCompletionMessage) error
	onNewToolCall       func(ctx context.Context, name string, args map[string]any) (string, error)
	onToolCallCompleted func(ctx context.Context, id string, output string) error
	toolCallAllowed     func(ctx context.Context, id string, args map[string]any) (bool, error)
}

func NewAIAgent(url, apiKey, model, systemPrompt string) (*AIAgent, error) {
	if url == "" || apiKey == "" || model == "" {
		return nil, fmt.Errorf("url, API key, and model name must all be provided to create an ai agent")
	}

	client := openai.NewClient(
		option.WithBaseURL(url),
		option.WithAPIKey(apiKey),
	)

	return &AIAgent{
		client:       &client,
		mcpClients:   make([]*McpClient, 0),
		model:        shared.ChatModel(model),
		systemPrompt: systemPrompt,
		usage:        &usage.TokenUsage{},
	}, nil
}

// AddMCPServer adds an MCP server to the agent
func (a *AIAgent) AddMCPServer(ctx context.Context, serverURL string) error {
	mcpClient, err := NewMcpClient(ctx, serverURL)
	if err != nil {
		return fmt.Errorf("failed to create MCP client for %s: %w", serverURL, err)
	}

	// Load available tools from the MCP server
	if err := mcpClient.LoadTools(ctx); err != nil {
		mcpClient.Close()
		return fmt.Errorf("failed to load MCP tools from %s: %w", serverURL, err)
	}

	a.mcpClients = append(a.mcpClients, mcpClient)
	return nil
}

func (a *AIAgent) Run(ctx context.Context, prompt string) (string, error) {
	return a.runTask(ctx, newNoopRunOpts(prompt))
}

func (a *AIAgent) runTask(ctx context.Context, opts runOpts) (string, error) {
	// Initialize usage tracker for this run
	a.usage = &usage.TokenUsage{}

	// Start conversation with system prompt (if provided) and user's prompt
	var messages []openai.ChatCompletionMessageParamUnion

	if a.systemPrompt != "" {
		messages = append(messages, openai.SystemMessage(a.systemPrompt))
	}

	messages = append(messages, openai.UserMessage(opts.prompt))

	// Merge agent's MCP clients with session-specific ones
	allMcpClients := append(a.mcpClients, opts.mcpClients...)

	// Get available tools from all MCP clients
	var tools []openai.ChatCompletionToolUnionParam
	for _, mcpClient := range allMcpClients {
		clientTools := mcpClient.GetTools()
		tools = append(tools, clientTools...)
	}

	// Agent loop - continue until we get a final response without tool calls
	for {
		params := openai.ChatCompletionNewParams{
			Model:    a.model,
			Messages: messages,
		}

		// Add tools if available
		if len(tools) > 0 {
			params.Tools = tools
		}

		// Make the chat completion request
		completion, err := a.client.Chat.Completions.New(ctx, params)
		if err != nil {
			return "", fmt.Errorf("failed to create chat completion: %w", err)
		}

		// Capture usage from this API call
		tokenUsage := usage.FromOpenAIUsage(completion.Usage)
		a.usage.Add(tokenUsage)

		if len(completion.Choices) == 0 {
			return "", fmt.Errorf("no completion choices returned")
		}

		choice := completion.Choices[0]
		message := choice.Message

		if err := opts.onNewMessage(ctx, message); err != nil {
			return "", fmt.Errorf("failed to handle new message: %w", err)
		}

		// Add the assistant's message to the conversation
		// Important: Use ToParam() to preserve tool_calls if present, not just the content
		messages = append(messages, message.ToParam())

		// If there are no tool calls, we're done
		if len(message.ToolCalls) == 0 {
			return message.Content, nil
		}

		// Execute tool calls and add results to conversation
		for _, toolCall := range message.ToolCalls {
			if toolCall.Function.Name == "" {
				continue
			}

			// Parse tool arguments
			var args map[string]any
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
				return "", fmt.Errorf("failed to parse tool arguments: %w", err)
			}

			toolCallID, err := opts.onNewToolCall(ctx, toolCall.Function.Name, args)
			if err != nil {
				return "", fmt.Errorf("failed to handle new tool call: %w", err)
			}

			allowed, err := opts.toolCallAllowed(ctx, toolCallID, args)
			if err != nil {
				return "", fmt.Errorf("failed to check tool call permission: %w", err)
			}

			var result string
			if !allowed {
				result = "Tool call was rejected by the user."
			} else {
				// Find which MCP client has this tool and execute it
				result, err = a.callToolOnClients(ctx, allMcpClients, toolCall.Function.Name, args)
				if err != nil {
					result = fmt.Sprintf("Error calling tool: %v", err)
				}
			}

			if err := opts.onToolCallCompleted(ctx, toolCallID, result); err != nil {
				return "", fmt.Errorf("failed to handle tool call completion: %w", err)
			}

			// Add tool result to conversation
			messages = append(messages, openai.ToolMessage(result, toolCall.ID))
		}
	}
}

// callToolOnClients finds the MCP client that has the specified tool and calls it
func (a *AIAgent) callToolOnClients(ctx context.Context, clients []*McpClient, toolName string, arguments map[string]any) (string, error) {
	// Search through all MCP clients to find one that has this tool
	for _, mcpClient := range clients {
		tools := mcpClient.GetTools()
		for _, tool := range tools {
			// Check if this is a function tool with the matching name
			if funcDef := tool.GetFunction(); funcDef != nil && funcDef.Name == toolName {
				// Found the tool, call it on this client
				return mcpClient.CallTool(ctx, toolName, arguments)
			}
		}
	}

	return "", fmt.Errorf("tool %s not found in any MCP client", toolName)
}

// GetUsage returns aggregated token usage from the most recent Run call
func (a *AIAgent) GetUsage() *usage.TokenUsage {
	return a.usage
}

// Close closes the agent and any associated resources
func (a *AIAgent) Close() error {
	var errs []error
	for _, mcpClient := range a.mcpClients {
		if err := mcpClient.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to close %d MCP clients: %v", len(errs), errs)
	}

	return nil
}

func newNoopRunOpts(prompt string) runOpts {
	return runOpts{
		prompt:              prompt,
		onNewMessage:        func(_ context.Context, _ openai.ChatCompletionMessage) error { return nil },
		onNewToolCall:       func(_ context.Context, name string, _ map[string]any) (string, error) { return name, nil },
		toolCallAllowed:     func(_ context.Context, _ string, _ map[string]any) (bool, error) { return true, nil },
		onToolCallCompleted: func(_ context.Context, _, _ string) error { return nil },
	}
}
