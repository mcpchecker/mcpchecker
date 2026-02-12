package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/mcpchecker/mcpchecker/pkg/agent"
	"github.com/mcpchecker/mcpchecker/pkg/eval"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/mcpchecker/mcpchecker/pkg/results"
	"github.com/mcpchecker/mcpchecker/pkg/task"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

const (
	defaultMaxEvents      = 40
	defaultMaxOutputLines = 6
	defaultMaxLineLength  = 100
)

// NewViewCmd creates the view command for rendering eval results.
func NewViewCmd() *cobra.Command {
	var (
		taskFilter     string
		showTimeline   = true
		maxEvents      = defaultMaxEvents
		maxOutputLines = defaultMaxOutputLines
		maxLineLength  = defaultMaxLineLength
	)

	cmd := &cobra.Command{
		Use:   "view <results-file>",
		Short: "Pretty-print evaluation results from a JSON file",
		Long: `Render the JSON output produced by "mcpchecker run" in a human-friendly format.

Examples:
  mcpchecker view mcpchecker-netedge-selector-mismatch-out.json
  mcpchecker view --task netedge-selector-mismatch --max-events 15 results.json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			evalResults, err := results.Load(args[0])
			if err != nil {
				return err
			}

			filtered := results.Filter(evalResults, taskFilter)
			if len(filtered) == 0 {
				if taskFilter == "" {
					return errors.New("no tasks found in results")
				}
				return fmt.Errorf("no tasks matched filter %q", taskFilter)
			}

			for idx, result := range filtered {
				if idx > 0 {
					fmt.Println()
				}
				printEvalResult(result, viewOptions{
					showTimeline:   showTimeline,
					maxEvents:      maxEvents,
					maxOutputLines: maxOutputLines,
					maxLineLength:  maxLineLength,
				})
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&taskFilter, "task", "", "Only show results for tasks whose name contains this value")
	cmd.Flags().BoolVar(&showTimeline, "timeline", showTimeline, "Include a condensed agent timeline derived from taskOutput")
	cmd.Flags().IntVar(&maxEvents, "max-events", maxEvents, "Maximum number of timeline entries (thought/command/tool/etc.) to display (0 = unlimited)")
	cmd.Flags().IntVar(&maxOutputLines, "max-output-lines", maxOutputLines, "Maximum lines to display for command output in the timeline")
	cmd.Flags().IntVar(&maxLineLength, "max-line-length", maxLineLength, "Maximum characters per line when formatting timeline output")

	return cmd
}

// viewOptions controls which portions of a result are rendered and how much detail is shown.
type viewOptions struct {
	showTimeline   bool
	maxEvents      int
	maxOutputLines int
	maxLineLength  int
}

// printEvalResult prints a formatted summary of a single evaluation result.
func printEvalResult(result *eval.EvalResult, opts viewOptions) {
	bold := color.New(color.Bold)
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)

	bold.Printf("Task: %s\n", result.TaskName)
	fmt.Printf("  Path: %s\n", result.TaskPath)
	if result.Difficulty != "" {
		fmt.Printf("  Difficulty: %s\n", result.Difficulty)
	}

	status := "PASSED"
	statusColor := green

	switch {
	case result.AgentExecutionError:
		status = "FAILED (agent error)"
		statusColor = red
	case !result.TaskPassed:
		status = "FAILED"
		statusColor = red
	case result.TaskPassed && !result.AllAssertionsPassed:
		status = "PASSED (assertions failed)"
		statusColor = yellow
	}

	statusColor.Printf("  Status: %s\n", status)
	if trimmed := strings.TrimSpace(result.TaskError); trimmed != "" {
		printMultilineField("Error", trimmed)
	}

	if prompt := loadTaskPrompt(result.TaskPath); prompt != "" {
		printMultilineField("Prompt", prompt)
	}

	printAssertions(result.AssertionResults, yellow)
	printTokenEstimate(result.TokenEstimate)
	printActualAgentTokenUsage(result.TokenEstimate)
	printJudgeTokenUsage(result.JudgeTokenUsage)
	printCallHistory(result.CallHistory, opts)

	if opts.showTimeline {
		timeline := summarizeTaskOutput(result.TaskOutput, opts.maxEvents, opts.maxOutputLines, opts.maxLineLength)
		if len(timeline) > 0 {
			fmt.Println("  Timeline:")
			for _, line := range timeline {
				printTimelineLine(line)
			}
		}
	}
}

// printTokenEstimate prints agent token usage estimates.
func printTokenEstimate(estimate *agent.TokenEstimate) {
	if estimate == nil || estimate.TotalTokens == 0 {
		return
	}

	fmt.Printf("  Estimated Tokens: ~%d (in=~%d, out=~%d)",
		estimate.TotalTokens, estimate.InputTokens, estimate.OutputTokens)
	if estimate.Error != "" {
		fmt.Printf(" [incomplete - %s]", estimate.Error)
	} else {
		fmt.Printf(" [excludes system prompt & cache]")
	}

	// Show breakdown if we have detailed data
	hasDetails := estimate.PromptTokens > 0 || estimate.MessageTokens > 0 ||
		estimate.ThinkingTokens > 0 || estimate.ToolCallTokens > 0 || estimate.ToolResultTokens > 0

	if hasDetails {
		fmt.Printf("\n    input: prompt=~%d", estimate.PromptTokens)
		if estimate.ToolResultTokens > 0 {
			fmt.Printf(", tool_results=~%d", estimate.ToolResultTokens)
		}
		fmt.Printf("\n    output: message=~%d", estimate.MessageTokens)
		if estimate.ThinkingTokens > 0 {
			fmt.Printf(", thinking=~%d", estimate.ThinkingTokens)
		}
		if estimate.ToolCallTokens > 0 {
			fmt.Printf(", tool_calls=~%d", estimate.ToolCallTokens)
		}
	}
	fmt.Println()
}

func printActualAgentTokenUsage(estimate *agent.TokenEstimate) {
	if estimate == nil || estimate.Source != agent.TokenSourceActual || estimate.Actual == nil {
		return
	}

	agentTokenUsage := estimate.Actual
	if agentTokenUsage.InputTokens > 0 || agentTokenUsage.OutputTokens > 0 {
		fmt.Printf("  Agent Tokens: %d (in=%d, out=%d)", agentTokenUsage.TotalTokens, agentTokenUsage.InputTokens, agentTokenUsage.OutputTokens)
		fmt.Println()
	}
}

func printJudgeTokenUsage(judgeTokenUsage *agent.ActualUsage) {
	if judgeTokenUsage != nil && (judgeTokenUsage.InputTokens > 0 || judgeTokenUsage.OutputTokens > 0) {
		fmt.Printf("  Judge Tokens: %d (in=%d, out=%d)", judgeTokenUsage.TotalTokens, judgeTokenUsage.InputTokens, judgeTokenUsage.OutputTokens)
		fmt.Println()
	}
}

// printAssertions prints assertion counts and any failing assertion reasons.
func printAssertions(results *eval.CompositeAssertionResult, warn *color.Color) {
	if results == nil {
		return
	}

	failed := results.FailedAssertions()
	total := results.TotalAssertions()
	if total == 0 {
		return
	}

	if failed == 0 {
		fmt.Printf("  Assertions: %d/%d passed\n", total, total)
		return
	}

	warn.Printf("  Assertions: %d/%d passed\n", total-failed, total)

	val := reflect.ValueOf(results).Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		if field.Kind() != reflect.Ptr || field.IsNil() {
			continue
		}

		res, ok := field.Interface().(*eval.SingleAssertionResult)
		if !ok || res.Passed {
			continue
		}

		fmt.Printf("    • %s: %s\n", fieldType.Name, res.Reason)
		for _, detail := range res.Details {
			fmt.Printf("      %s\n", detail)
		}
	}
}

// printCallHistory emits an aggregated summary of tool/resource/prompt usage.
func printCallHistory(history *mcpproxy.CallHistory, opts viewOptions) {
	if history == nil {
		return
	}

	toolCalls := len(history.ToolCalls)
	resourceReads := len(history.ResourceReads)
	promptGets := len(history.PromptGets)

	if toolCalls == 0 && resourceReads == 0 && promptGets == 0 {
		return
	}

	fmt.Printf("  Call history:")
	if toolCalls > 0 {
		fmt.Printf(" tools=%d", toolCalls)
		if summaries := summarizeToolCalls(history.ToolCalls); summaries != "" {
			fmt.Printf(" (%s)", summaries)
		}
	}
	if resourceReads > 0 {
		fmt.Printf(" resources=%d", resourceReads)
	}
	if promptGets > 0 {
		fmt.Printf(" prompts=%d", promptGets)
	}
	fmt.Println()

	if toolCalls > 0 {
		printToolCallDetails(history.ToolCalls, opts)
	}
}

// printToolCallDetails prints detailed tool call output for timeline inspection.
func printToolCallDetails(calls []*mcpproxy.ToolCall, opts viewOptions) {
	fmt.Println("    Tool output:")
	for _, call := range calls {
		status := "ok"
		if !call.Success {
			status = "fail"
		}
		header := fmt.Sprintf("      • %s::%s (%s)", call.ServerName, call.ToolName, status)
		fmt.Println(header)

		snippet := strings.TrimSpace(extractToolText(call))
		if snippet == "" {
			continue
		}

		lineLimit := opts.maxOutputLines
		width := opts.maxLineLength
		block := limitMultiline(snippet, lineLimit, width)
		for _, line := range strings.Split(block, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			fmt.Printf("        %s\n", line)
		}
	}
}

// extractToolText flattens the mixed content of a tool call into readable text.
func extractToolText(call *mcpproxy.ToolCall) string {
	if call == nil || call.Result == nil {
		return ""
	}

	var builder strings.Builder
	for _, content := range call.Result.Content {
		switch v := content.(type) {
		case *mcp.TextContent:
			builder.WriteString(v.Text)
			if !strings.HasSuffix(v.Text, "\n") {
				builder.WriteString("\n")
			}
		case *mcp.ResourceLink:
			data, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				builder.WriteString(fmt.Sprintf("[ResourceLink marshal error: %v]\n", err))
				continue
			}
			builder.Write(data)
			builder.WriteString("\n")
		case *mcp.EmbeddedResource:
			data, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				builder.WriteString(fmt.Sprintf("[EmbeddedResource marshal error: %v]\n", err))
				continue
			}
			builder.Write(data)
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

// summarizeToolCalls groups tool calls by server and success outcome into a compact string.
func summarizeToolCalls(calls []*mcpproxy.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}

	type key struct {
		server  string
		success bool
	}

	counts := make(map[key]int)
	for _, call := range calls {
		callKey := key{server: call.ServerName, success: call.Success}
		counts[callKey]++
	}

	type serverSummary struct {
		server  string
		success bool
		count   int
	}

	summaries := make([]serverSummary, 0, len(counts))
	for k, v := range counts {
		summaries = append(summaries, serverSummary{
			server:  k.server,
			success: k.success,
			count:   v,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].server == summaries[j].server {
			return summaries[i].success && !summaries[j].success
		}
		return summaries[i].server < summaries[j].server
	})

	parts := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		state := "ok"
		if !summary.success {
			state = "fail"
		}
		parts = append(parts, fmt.Sprintf("%s:%d %s", summary.server, summary.count, state))
	}

	return strings.Join(parts, ", ")
}

// agentEvent represents a single event emitted by the agent JSON log stream.
type agentEvent struct {
	Type    string          `json:"type"`
	Item    json.RawMessage `json:"item,omitempty"`
	Message string          `json:"message,omitempty"`
}

// agentItem captures the payload attached to an agent event.
type agentItem struct {
	ID               string      `json:"id"`
	Type             string      `json:"type"`
	Text             string      `json:"text,omitempty"`
	Command          string      `json:"command,omitempty"`
	AggregatedOutput string      `json:"aggregated_output,omitempty"`
	Status           string      `json:"status,omitempty"`
	Server           string      `json:"server,omitempty"`
	Tool             string      `json:"tool,omitempty"`
	ExitCode         *int        `json:"exit_code,omitempty"`
	Items            []todoEntry `json:"items,omitempty"`
}

// todoEntry models a single task entry inside an agent todo list.
type todoEntry struct {
	Text      string `json:"text"`
	Completed bool   `json:"completed"`
}

// summarizeTaskOutput condenses raw agent event lines into human-readable timeline entries.
func summarizeTaskOutput(raw string, maxEvents, maxOutputLines, maxLineLength int) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var summaries []string
	if isLikelyJSONTaskOutput(raw) {
		summaries = summarizeJSONTaskOutput(raw, maxOutputLines, maxLineLength)
	} else {
		summaries = summarizePlaintextTaskOutput(raw, maxOutputLines, maxLineLength)
	}

	if maxEvents > 0 && len(summaries) > maxEvents {
		extra := len(summaries) - maxEvents
		summaries = append(summaries[:maxEvents], fmt.Sprintf("… %d additional events omitted", extra))
	}

	return summaries
}

// formatEvent converts an agent event into a concise timeline string, if applicable.
func formatEvent(evt agentEvent, maxOutputLines, maxLineLength int) string {
	switch evt.Type {
	case "thread.started", "turn.started", "turn.completed":
		return ""
	}

	if evt.Type != "item.completed" && evt.Type != "item.failed" && evt.Type != "item.updated" && evt.Type != "item.started" {
		if evt.Message != "" {
			msg := evt.Message
			if wrapped := wrapText(msg, maxLineLength); wrapped != "" {
				return wrapped
			}
			return msg
		}
		return ""
	}

	if len(evt.Item) == 0 {
		return ""
	}

	var item agentItem
	if err := json.Unmarshal(evt.Item, &item); err != nil {
		// Fallback for Gemini stream-json events where the event IS the item
		// Gemini stream events: {"type": "message", "content": "..."} or {"type": "tool_use", ...}
		type GeminiEvent struct {
			Type     string          `json:"type"`
			Role     string          `json:"role"`
			Content  string          `json:"content"`
			ToolName string          `json:"tool_name"`
			Params   json.RawMessage `json:"parameters"`
		}
		var gEvt GeminiEvent
		if err2 := json.Unmarshal(evt.Item, &gEvt); err2 == nil && gEvt.Type != "" {
			switch gEvt.Type {
			case "message":
				if gEvt.Role == "assistant" && gEvt.Content != "" {
					return fmt.Sprintf("assistant: %s", wrapText(gEvt.Content, maxLineLength))
				}
				if gEvt.Role == "user" && gEvt.Content != "" {
					return fmt.Sprintf("user: %s", wrapText(gEvt.Content, maxLineLength))
				}
				return ""
			case "tool_use":
				return fmt.Sprintf("tool call: %s", gEvt.ToolName)
			case "tool_result":
				return "tool result"
			}
		}
		return ""
	}

	if evt.Type == "item.started" {
		switch item.Type {
		case "command_execution", "mcp_tool_call":
			return ""
		}
	}

	switch item.Type {
	case "reasoning":
		text := normalizeWhitespace(item.Text)
		text = wrapText(text, maxLineLength)
		return fmt.Sprintf("thought: %s", text)
	case "command_execution":
		summary := fmt.Sprintf("command: %s", item.Command)
		if item.Status != "" {
			summary = fmt.Sprintf("%s (%s)", summary, item.Status)
		}
		if item.ExitCode != nil {
			summary = fmt.Sprintf("%s exit=%d", summary, *item.ExitCode)
		}
		summary = wrapText(summary, maxLineLength)
		if item.AggregatedOutput != "" {
			block := limitMultiline(item.AggregatedOutput, maxOutputLines, maxLineLength)
			if block != "" {
				summary = fmt.Sprintf("%s\n%s", summary, indentBlock(block, "      "))
			}
		}
		return summary
	case "mcp_tool_call":
		if item.Server == "" && item.Tool == "" {
			return "tool call"
		}
		detail := fmt.Sprintf("tool: %s::%s", item.Server, item.Tool)
		if item.Status != "" {
			detail = fmt.Sprintf("%s (%s)", detail, item.Status)
		}
		return detail
	case "todo_list":
		count := len(item.Items)
		if count == 0 {
			return "plan: todo list started"
		}
		headline := normalizeWhitespace(item.Items[0].Text)
		headline = wrapText(headline, maxLineLength)
		if count == 1 {
			return fmt.Sprintf("plan: %s", headline)
		}
		return fmt.Sprintf("plan: %d tasks (%s)", count, headline)
	default:
		return fmt.Sprintf("%s event", item.Type)
	}
}

func isLikelyJSONTaskOutput(raw string) bool {
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{") {
			return true
		}
	}
	return false
}

func summarizeJSONTaskOutput(raw string, maxOutputLines, maxLineLength int) []string {
	lines := strings.Split(raw, "\n")
	summaries := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Skip non-JSON lines (e.g. YOLO mode preamble)
		if !strings.HasPrefix(trimmed, "{") {
			continue
		}

		var evt agentEvent
		// Attempt to parse standard agent event (wrapped in "item")
		// We treat it as a standard event ONLY if it has a populated "item" field or is a known structure
		// Gemini events are flat, so they have "type" but NO "item".
		if err := json.Unmarshal([]byte(trimmed), &evt); err == nil && len(evt.Item) > 0 {
			if summary := formatEvent(evt, maxOutputLines, maxLineLength); summary != "" {
				summaries = append(summaries, summary)
			}
			continue
		}
		// If that fails, try parsing as a direct Gemini event (flat JSON)
		type GeminiEvent struct {
			Type     string `json:"type"`
			Role     string `json:"role"`
			Content  string `json:"content"`
			ToolName string `json:"tool_name"`
			Output   string `json:"output"`
		}
		var gEvt GeminiEvent
		if err2 := json.Unmarshal([]byte(trimmed), &gEvt); err2 == nil && gEvt.Type != "" {
			switch gEvt.Type {
			case "message":
				if gEvt.Role == "assistant" && gEvt.Content != "" {
					summaries = append(summaries, fmt.Sprintf("assistant: %s", wrapText(gEvt.Content, maxLineLength)))
				} else if gEvt.Role == "user" && gEvt.Content != "" {
					summaries = append(summaries, fmt.Sprintf("user: %s", wrapText(gEvt.Content, maxLineLength)))
				}
			case "tool_use":
				summaries = append(summaries, fmt.Sprintf("tool call: %s", gEvt.ToolName))
			case "tool_result":
				summary := fmt.Sprintf("tool result: %s", gEvt.Output)
				summaries = append(summaries, limitMultiline(summary, maxOutputLines, maxLineLength))
			}
			continue
		}

		summaries = append(summaries, fmt.Sprintf("unparsed event: %s", truncateString(trimmed, maxLineLength)))
		continue

	}

	return summaries
}

func summarizePlaintextTaskOutput(raw string, maxOutputLines, maxLineLength int) []string {
	lines := strings.Split(raw, "\n")
	// Check if we have any known headers; if not, treat the whole thing as content
	hasHeaders := false
	for _, line := range lines {
		if plaintextIsEventHeader(line) {
			hasHeaders = true
			break
		}
	}

	i := 0
	if hasHeaders {
		for i < len(lines) {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "" {
				i++
				continue
			}
			if plaintextIsEventHeader(trimmed) {
				break
			}
			i++
		}
	}

	summaries := make([]string, 0, len(lines))
	lastAssistant := ""

	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			continue
		}

		header := strings.ToLower(strings.TrimSuffix(line, ":"))

		switch header {
		case "thinking", "analysis", "reasoning":
			i++
			block, next := collectPlaintextBlock(lines, i, false)
			i = next
			if len(block) == 0 {
				continue
			}

			textParts := make([]string, 0, len(block))
			for _, segment := range block {
				textParts = append(textParts, stripMarkdownEmphasis(strings.TrimSpace(segment)))
			}

			text := normalizeWhitespace(strings.Join(textParts, " "))
			if text == "" {
				continue
			}

			wrapped := wrapText(text, maxLineLength)
			summaries = append(summaries, fmt.Sprintf("thought: %s", wrapped))

		case "plan", "todo":
			i++
			block, next := collectPlaintextBlock(lines, i, false)
			i = next
			if len(block) == 0 {
				continue
			}

			items := make([]string, 0, len(block))
			for _, segment := range block {
				items = append(items, normalizeWhitespace(stripMarkdownEmphasis(segment)))
			}

			headline := wrapText(items[0], maxLineLength)
			if len(items) == 1 {
				summaries = append(summaries, fmt.Sprintf("plan: %s", headline))
			} else {
				summaries = append(summaries, fmt.Sprintf("plan: %d steps (%s)", len(items), headline))
			}

		case "exec", "command":
			i++
			var summaryLine string
			if i < len(lines) {
				summaryLine = strings.TrimSpace(lines[i])
				i++
			}

			summary := summarizePlaintextCommand(summaryLine, maxLineLength)

			output, next := collectPlaintextBlock(lines, i, true)
			i = next
			if len(output) > 0 {
				block := limitMultiline(strings.Join(output, "\n"), maxOutputLines, maxLineLength)
				if block != "" {
					summary = fmt.Sprintf("%s\n%s", summary, indentBlock(block, "      "))
				}
			}

			summaries = append(summaries, summary)

		case "tool":
			i++
			var toolLine string
			if i < len(lines) {
				toolLine = strings.TrimSpace(lines[i])
				i++
			}

			summary := summarizePlaintextTool(toolLine, maxLineLength)

			output, next := collectPlaintextBlock(lines, i, true)
			i = next
			if len(output) > 0 {
				block := limitMultiline(strings.Join(output, "\n"), maxOutputLines, maxLineLength)
				if block != "" {
					summary = fmt.Sprintf("%s\n%s", summary, indentBlock(block, "      "))
				}
			}

			summaries = append(summaries, summary)

		case "assistant", "codex":
			i++
			block, next := collectPlaintextBlock(lines, i, false)
			i = next
			if len(block) == 0 {
				continue
			}

			textParts := make([]string, 0, len(block))
			for _, segment := range block {
				textParts = append(textParts, normalizeWhitespace(stripMarkdownEmphasis(segment)))
			}

			text := normalizeWhitespace(strings.Join(textParts, " "))
			if text == "" {
				continue
			}

			lastAssistant = text
			wrapped := wrapText(text, maxLineLength)
			summaries = append(summaries, fmt.Sprintf("assistant: %s", wrapped))

		case "user", "system":
			i++
			_, next := collectPlaintextBlock(lines, i, false)
			i = next

		case "tokens used", "token usage":
			i++
			if i < len(lines) {
				i++
			}

		default:
			block, next := collectPlaintextBlock(lines, i, false)
			if len(block) == 0 {
				i++
				continue
			}

			text := normalizeWhitespace(strings.Join(block, " "))
			if text == "" || (lastAssistant != "" && text == lastAssistant) {
				i = next
				continue
			}

			wrapped := wrapText(text, maxLineLength)
			summaries = append(summaries, fmt.Sprintf("note: %s", wrapped))
			i = next
		}
	}

	return summaries
}

func collectPlaintextBlock(lines []string, start int, allowBlank bool) ([]string, int) {
	block := make([]string, 0)
	i := start

	for i < len(lines) {
		raw := strings.TrimRight(lines[i], "\r")
		trimmed := strings.TrimSpace(raw)

		if trimmed == "" {
			if allowBlank {
				block = append(block, "")
				i++
				continue
			}
			i++
			break
		}

		if plaintextIsEventHeader(trimmed) {
			break
		}

		block = append(block, raw)
		i++
	}

	return block, i
}

func plaintextIsEventHeader(line string) bool {
	line = strings.TrimSpace(line)
	line = strings.TrimSuffix(line, ":")
	lower := strings.ToLower(line)

	switch lower {
	case "analysis", "assistant", "codex", "command", "commentary", "exec", "observation", "plan", "reasoning", "thinking", "thought", "todo", "tool", "tokens used", "token usage", "user", "system":
		return true
	default:
		return false
	}
}

func summarizePlaintextCommand(line string, maxLineLength int) string {
	line = strings.TrimSpace(line)
	line = strings.TrimSuffix(line, ":")

	if line == "" {
		return "command"
	}

	cmd := line
	if idx := strings.Index(line, " in "); idx != -1 {
		cmd = line[:idx]
	}

	status := ""
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "succeeded"):
		status = "completed"
	case strings.Contains(lower, "failed"):
		status = "failed"
	case strings.Contains(lower, "canceled"), strings.Contains(lower, "cancelled"):
		status = "cancelled"
	}

	summary := fmt.Sprintf("command: %s", strings.TrimSpace(cmd))
	if status != "" {
		summary = fmt.Sprintf("%s (%s)", summary, status)
	}

	return wrapText(summary, maxLineLength)
}

func summarizePlaintextTool(line string, maxLineLength int) string {
	line = strings.TrimSpace(line)
	line = strings.TrimSuffix(line, ":")
	if line == "" {
		return "tool call"
	}

	base := line
	if idx := strings.Index(line, " in "); idx != -1 {
		base = line[:idx]
	}

	summary := fmt.Sprintf("tool: %s", strings.TrimSpace(base))
	return wrapText(summary, maxLineLength)
}

func stripMarkdownEmphasis(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "*`_")
	return strings.TrimSpace(s)
}

// limitMultiline trims a block to the requested number of lines and line length, wrapping as needed.
func limitMultiline(raw string, maxLines, maxLineLength int) string {
	raw = strings.TrimRight(raw, "\n")
	if raw == "" {
		return ""
	}

	lines := strings.Split(raw, "\n")
	limited := make([]string, 0, len(lines))
	for i, line := range lines {
		segments := splitWrappedLines(line, maxLineLength)
		for j, segment := range segments {
			if maxLines > 0 && len(limited) >= maxLines {
				remaining := len(segments) - j
				for _, future := range lines[i+1:] {
					remaining += len(splitWrappedLines(future, maxLineLength))
				}
				if remaining > 0 {
					limited = append(limited, fmt.Sprintf("… (+%d lines)", remaining))
				}
				return strings.Join(limited, "\n")
			}
			limited = append(limited, segment)
		}
	}

	return strings.Join(limited, "\n")
}

// splitWrappedLines wraps a single line to the max width and returns its segments.
func splitWrappedLines(line string, maxLineLength int) []string {
	if maxLineLength > 0 {
		return strings.Split(wrapText(line, maxLineLength), "\n")
	}
	return []string{line}
}

// truncateString shortens s to at most max characters, appending an ellipsis when truncated.
func truncateString(s string, max int) string {
	if max <= 0 {
		return s
	}

	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}

	trimmed := strings.TrimSpace(string(runes[:max-1]))
	return fmt.Sprintf("%s…", trimmed)
}

// indentBlock prefixes each line in block with indent.
func indentBlock(block, indent string) string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

// normalizeWhitespace collapses whitespace and removes simple emphasis markers.
func normalizeWhitespace(in string) string {
	in = strings.ReplaceAll(in, "\n", " ")
	in = strings.ReplaceAll(in, "\t", " ")
	in = strings.ReplaceAll(in, "**", "")
	fields := strings.Fields(in)
	return strings.Join(fields, " ")
}

// wrapText breaks s into multiple lines no wider than width characters.
func wrapText(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}

	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}

	lines := make([]string, 0)
	current := words[0]

	for _, word := range words[1:] {
		if len(current)+1+len(word) > width {
			lines = append(lines, current)
			current = word
		} else {
			current += " " + word
		}
	}
	lines = append(lines, current)

	return strings.Join(lines, "\n")
}

// loadTaskPrompt returns the prompt text defined in the task manifest, if present.
func loadTaskPrompt(taskPath string) string {
	if taskPath == "" {
		return ""
	}

	taskConfig, err := task.FromFile(taskPath)
	if err != nil || taskConfig == nil || taskConfig.Spec == nil || taskConfig.Spec.Prompt == nil || taskConfig.Spec.Prompt.IsEmpty() {
		return ""
	}

	text, err := taskConfig.Spec.Prompt.GetValue()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(text)
}

// printMultilineField prints a label/value pair, indenting multi-line values neatly.
func printMultilineField(label, value string) {
	value = strings.TrimRight(value, "\n")
	// Clean up specific artifact from some agent logs where exit status wraps oddly
	value = strings.ReplaceAll(value, "\n': exit status", " exit status")
	if !strings.Contains(value, "\n") {
		fmt.Printf("  %s: %s\n", label, value)
		return
	}

	fmt.Printf("  %s:\n", label)
	lines := mergeContinuationLines(strings.Split(value, "\n"))
	for _, line := range lines {
		fmt.Printf("    %s\n", line)
	}
}

// mergeContinuationLines rejoins log lines that were split across multiple rows.
// This handles cases where error messages or specific log formats wrap unexpectedly,
// often starting the next line with a punctuation mark like ' or :.
func mergeContinuationLines(lines []string) []string {
	merged := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if len(merged) > 0 {
			switch trimmed[0] {
			case '\'', '"', ')', '.', ':':
				merged[len(merged)-1] = merged[len(merged)-1] + " " + trimmed
				continue
			}
		}

		merged = append(merged, trimmed)
	}

	for i, line := range merged {
		line = strings.ReplaceAll(line, "' : exit", "' exit")
		line = strings.ReplaceAll(line, "\" : exit", "\" exit")
		merged[i] = line
	}

	return merged
}

// printTimelineLine prints a timeline entry and any subsequent indented lines.
func printTimelineLine(entry string) {
	parts := strings.Split(entry, "\n")
	if len(parts) == 0 {
		return
	}

	fmt.Printf("    - %s\n", parts[0])
	for _, part := range parts[1:] {
		if strings.TrimSpace(part) == "" {
			continue
		}
		clean := part
		if strings.HasPrefix(clean, "      ") {
			clean = strings.TrimPrefix(clean, "      ")
		}
		fmt.Printf("      %s\n", clean)
	}
}
