package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/coder/acp-go-sdk"
	"github.com/mcpchecker/mcpchecker/pkg/acpclient"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/mcpchecker/mcpchecker/pkg/tokenizer"
	"github.com/mcpchecker/mcpchecker/pkg/tokens"
	"github.com/mcpchecker/mcpchecker/pkg/util"
)

type Runner interface {
	RunTask(ctx context.Context, prompt string) (AgentResult, error)
	WithMcpServerInfo(mcpServers mcpproxy.ServerManager) Runner
	WithSkillInfo(skills *SkillInfo) Runner
	AgentName() string
}

// SkillInfo contains skill mounting information for the agent runner.
// Implements acpclient.SkillInfo.
type SkillInfo struct {
	// MountPath is the relative path within the agent's working directory
	// where skill files should be placed (e.g., ".claude/skills")
	MountPath string

	// SourceDirs contains absolute paths to skill source directories
	// whose contents will be copied into the mount path
	SourceDirs []string
}

func (s *SkillInfo) GetMountPath() string    { return s.MountPath }
func (s *SkillInfo) GetSourceDirs() []string { return s.SourceDirs }

// ClientOptions returns ACP client options for skill mounting.
// Safe to call on a nil receiver (returns nil).
func (s *SkillInfo) ClientOptions() []acpclient.ClientOption {
	if s == nil {
		return nil
	}
	return []acpclient.ClientOption{acpclient.WithSkills(s)}
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

// OutputStep represents a single logical step in the agent's output.
type OutputStep struct {
	Type     string           `json:"type"`              // "thinking", "message", "tool_call"
	Content  string           `json:"content,omitempty"` // text for thinking/message steps
	ToolCall *ToolCallSummary `json:"toolCall,omitempty"`
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

// ExtractOutputSteps processes ACP session updates into chronological OutputStep slices.
// Consecutive thinking chunks are consolidated into a single "thinking" step,
// consecutive message chunks into a single "message" step, and tool calls
// (deduplicated by ID, merged with ToolCallUpdate data) become "tool_call" steps.
// Non-consecutive runs of the same type produce separate steps.
func ExtractOutputSteps(updates []acp.SessionUpdate) []OutputStep {
	var steps []OutputStep
	var currentType string
	var buf strings.Builder

	// Track tool calls for dedup/merge, same pattern as ExtractToolCalls
	toolCallMap := make(map[acp.ToolCallId]*ToolCallSummary)
	var toolCallOrder []acp.ToolCallId

	flush := func() {
		if currentType == "" {
			return
		}
		if currentType == "thinking" || currentType == "message" {
			if buf.Len() > 0 {
				steps = append(steps, OutputStep{
					Type:    currentType,
					Content: buf.String(),
				})
				buf.Reset()
			}
		}
		currentType = ""
	}

	for _, update := range updates {
		isThinking := update.AgentThoughtChunk != nil && update.AgentThoughtChunk.Content.Text != nil
		isMessage := update.AgentMessageChunk != nil && update.AgentMessageChunk.Content.Text != nil
		isToolCall := update.ToolCall != nil
		isToolCallUpdate := update.ToolCallUpdate != nil

		if isThinking {
			if currentType != "thinking" {
				flush()
				currentType = "thinking"
			}
			buf.WriteString(update.AgentThoughtChunk.Content.Text.Text)
		}

		if isMessage {
			if currentType != "message" {
				flush()
				currentType = "message"
			}
			buf.WriteString(update.AgentMessageChunk.Content.Text.Text)
		}

		if isToolCall {
			flush()
			id := update.ToolCall.ToolCallId
			if _, exists := toolCallMap[id]; !exists {
				toolCallOrder = append(toolCallOrder, id)
				tc := &ToolCallSummary{
					Title:     update.ToolCall.Title,
					Kind:      string(update.ToolCall.Kind),
					Status:    string(update.ToolCall.Status),
					RawInput:  update.ToolCall.RawInput,
					RawOutput: update.ToolCall.RawOutput,
				}
				toolCallMap[id] = tc
				steps = append(steps, OutputStep{
					Type:     "tool_call",
					ToolCall: tc,
				})
			} else {
				tc := toolCallMap[id]
				if update.ToolCall.Title != "" {
					tc.Title = update.ToolCall.Title
				}
				if string(update.ToolCall.Kind) != "" {
					tc.Kind = string(update.ToolCall.Kind)
				}
				if string(update.ToolCall.Status) != "" {
					tc.Status = string(update.ToolCall.Status)
				}
				if update.ToolCall.RawInput != nil {
					tc.RawInput = update.ToolCall.RawInput
				}
				if update.ToolCall.RawOutput != nil {
					tc.RawOutput = update.ToolCall.RawOutput
				}
			}
		}

		if isToolCallUpdate {
			id := update.ToolCallUpdate.ToolCallId
			tc, exists := toolCallMap[id]
			if !exists {
				flush()
				toolCallOrder = append(toolCallOrder, id)
				tc = &ToolCallSummary{}
				toolCallMap[id] = tc
				steps = append(steps, OutputStep{
					Type:     "tool_call",
					ToolCall: tc,
				})
			}
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

	flush()
	return steps
}

// FinalMessageFromSteps returns the content of the last "message"-type OutputStep.
func FinalMessageFromSteps(steps []OutputStep) string {
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Type == "message" {
			return steps[i].Content
		}
	}
	return ""
}

// turnBuilder accumulates session update data and produces per-turn token counts.
type turnBuilder struct {
	tok            tokenizer.Tokenizer
	thinking       strings.Builder
	message        strings.Builder
	numToolCalls   int
	started        bool
	seenResults    bool
	turns          []tokens.TurnTokens
}

func newTurnBuilder() *turnBuilder {
	return &turnBuilder{tok: tokenizer.Get()}
}

func (tb *turnBuilder) flush() {
	thinkingCount, _ := tb.tok.CountTokens(tb.thinking.String())
	messageCount, _ := tb.tok.CountTokens(tb.message.String())
	tb.turns = append(tb.turns, tokens.TurnTokens{
		OutputTokens: int64(thinkingCount) + int64(messageCount),
		NumToolCalls: tb.numToolCalls,
	})
	tb.thinking.Reset()
	tb.message.Reset()
	tb.numToolCalls = 0
	tb.seenResults = false
	tb.started = false
}

// ExtractTurns identifies LLM turns from the session update stream.
// Each turn consists of thinking + message output followed by zero or more
// tool calls. Parallel tool calls (multiple ToolCall events before any
// results arrive) are grouped into a single turn.
func ExtractTurns(updates []acp.SessionUpdate) []tokens.TurnTokens {
	tb := newTurnBuilder()

	for _, u := range updates {
		isThinking := u.AgentThoughtChunk != nil && u.AgentThoughtChunk.Content.Text != nil
		isMessage := u.AgentMessageChunk != nil && u.AgentMessageChunk.Content.Text != nil

		// Thinking or message after tool results means a new turn has started.
		if (isThinking || isMessage) && tb.seenResults {
			tb.flush()
		}

		if isThinking {
			tb.started = true
			tb.thinking.WriteString(u.AgentThoughtChunk.Content.Text.Text)
		}
		if isMessage {
			tb.started = true
			tb.message.WriteString(u.AgentMessageChunk.Content.Text.Text)
		}
		if u.ToolCall != nil {
			tb.started = true
			tb.numToolCalls++
		}
		if u.ToolCallUpdate != nil && u.ToolCallUpdate.RawOutput != nil {
			tb.seenResults = true
		}
	}

	if tb.started {
		tb.flush()
	}

	return tb.turns
}

// AgentResult provides access to the results of an agent execution.
type AgentResult interface {
	GetOutput() []OutputStep
	GetToolCalls() []ToolCallSummary
	GetRawUpdates() any
	GetTokenEstimate() tokens.Estimate
}

type agentSpecRunner struct {
	*AgentSpec
	mcpInfo McpServerInfo
	skills  *SkillInfo
}

type agentSpecRunnerResult struct {
	commandOutput string
}

func (a *agentSpecRunnerResult) GetOutput() []OutputStep {
	return []OutputStep{{Type: "message", Content: a.commandOutput}}
}

func (a *agentSpecRunnerResult) GetToolCalls() []ToolCallSummary {
	return nil // Shell runner doesn't have structured tool call data
}

func (a *agentSpecRunnerResult) GetRawUpdates() any {
	return nil // Shell runner doesn't have session updates
}

func (a *agentSpecRunnerResult) GetTokenEstimate() tokens.Estimate {
	return tokens.Estimate{Error: "token estimation not supported for shell runner"}
}

func NewRunnerForSpec(spec *AgentSpec) (Runner, error) {
	if spec == nil {
		return nil, fmt.Errorf("cannot create a Runner for a nil AgentSpec")
	}

	// check first for acp config
	if spec.AcpConfig != nil {
		return NewAcpRunner(spec.AcpConfig, spec.Metadata.Name), nil
	}

	// Check if this is an LLM agent (or a deprecated alias)
	if spec.Builtin != nil {
		switch spec.Builtin.Type {
		case "llm-agent", "openai-agent", "openai-acp":
			model := spec.Builtin.Model

			if spec.Builtin.Type != "llm-agent" {
				fmt.Fprintf(os.Stderr, "\nWARNING: The %q agent type is deprecated and will be removed in a future release.\n"+
					"  Migrate to \"llm-agent\" with model in \"provider:model-id\" format:\n"+
					"    - In eval config: type: \"builtin.llm-agent\", model: \"openai:<model>\"\n"+
					"    - In agent YAML: type: \"llm-agent\", model: \"openai:<model>\"\n"+
					"  Use provider-specific env vars (e.g. OPENAI_API_KEY) instead of MODEL_KEY.\n\n",
					spec.Builtin.Type)

				// Convert bare model name to provider:model format for backwards compat
				if model != "" && !strings.Contains(model, ":") {
					model = "openai:" + model
				}
			}

			migrateLegacyEnvVars(spec.Builtin)
			return NewLLMACPRunner(model)
		}
	}

	// Use the standard shell-based runner for all other agents
	return &agentSpecRunner{
		AgentSpec: spec,
	}, nil
}

// migrateLegacyEnvVars migrates deprecated env vars and builtin ref fields
// to the provider-specific env vars expected by the llmagent package.
// Only sets new env vars if they are not already set.
func migrateLegacyEnvVars(ref *BuiltinRef) {
	setIfEmpty := func(newVar, value, source string) {
		if value == "" {
			return
		}
		if os.Getenv(newVar) != "" {
			return
		}
		fmt.Fprintf(os.Stderr, "WARNING: %s is deprecated. Set %s directly instead.\n", source, newVar)
		os.Setenv(newVar, value)
	}

	// Migrate base URL: ref.BaseURL holds a literal URL value
	if ref.BaseURL != "" {
		setIfEmpty("OPENAI_BASE_URL", ref.BaseURL, "builtin baseUrl field")
	}
	// Fall back to MODEL_BASE_URL env var
	setIfEmpty("OPENAI_BASE_URL", os.Getenv("MODEL_BASE_URL"), "MODEL_BASE_URL")

	// Migrate API key: ref.APIKey references an env var name containing the key
	if ref.APIKey != "" {
		setIfEmpty("OPENAI_API_KEY", os.Getenv(ref.APIKey), fmt.Sprintf("builtin apiKey (%s)", ref.APIKey))
	}
	// Fall back to MODEL_KEY env var
	setIfEmpty("OPENAI_API_KEY", os.Getenv("MODEL_KEY"), "MODEL_KEY")
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

	// Mount skills into the temp directory if configured
	if a.skills != nil {
		if err := util.MountSkills(tempDir, a.skills.MountPath, a.skills.SourceDirs); err != nil {
			return nil, fmt.Errorf("failed to mount skills: %w", err)
		}
	}

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
		skills:    a.skills,
	}
}

func (a *agentSpecRunner) WithSkillInfo(skills *SkillInfo) Runner {
	return &agentSpecRunner{
		AgentSpec: a.AgentSpec,
		mcpInfo:   a.mcpInfo,
		skills:    skills,
	}
}

func (a *agentSpecRunner) AgentName() string {
	return a.Metadata.Name
}

func toolCallSummaryToToolCallData(summaries []ToolCallSummary) []tokens.ToolCallData {
	res := make([]tokens.ToolCallData, len(summaries))
	for i, summary := range summaries {
		res[i] = tokens.ToolCallData{
			Name:      summary.Title,
			RawInput:  summary.RawInput,
			RawOutput: summary.RawOutput,
		}
	}

	return res
}
