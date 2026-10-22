package mcpproxy

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/mcpchecker/mcpchecker/pkg/tokenizer"
)

// ComputeCallHistoryTokens populates token counts on each call record in the history.
// Returns an error description if any token counting failed, empty string on success.
func ComputeCallHistoryTokens(history *CallHistory) string {
	if history == nil {
		return ""
	}

	tok := tokenizer.Get()
	var errors []string

	for _, tc := range history.ToolCalls {
		var inputTokens, outputTokens int64

		// Count input tokens (request arguments)
		if tc.Request != nil {
			if count, err := tok.CountJSONTokens(tc.Request.Params.Arguments); err != nil {
				log.Printf("Warning: failed to count tool call input tokens for %q: %v", tc.ToolName, err)
				errors = append(errors, fmt.Sprintf("tool_input:%s", tc.ToolName))
			} else {
				inputTokens = int64(count)
			}
		}

		// Count output tokens (result content)
		if tc.Result != nil {
			if count, err := tok.CountJSONTokens(tc.Result.Content); err != nil {
				log.Printf("Warning: failed to count tool call output tokens for %q: %v", tc.ToolName, err)
				errors = append(errors, fmt.Sprintf("tool_output:%s", tc.ToolName))
			} else {
				outputTokens = int64(count)
			}
		}

		tc.Tokens = NewTokenCount(inputTokens, outputTokens)
	}

	for _, rr := range history.ResourceReads {
		if rr.HostUIRead {
			rr.Tokens = nil
			continue
		}
		var inputTokens, outputTokens int64

		// Count input tokens (request params - URI)
		if rr.Request != nil {
			if count, err := tok.CountTokens(rr.Request.Params.URI); err != nil {
				log.Printf("Warning: failed to count resource read input tokens for %q: %v", rr.URI, err)
				errors = append(errors, fmt.Sprintf("resource_input:%s", rr.URI))
			} else {
				inputTokens = int64(count)
			}
		}

		// Count output tokens (result contents)
		if rr.Result != nil {
			if count, err := tok.CountJSONTokens(rr.Result.Contents); err != nil {
				log.Printf("Warning: failed to count resource read output tokens for %q: %v", rr.URI, err)
				errors = append(errors, fmt.Sprintf("resource_output:%s", rr.URI))
			} else {
				outputTokens = int64(count)
			}
		}

		rr.Tokens = NewTokenCount(inputTokens, outputTokens)
	}

	for _, pg := range history.PromptGets {
		var inputTokens, outputTokens int64

		// Count input tokens (request arguments)
		if pg.Request != nil {
			if count, err := tok.CountJSONTokens(pg.Request.Params.Arguments); err != nil {
				log.Printf("Warning: failed to count prompt get input tokens for %q: %v", pg.Name, err)
				errors = append(errors, fmt.Sprintf("prompt_input:%s", pg.Name))
			} else {
				inputTokens = int64(count)
			}
		}

		// Count output tokens (result messages)
		if pg.Result != nil {
			if count, err := tok.CountJSONTokens(pg.Result.Messages); err != nil {
				log.Printf("Warning: failed to count prompt get output tokens for %q: %v", pg.Name, err)
				errors = append(errors, fmt.Sprintf("prompt_output:%s", pg.Name))
			} else {
				outputTokens = int64(count)
			}
		}

		pg.Tokens = NewTokenCount(inputTokens, outputTokens)
	}

	if len(errors) > 0 {
		return fmt.Sprintf("failed to count: %s", strings.Join(errors, ", "))
	}
	return ""
}

// countTextWithErrors counts tokens in text, logging warnings and appending error labels on failure.
func countTextWithErrors(tok tokenizer.Tokenizer, text string, label string, errors *[]string) int64 {
	count, err := tok.CountTokens(text)
	if err != nil {
		log.Printf("Warning: failed to count tokens for %s: %v", label, err)
		*errors = append(*errors, label)
		return 0
	}
	return int64(count)
}

// ComputeSchemaTokens counts tokens for all tool definitions and server instructions
// across the given servers. These represent the MCP server's schema overhead sent to
// the LLM on each API call.
func ComputeSchemaTokens(ctx context.Context, servers []Server) (int64, error) {
	tok := tokenizer.Get()
	var total int64
	var errors []string

	for _, srv := range servers {
		// Count server instructions
		if instructions := srv.GetInstructions(); instructions != "" {
			total += countTextWithErrors(tok, instructions, fmt.Sprintf("instructions:%s", srv.GetName()), &errors)
		}

		// Count tool definitions (name + description + inputSchema)
		tools, err := srv.GetAllowedTools(ctx)
		if err != nil {
			return total, fmt.Errorf("failed to discover allowed tools for MCP server %q: %w", srv.GetName(), err)
		}
		for _, tool := range tools {
			// Count name + description together
			text := tool.Name
			if tool.Description != "" {
				text += " " + tool.Description
			}
			total += countTextWithErrors(tok, text, fmt.Sprintf("tool_def:%s", tool.Name), &errors)

			// Count inputSchema
			if tool.InputSchema != nil {
				if count, err := tok.CountJSONTokens(tool.InputSchema); err != nil {
					log.Printf("Warning: failed to count tool schema tokens for %q: %v", tool.Name, err)
					errors = append(errors, fmt.Sprintf("tool_schema:%s", tool.Name))
				} else {
					total += int64(count)
				}
			}
		}
	}

	if len(errors) > 0 {
		return total, fmt.Errorf("failed to count: %s", strings.Join(errors, ", "))
	}
	return total, nil
}
