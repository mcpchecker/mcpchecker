package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/coder/acp-go-sdk"
	"github.com/mcpchecker/mcpchecker/pkg/acpclient"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/mcpchecker/mcpchecker/pkg/openaiagent"
	"github.com/mcpchecker/mcpchecker/pkg/tokenizer"
)

type openAIACPRunner struct {
	model      string
	baseURL    string
	apiKey     string
	mcpServers mcpproxy.ServerManager
}

var _ Runner = &openAIACPRunner{}

func NewOpenAIACPRunner(model, baseURL, apiKey string) (Runner, error) {
	if model == "" || baseURL == "" || apiKey == "" {
		return nil, fmt.Errorf("model, baseURL, and apiKey are required for OpenAI ACP agent")
	}

	return &openAIACPRunner{
		model:   model,
		baseURL: baseURL,
		apiKey:  apiKey,
	}, nil
}

func (r *openAIACPRunner) AgentName() string {
	return fmt.Sprintf("openai-acp-%s", r.model)
}

func (r *openAIACPRunner) WithMcpServerInfo(mcpServers mcpproxy.ServerManager) Runner {
	return &openAIACPRunner{
		model:      r.model,
		baseURL:    r.baseURL,
		apiKey:     r.apiKey,
		mcpServers: mcpServers,
	}
}

func (r *openAIACPRunner) RunTask(ctx context.Context, prompt string) (AgentResult, error) {
	agent, err := openaiagent.NewAIAgent(r.baseURL, r.apiKey, r.model, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI agent: %w", err)
	}

	// Add MCP servers to the agent
	if r.mcpServers != nil {
		for _, srv := range r.mcpServers.GetMcpServers() {
			cfg, err := srv.GetConfig()
			if err != nil {
				agent.Close()
				return nil, fmt.Errorf("failed to get config for server %s: %w", srv.GetName(), err)
			}
			if err := agent.AddMCPServer(ctx, cfg.URL); err != nil {
				agent.Close()
				return nil, fmt.Errorf("failed to add MCP server %s: %w", srv.GetName(), err)
			}
		}
	}

	transport := newOpenAIACPTransport(ctx, agent)

	client := acpclient.NewClient(ctx, &acpclient.AcpConfig{
		Transport: transport,
	})

	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start ACP client: %w", err)
	}
	// Only defer Close after successful Start - Start handles its own cleanup on failure
	defer client.Close(ctx)

	updates, err := client.Run(ctx, prompt, r.mcpServers)
	if err != nil {
		return nil, fmt.Errorf("failed to run ACP agent: %w", err)
	}

	return &openAIACPResult{updates: updates, prompt: prompt}, nil
}

type openAIACPResult struct {
	updates []acp.SessionUpdate
	prompt  string
}

var _ AgentResult = &openAIACPResult{}

func (res *openAIACPResult) GetOutput() string {
	if len(res.updates) == 0 {
		return "got no output from acp agent"
	}

	out, err := json.Marshal(res.updates)
	if err != nil {
		lastUpdate := res.updates[len(res.updates)-1]
		if lastUpdate.AgentMessageChunk != nil &&
			lastUpdate.AgentMessageChunk.Content.Text != nil {
			return lastUpdate.AgentMessageChunk.Content.Text.Text
		}
		return "unable to get agent output from last acp update"
	}

	return string(out)
}

func (res *openAIACPResult) GetFinalMessage() string {
	var message strings.Builder
	for _, update := range res.updates {
		if update.AgentMessageChunk != nil && update.AgentMessageChunk.Content.Text != nil {
			message.WriteString(update.AgentMessageChunk.Content.Text.Text)
		}
	}
	return message.String()
}

func (res *openAIACPResult) GetToolCalls() []ToolCallSummary {
	var toolCalls []ToolCallSummary
	seen := make(map[acp.ToolCallId]bool)

	for _, update := range res.updates {
		if update.ToolCall != nil {
			if seen[update.ToolCall.ToolCallId] {
				continue
			}
			seen[update.ToolCall.ToolCallId] = true

			tc := ToolCallSummary{
				Title:     update.ToolCall.Title,
				Kind:      string(update.ToolCall.Kind),
				Status:    string(update.ToolCall.Status),
				RawInput:  update.ToolCall.RawInput,
				RawOutput: update.ToolCall.RawOutput,
			}
			toolCalls = append(toolCalls, tc)
		}
	}
	return toolCalls
}

func (res *openAIACPResult) GetThinking() string {
	var thinking strings.Builder
	for _, update := range res.updates {
		if update.AgentThoughtChunk != nil && update.AgentThoughtChunk.Content.Text != nil {
			thinking.WriteString(update.AgentThoughtChunk.Content.Text.Text)
		}
	}
	return thinking.String()
}

func (res *openAIACPResult) GetRawUpdates() any {
	return res.updates
}

func (res *openAIACPResult) GetTokenEstimate() TokenEstimate {
	tok := tokenizer.Get()
	var errors []string

	// Count prompt tokens (INPUT)
	promptTokens, err := tok.CountTokens(res.prompt)
	if err != nil {
		log.Printf("Warning: failed to count prompt tokens: %v", err)
		errors = append(errors, "prompt")
	}

	// Count agent's final message tokens (OUTPUT)
	messageTokens, err := tok.CountTokens(res.GetFinalMessage())
	if err != nil {
		log.Printf("Warning: failed to count message tokens: %v", err)
		errors = append(errors, "message")
		messageTokens = 0
	}

	// Count thinking tokens (OUTPUT)
	thinkingTokens, err := tok.CountTokens(res.GetThinking())
	if err != nil {
		log.Printf("Warning: failed to count thinking tokens: %v", err)
		errors = append(errors, "thinking")
		thinkingTokens = 0
	}

	var toolCallTokens, toolResultTokens int64
	for _, tc := range res.GetToolCalls() {
		// Tool call parameters: agent -> tools (OUTPUT - agent generates these)
		if inputJSON, err := json.Marshal(tc.RawInput); err == nil {
			if count, err := tok.CountTokens(string(inputJSON)); err != nil {
				log.Printf("Warning: failed to count tool call tokens: %v", err)
				errors = append(errors, "tool_calls")
			} else {
				toolCallTokens += int64(count)
			}
		}
		// Tool results: tools -> agent (INPUT - these go back into agent context)
		if outputJSON, err := json.Marshal(tc.RawOutput); err == nil {
			if count, err := tok.CountTokens(string(outputJSON)); err != nil {
				log.Printf("Warning: failed to count tool result tokens: %v", err)
				errors = append(errors, "tool_results")
			} else {
				toolResultTokens += int64(count)
			}
		}
	}

	// Input = prompt + tool results (what goes INTO the model)
	inputTokens := int64(promptTokens) + toolResultTokens

	// Output = message + thinking + tool calls (what model GENERATES)
	outputTokens := int64(messageTokens) + int64(thinkingTokens) + toolCallTokens

	var errorStr string
	if len(errors) > 0 {
		errorStr = fmt.Sprintf("failed to count: %s", strings.Join(errors, ", "))
	}

	return TokenEstimate{
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		PromptTokens:     int64(promptTokens),
		MessageTokens:    int64(messageTokens),
		ThinkingTokens:   int64(thinkingTokens),
		ToolCallTokens:   toolCallTokens,
		ToolResultTokens: toolResultTokens,
		TotalTokens:      inputTokens + outputTokens,
		Error:            errorStr,
	}
}

// openAIACPTransport implements acpclient.Transport using in-memory pipes
// connected to an OpenAI agent running the ACP protocol.
type openAIACPTransport struct {
	ctx    context.Context
	cancel context.CancelFunc
	agent  *openaiagent.AIAgent

	// Pipes for communication
	clientToAgentReader *io.PipeReader
	clientToAgentWriter *io.PipeWriter
	agentToClientReader *io.PipeReader
	agentToClientWriter *io.PipeWriter

	done chan error
}

func newOpenAIACPTransport(ctx context.Context, agent *openaiagent.AIAgent) *openAIACPTransport {
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()

	ctx, cancel := context.WithCancel(ctx)

	return &openAIACPTransport{
		ctx:                 ctx,
		cancel:              cancel,
		agent:               agent,
		clientToAgentReader: clientToAgentReader,
		clientToAgentWriter: clientToAgentWriter,
		agentToClientReader: agentToClientReader,
		agentToClientWriter: agentToClientWriter,
		done:                make(chan error, 1),
	}
}

func (t *openAIACPTransport) Start(ctx context.Context) (stdin io.Writer, stdout io.Reader, err error) {
	// Start the agent's ACP handler in a goroutine
	go func() {
		// Agent reads from clientToAgentReader, writes to agentToClientWriter
		err := openaiagent.RunACP(t.ctx, t.agent, t.clientToAgentReader, t.agentToClientWriter)
		t.done <- err
	}()

	// Client writes to clientToAgentWriter, reads from agentToClientReader
	return t.clientToAgentWriter, t.agentToClientReader, nil
}

func (t *openAIACPTransport) Close(ctx context.Context) error {
	// Cancel the context to signal RunACP to exit
	t.cancel()

	// Close all pipe ends to signal EOF and unblock any pending I/O
	t.clientToAgentWriter.Close()
	t.clientToAgentReader.Close()
	t.agentToClientWriter.Close()
	t.agentToClientReader.Close()

	// Wait for agent goroutine to finish and capture its error
	var err error
	select {
	case err = <-t.done:
	case <-ctx.Done():
		err = ctx.Err()
	}

	return errors.Join(err, t.agent.Close())
}

var _ acpclient.Transport = &openAIACPTransport{}
