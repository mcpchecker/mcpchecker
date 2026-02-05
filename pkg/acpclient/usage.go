package acpclient

import (
	"log"
	"strconv"

	"github.com/coder/acp-go-sdk"
)

// Usage represents token usage data from an agent.
type Usage struct {
	InputTokens       int64  `json:"input_tokens"`
	OutputTokens      int64  `json:"output_tokens"`
	TotalTokens       int64  `json:"total_tokens"`
	ThoughtTokens     *int64 `json:"thought_tokens,omitempty"`
	CachedReadTokens  *int64 `json:"cached_read_tokens,omitempty"`
	CachedWriteTokens *int64 `json:"cached_write_tokens,omitempty"`
}

// ExtractUsageFromMeta attempts to extract usage data from the Meta field
// of session updates. Returns the LAST usage found since token counts are
// typically cumulative and reported at the end of a session.
// Returns nil if no usage data is found.
func ExtractUsageFromMeta(updates []acp.SessionUpdate) *Usage {
	var lastUsage *Usage
	for _, update := range updates {
		// Check AgentMessageChunk Meta
		if update.AgentMessageChunk != nil && update.AgentMessageChunk.Meta != nil {
			if usage := parseUsageFromMeta(update.AgentMessageChunk.Meta); usage != nil {
				lastUsage = usage
			}
		}
		// Check AgentThoughtChunk Meta
		if update.AgentThoughtChunk != nil && update.AgentThoughtChunk.Meta != nil {
			if usage := parseUsageFromMeta(update.AgentThoughtChunk.Meta); usage != nil {
				lastUsage = usage
			}
		}
		// Check ToolCall Meta
		if update.ToolCall != nil && update.ToolCall.Meta != nil {
			if usage := parseUsageFromMeta(update.ToolCall.Meta); usage != nil {
				lastUsage = usage
			}
		}
	}
	return lastUsage
}

// parseUsageFromMeta tries to extract usage from a Meta field (any type).
// The Meta field could be a map with usage data embedded.
func parseUsageFromMeta(meta any) *Usage {
	m, ok := meta.(map[string]any)
	if !ok {
		return nil
	}

	// Look for usage field in meta
	usageData, ok := m["usage"]
	if !ok {
		return nil
	}

	usageMap, ok := usageData.(map[string]any)
	if !ok {
		log.Printf("Warning: usage field exists but is not a map: %T", usageData)
		return nil
	}

	usage := &Usage{}

	if v, ok := toInt64(usageMap["input_tokens"]); ok {
		usage.InputTokens = v
	}
	if v, ok := toInt64(usageMap["output_tokens"]); ok {
		usage.OutputTokens = v
	}
	if v, ok := toInt64(usageMap["total_tokens"]); ok {
		usage.TotalTokens = v
	}
	if v, ok := toInt64(usageMap["thought_tokens"]); ok {
		usage.ThoughtTokens = &v
	}
	if v, ok := toInt64(usageMap["cached_read_tokens"]); ok {
		usage.CachedReadTokens = &v
	}
	if v, ok := toInt64(usageMap["cached_write_tokens"]); ok {
		usage.CachedWriteTokens = &v
	}

	// Only return if we found meaningful data
	if usage.TotalTokens > 0 || usage.InputTokens > 0 || usage.OutputTokens > 0 {
		return usage
	}

	log.Printf("Warning: usage map found but contained no valid token counts: %v", usageMap)
	return nil
}

// toInt64 converts various numeric types to int64.
func toInt64(v any) (int64, bool) {
	if v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case string:
		if i, err := strconv.ParseInt(n, 10, 64); err == nil {
			return i, true
		}
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return int64(f), true
		}
		log.Printf("Warning: failed to parse token count from string: %q", n)
		return 0, false
	default:
		log.Printf("Warning: unexpected type for token count: %T (%v)", n, v)
		return 0, false
	}
}
