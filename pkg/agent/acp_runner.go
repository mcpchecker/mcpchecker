package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/coder/acp-go-sdk"
	"github.com/mcpchecker/mcpchecker/pkg/acpclient"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/mcpchecker/mcpchecker/pkg/tokenizer"
)

type acpRunner struct {
	name       string
	cfg        *acpclient.AcpConfig
	mcpServers mcpproxy.ServerManager
}

var _ Runner = &acpRunner{}

func NewAcpRunner(cfg *acpclient.AcpConfig, name string) Runner {
	return &acpRunner{
		name: name,
		cfg:  cfg,
	}
}

func (r *acpRunner) RunTask(ctx context.Context, prompt string) (AgentResult, error) {
	client := acpclient.NewClient(ctx, r.cfg)
	defer client.Close(ctx)

	err := client.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start acp client: %w", err)
	}

	result, err := client.Run(ctx, prompt, r.mcpServers)
	if err != nil {
		return nil, fmt.Errorf("failed to run acp agent: %w", err)
	}

	return &acpRunnerResult{
		updates: result,
		prompt:  prompt,
	}, nil
}

func (r *acpRunner) WithMcpServerInfo(mcpServers mcpproxy.ServerManager) Runner {
	return &acpRunner{
		name:       r.name,
		cfg:        r.cfg,
		mcpServers: mcpServers,
	}
}

func (r *acpRunner) AgentName() string {
	return r.name
}

type acpRunnerResult struct {
	updates []acp.SessionUpdate
	prompt  string // Original prompt sent to agent
}

var _ AgentResult = &acpRunnerResult{}

func (res *acpRunnerResult) GetOutput() string {
	if len(res.updates) == 0 {
		return "got no output from acp agent"
	}

	out, err := json.Marshal(res.updates)
	if err != nil {
		text := res.updates[len(res.updates)-1].AgentMessageChunk.Content.Text
		if text != nil {
			return text.Text
		}

		return "unable to get agent output from last acp update"
	}

	return string(out)
}

func (res *acpRunnerResult) GetFinalMessage() string {
	var message strings.Builder
	for _, update := range res.updates {
		if update.AgentMessageChunk != nil && update.AgentMessageChunk.Content.Text != nil {
			message.WriteString(update.AgentMessageChunk.Content.Text.Text)
		}
	}
	return message.String()
}

func (res *acpRunnerResult) GetToolCalls() []ToolCallSummary {
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

func (res *acpRunnerResult) GetThinking() string {
	var thinking strings.Builder
	for _, update := range res.updates {
		if update.AgentThoughtChunk != nil && update.AgentThoughtChunk.Content.Text != nil {
			thinking.WriteString(update.AgentThoughtChunk.Content.Text.Text)
		}
	}
	return thinking.String()
}

func (res *acpRunnerResult) GetRawUpdates() any {
	return res.updates
}

func (res *acpRunnerResult) GetTokenEstimate() TokenEstimate {
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
