package eval

import "net/url"

// EvalOutput wraps evaluation results with configuration summary metadata.
// This is the top-level structure written to the JSON output file.
type EvalOutput struct {
	Summary *EvalSummary `json:"summary"`
	Results []*EvalResult `json:"results"`
}

// EvalSummary captures the resolved configuration used for an evaluation run.
type EvalSummary struct {
	Agent           *AgentSummary      `json:"agent"`
	Judge           *JudgeSummary      `json:"judge,omitempty"`
	MCPServers      []MCPServerSummary `json:"mcpServers,omitempty"`
	Evals           *EvalsSummary      `json:"evals"`
	Timeout         *TimeoutSummary    `json:"timeout,omitempty"`
	ParallelWorkers int                `json:"parallelWorkers"`
	Runs            int                `json:"runs"`
}

// AgentSummary describes the agent configuration.
type AgentSummary struct {
	Type    string `json:"type"`
	Name    string `json:"name,omitempty"`
	Model   string `json:"model,omitempty"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
}

// JudgeSummary describes the LLM judge configuration.
type JudgeSummary struct {
	Type    string `json:"type,omitempty"`
	Name    string `json:"name,omitempty"`
	Model   string `json:"model,omitempty"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
}

// MCPServerSummary describes a single MCP server.
type MCPServerSummary struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	URL     string `json:"url,omitempty"`
	Command string `json:"command,omitempty"`
}

// EvalsSummary describes the matched evaluations.
type EvalsSummary struct {
	Names    []string         `json:"names"`
	TaskSets []TaskSetSummary `json:"taskSets,omitempty"`
}

// TaskSetSummary describes a single task set configuration.
type TaskSetSummary struct {
	Glob          string            `json:"glob,omitempty"`
	Path          string            `json:"path,omitempty"`
	LabelSelector map[string]string `json:"labelSelector,omitempty"`
}

// TimeoutSummary describes the timeout configuration.
type TimeoutSummary struct {
	DefaultTask    string `json:"defaultTask,omitempty"`
	Task           string `json:"task,omitempty"`
	DefaultCleanup string `json:"defaultCleanup,omitempty"`
	Cleanup        string `json:"cleanup,omitempty"`
}

// sanitizeURL strips query parameters and userinfo from a URL to avoid
// leaking credentials (tokens, passwords) into output files.
func sanitizeURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.User = nil
	return parsed.String()
}
