package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/coder/acp-go-sdk"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/mcpchecker/mcpchecker/pkg/tokenizer"
)

// TokenSource indicates where token counts came from.
type TokenSource string

const (
	TokenSourceEstimated TokenSource = "estimated"
	TokenSourceActual    TokenSource = "actual"
)

type Runner interface {
	RunTask(ctx context.Context, prompt string) (AgentResult, error)
	WithMcpServerInfo(mcpServers mcpproxy.ServerManager) Runner
	AgentName() string
}

type McpServerInfo interface {
	GetMcpServerFiles() ([]string, error)
	GetMcpServers() []mcpproxy.Server
}

// ToolCallSummary provides structured access to tool call data.
type ToolCallSummary struct {
	Title     string `json:"title"`
	Kind      string `json:"kind,omitempty"`
	Status    string `json:"status,omitempty"`
	RawInput  any    `json:"rawInput,omitempty"`
	RawOutput any    `json:"rawOutput,omitempty"`
}

// ActualUsage contains real token counts from the agent (when reported).
type ActualUsage struct {
	InputTokens       int64  `json:"inputTokens"`
	OutputTokens      int64  `json:"outputTokens"`
	TotalTokens       int64  `json:"totalTokens"`
	ThoughtTokens     *int64 `json:"thoughtTokens,omitempty"`
	CachedReadTokens  *int64 `json:"cachedReadTokens,omitempty"`
	CachedWriteTokens *int64 `json:"cachedWriteTokens,omitempty"`
}

// TokenEstimate provides token count estimates for different components.
// Uses tiktoken library with cl100k_base encoding (may differ 10-30% for non-OpenAI models).
type TokenEstimate struct {
	// InputTokens: initial prompt + tool results only (excludes system prompt,
	// multi-turn context, and cache)
	InputTokens int64 `json:"inputTokens"`
	// OutputTokens: agent's final message + thinking + tool call params
	OutputTokens int64 `json:"outputTokens"`
	// PromptTokens: the initial prompt sent to the agent
	PromptTokens int64 `json:"promptTokens"`
	// MessageTokens: agent's final response message
	MessageTokens int64 `json:"messageTokens"`
	// ThinkingTokens: agent's reasoning/thinking content
	ThinkingTokens int64 `json:"thinkingTokens"`
	// ToolCallTokens: tool call parameters (agent -> tools)
	ToolCallTokens int64 `json:"toolCallTokens"`
	// ToolResultTokens: tool results (tools -> agent, counted as input)
	ToolResultTokens int64 `json:"toolResultTokens"`
	// TotalTokens is InputTokens + OutputTokens
	TotalTokens int64 `json:"totalTokens"`
	// Error indicates if tokenization failed (partial or complete failure)
	Error string `json:"error,omitempty"`
	// Source indicates where the counts came from: "actual" or "estimated"
	Source TokenSource `json:"source,omitempty"`
	// Actual contains real usage from agent when available (nil if only estimated)
	Actual *ActualUsage `json:"actual,omitempty"`
}

// ExtractToolCalls extracts deduplicated tool call summaries from ACP session updates.
// It merges data from both ToolCall (initial) and ToolCallUpdate (subsequent) messages.
func ExtractToolCalls(updates []acp.SessionUpdate) []ToolCallSummary {
	// Use a map to collect and merge tool call data by ID
	toolCallMap := make(map[acp.ToolCallId]*ToolCallSummary)
	var order []acp.ToolCallId // preserve insertion order

	for _, update := range updates {
		// Handle initial tool call notifications
		if update.ToolCall != nil {
			id := update.ToolCall.ToolCallId
			if _, exists := toolCallMap[id]; !exists {
				order = append(order, id)
				toolCallMap[id] = &ToolCallSummary{
					Title:     update.ToolCall.Title,
					Kind:      string(update.ToolCall.Kind),
					Status:    string(update.ToolCall.Status),
					RawInput:  update.ToolCall.RawInput,
					RawOutput: update.ToolCall.RawOutput,
				}
			}
		}

		// Handle tool call updates (may contain RawOutput that arrives later)
		if update.ToolCallUpdate != nil {
			id := update.ToolCallUpdate.ToolCallId
			tc, exists := toolCallMap[id]
			if !exists {
				// ToolCallUpdate arrived before ToolCall (unusual but handle it)
				order = append(order, id)
				tc = &ToolCallSummary{}
				toolCallMap[id] = tc
			}

			// Merge update fields (only if non-nil/non-empty)
			if update.ToolCallUpdate.Title != nil {
				tc.Title = *update.ToolCallUpdate.Title
			}
			if update.ToolCallUpdate.Kind != nil {
				tc.Kind = string(*update.ToolCallUpdate.Kind)
			}
			if update.ToolCallUpdate.Status != nil {
				tc.Status = string(*update.ToolCallUpdate.Status)
			}
			if update.ToolCallUpdate.RawInput != nil {
				tc.RawInput = update.ToolCallUpdate.RawInput
			}
			if update.ToolCallUpdate.RawOutput != nil {
				tc.RawOutput = update.ToolCallUpdate.RawOutput
			}
		}
	}

	// Convert map to slice preserving order
	toolCalls := make([]ToolCallSummary, 0, len(order))
	for _, id := range order {
		toolCalls = append(toolCalls, *toolCallMap[id])
	}
	return toolCalls
}

// ExtractFinalMessage extracts the agent's final message from ACP session updates.
func ExtractFinalMessage(updates []acp.SessionUpdate) string {
	var message strings.Builder
	for _, update := range updates {
		if update.AgentMessageChunk != nil && update.AgentMessageChunk.Content.Text != nil {
			message.WriteString(update.AgentMessageChunk.Content.Text.Text)
		}
	}
	return message.String()
}

// ExtractThinking extracts the agent's thinking/reasoning from ACP session updates.
func ExtractThinking(updates []acp.SessionUpdate) string {
	var thinking strings.Builder
	for _, update := range updates {
		if update.AgentThoughtChunk != nil && update.AgentThoughtChunk.Content.Text != nil {
			thinking.WriteString(update.AgentThoughtChunk.Content.Text.Text)
		}
	}
	return thinking.String()
}

// ComputeTokenEstimate calculates token estimates for agent execution.
func ComputeTokenEstimate(prompt, message, thinking string, toolCalls []ToolCallSummary) TokenEstimate {
	tok := tokenizer.Get()
	var errors []string

	// Count prompt tokens (INPUT)
	promptTokens, err := tok.CountTokens(prompt)
	if err != nil {
		log.Printf("Warning: failed to count prompt tokens: %v", err)
		errors = append(errors, "prompt")
		promptTokens = 0
	}

	// Count agent's final message tokens (OUTPUT)
	messageTokens, err := tok.CountTokens(message)
	if err != nil {
		log.Printf("Warning: failed to count message tokens: %v", err)
		errors = append(errors, "message")
		messageTokens = 0
	}

	// Count thinking tokens (OUTPUT)
	thinkingTokens, err := tok.CountTokens(thinking)
	if err != nil {
		log.Printf("Warning: failed to count thinking tokens: %v", err)
		errors = append(errors, "thinking")
		thinkingTokens = 0
	}

	var toolCallTokens, toolResultTokens int64
	for i, tc := range toolCalls {
		// Tool call parameters: agent -> tools (OUTPUT - agent generates these)
		if inputJSON, err := json.Marshal(tc.RawInput); err != nil {
			log.Printf("Warning: failed to marshal tool call input [%d] %q: %v", i, tc.Title, err)
			errors = append(errors, "tool_calls")
		} else {
			if count, err := tok.CountTokens(string(inputJSON)); err != nil {
				log.Printf("Warning: failed to count tool call tokens [%d] %q: %v", i, tc.Title, err)
				errors = append(errors, "tool_calls")
			} else {
				toolCallTokens += int64(count)
			}
		}
		// Tool results: tools -> agent (INPUT - these go back into agent context)
		if outputJSON, err := json.Marshal(tc.RawOutput); err != nil {
			log.Printf("Warning: failed to marshal tool result output [%d] %q: %v", i, tc.Title, err)
			errors = append(errors, "tool_results")
		} else {
			if count, err := tok.CountTokens(string(outputJSON)); err != nil {
				log.Printf("Warning: failed to count tool result tokens [%d] %q: %v", i, tc.Title, err)
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

// AgentResult provides access to the results of an agent execution.
type AgentResult interface {
	GetOutput() string
	GetFinalMessage() string
	GetToolCalls() []ToolCallSummary
	GetThinking() string
	GetRawUpdates() any
	GetTokenEstimate() TokenEstimate
}

type agentSpecRunner struct {
	*AgentSpec
	mcpInfo McpServerInfo
}

type agentSpecRunnerResult struct {
	commandOutput string
}

func (a *agentSpecRunnerResult) GetOutput() string {
	return a.commandOutput
}

func (a *agentSpecRunnerResult) GetFinalMessage() string {
	return a.commandOutput // Shell output is the final message
}

func (a *agentSpecRunnerResult) GetToolCalls() []ToolCallSummary {
	return nil // Shell runner doesn't have structured tool call data
}

func (a *agentSpecRunnerResult) GetThinking() string {
	return "" // Shell runner doesn't capture thinking
}

func (a *agentSpecRunnerResult) GetRawUpdates() any {
	return nil // Shell runner doesn't have session updates
}

func (a *agentSpecRunnerResult) GetTokenEstimate() TokenEstimate {
	return TokenEstimate{Error: "token estimation not supported for shell runner"}
}

func NewRunnerForSpec(spec *AgentSpec) (Runner, error) {
	if spec == nil {
		return nil, fmt.Errorf("cannot create a Runner for a nil AgentSpec")
	}

	// check first for acp config
	if spec.AcpConfig != nil {
		return NewAcpRunner(spec.AcpConfig, spec.Metadata.Name), nil
	}

	// Check if this is an OpenAI agent with builtin configuration
	if spec.Builtin != nil && spec.Builtin.Type == "openai-agent" {
		return NewOpenAIAgentRunner(spec.Builtin.Model, spec.Builtin.BaseURL, spec.Builtin.APIKey)
	}

	// Check if this is an OpenAI ACP agent with builtin configuration
	if spec.Builtin != nil && spec.Builtin.Type == "openai-acp" {
		return NewOpenAIACPRunner(spec.Builtin.Model, spec.Builtin.BaseURL, spec.Builtin.APIKey)
	}

	// Use the standard shell-based runner for all other agents
	return &agentSpecRunner{
		AgentSpec: spec,
	}, nil
}

func (a *agentSpecRunner) RunTask(ctx context.Context, prompt string) (AgentResult, error) {
	debugDir := ""
	if os.Getenv("MCPCHECKER_DEBUG") != "" {
		if dir, err := os.MkdirTemp("", "mcpchecker-debug-"); err == nil {
			debugDir = dir
		} else {
			fmt.Fprintf(os.Stderr, "Warning: failed to create debug directory: %v\n", err)
		}
	}

	// Create an empty temporary directory for agent execution to isolate it from source code
	tempDir, err := os.MkdirTemp("", "mcpchecker-agent-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory for agent execution: %w", err)
	}
	executionSucceeded := false
	defer func() {
		// Clean up temp directory unless execution failed OR MCPCHECKER_DEBUG is set
		// In that case, preserve it for debugging
		shouldPreserve := !executionSucceeded || os.Getenv("MCPCHECKER_DEBUG") != ""
		if !shouldPreserve {
			_ = os.RemoveAll(tempDir)
		} else {
			var reason string
			if !executionSucceeded && os.Getenv("MCPCHECKER_DEBUG") != "" {
				reason = "execution failed and MCPCHECKER_DEBUG is set"
			} else if !executionSucceeded {
				reason = "execution failed"
			} else {
				reason = "MCPCHECKER_DEBUG is set"
			}
			fmt.Fprintf(os.Stderr, "Preserving temporary directory %s because %s\n", tempDir, reason)
		}
	}()

	argTemplateMcpServer, err := template.New("argTemplateMcpServer").Parse(a.Commands.ArgTemplateMcpServer)
	if err != nil {
		return nil, fmt.Errorf("failed to parse argTemplateMcpServer: %w", err)
	}

	argTemplateAllowedTools, err := template.New("argTemplateAllowedTools").Parse(a.Commands.ArgTemplateAllowedTools)
	if err != nil {
		return nil, fmt.Errorf("failed to parse argTemplateAllowedTools: %w", err)
	}

	runPrompt, err := template.New("runPrompt").Parse(a.Commands.RunPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse runPrompt: %w", err)
	}

	var serverFiles []string
	filesRaw, err := a.mcpInfo.GetMcpServerFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to get the mcp server files: %w", err)
	}

	// Get servers to extract URLs
	servers := a.mcpInfo.GetMcpServers()
	if len(filesRaw) != len(servers) {
		return nil, fmt.Errorf("mismatch between number of server files (%d) and servers (%d)", len(filesRaw), len(servers))
	}

	for i, f := range filesRaw {
		serverCfg, err := servers[i].GetConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get config for server %s: %w", servers[i].GetName(), err)
		}

		tmp := struct {
			File string
			URL  string
		}{
			File: f,
			URL:  serverCfg.URL,
		}

		formatted := bytes.NewBuffer(nil)
		err = argTemplateMcpServer.Execute(formatted, tmp)
		if err != nil {
			return nil, fmt.Errorf("failed to execute argTemplateMcpServer: %w", err)
		}

		serverFiles = append(serverFiles, formatted.String())
	}

	var allowedTools []string
	for _, s := range a.mcpInfo.GetMcpServers() {
		for _, t := range s.GetAllowedTools(ctx) {
			tmp := struct {
				ServerName string
				ToolName   string
			}{
				ServerName: s.GetName(),
				ToolName:   t.Name,
			}

			formatted := bytes.NewBuffer(nil)
			err := argTemplateAllowedTools.Execute(formatted, tmp)
			if err != nil {
				return nil, fmt.Errorf("failed to execute argTemplateAllowedTools: %w", err)
			}

			allowedTools = append(allowedTools, formatted.String())
		}
	}

	// Default to space separator if not specified
	allowedToolsSeparator := " "
	if a.Commands.AllowedToolsJoinSeparator != nil {
		allowedToolsSeparator = *a.Commands.AllowedToolsJoinSeparator
	}

	tmp := struct {
		McpServerFileArgs string
		AllowedToolArgs   string
		Prompt            string
	}{
		McpServerFileArgs: strings.Join(serverFiles, " "),
		AllowedToolArgs:   strings.Join(allowedTools, allowedToolsSeparator),
		Prompt:            prompt,
	}

	formatted := bytes.NewBuffer(nil)
	err = runPrompt.Execute(formatted, tmp)
	if err != nil {
		return nil, fmt.Errorf("failed to execute runPrompt: %w", err)
	}

	shell, ok := os.LookupEnv("SHELL")
	if !ok {
		shell = "/usr/bin/bash"
	}

	cmd := exec.CommandContext(ctx, shell, "-c", formatted.String())
	cmd.Dir = tempDir
	envVars := os.Environ()
	if debugDir != "" {
		envVars = append(envVars, fmt.Sprintf("MCPCHECKER_DEBUG_DIR=%s", debugDir))
		envVars = append(envVars, "MCPCHECKER_DEBUG=1")
	}
	cmd.Env = envVars

	res, err := cmd.CombinedOutput()
	if err != nil {
		debugSuffix := ""
		if debugDir != "" {
			debugSuffix = fmt.Sprintf("\n\ndebug artifacts preserved at: %s", debugDir)
		}
		// executionSucceeded remains false, so tempDir will be preserved
		tempDirSuffix := fmt.Sprintf("\n\ntemporary directory preserved at: %s", tempDir)
		return nil, fmt.Errorf("failed to run command: %s -c %q: %w.\n\noutput: %s%s%s", shell, formatted.String(), err, res, debugSuffix, tempDirSuffix)
	}

	executionSucceeded = true

	if debugDir != "" {
		_ = os.RemoveAll(debugDir)
	}

	output := string(res)
	// If MCPCHECKER_DEBUG is set, append temp directory info to output so it appears in JSON log
	if os.Getenv("MCPCHECKER_DEBUG") != "" {
		output += fmt.Sprintf("\n\ntemporary directory preserved at: %s", tempDir)
	}

	return &agentSpecRunnerResult{
		commandOutput: output,
	}, nil
}

func (a *agentSpecRunner) WithMcpServerInfo(mcpServers mcpproxy.ServerManager) Runner {
	return &agentSpecRunner{
		AgentSpec: a.AgentSpec,
		mcpInfo:   mcpServers,
	}
}

func (a *agentSpecRunner) AgentName() string {
	return a.Metadata.Name
}
