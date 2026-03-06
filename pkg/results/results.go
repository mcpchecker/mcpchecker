// Package results provides utilities for loading, filtering, and analyzing evaluation results.
package results

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mcpchecker/mcpchecker/pkg/eval"
)

// Stats holds computed statistics from evaluation results.
type Stats struct {
	ResultsFile       string  `json:"resultsFile"`
	TasksTotal        int     `json:"tasksTotal"`
	TasksPassed       int     `json:"tasksPassed"`
	TaskPassRate      float64 `json:"taskPassRate"`
	AssertionsTotal   int     `json:"assertionsTotal"`
	AssertionsPassed  int     `json:"assertionsPassed"`
	AssertionPassRate float64 `json:"assertionPassRate"`
	TotalTokens       int64   `json:"totalTokens"`
	McpSchemaTokens   int64   `json:"mcpSchemaTokens"`
	TasksWithTokens   int     `json:"tasksWithTokens"` // number of tasks that have token data
}

// Load reads a JSON results file and returns the parsed evaluations.
func Load(path string) ([]*eval.EvalResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read results file: %w", err)
	}

	var results []*eval.EvalResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("failed to parse results JSON: %w", err)
	}

	return results, nil
}

// Filter returns the subset of results whose task names contain the filter substring.
func Filter(results []*eval.EvalResult, filter string) []*eval.EvalResult {
	if filter == "" {
		return results
	}

	filter = strings.ToLower(filter)
	filtered := make([]*eval.EvalResult, 0, len(results))
	for _, r := range results {
		if strings.Contains(strings.ToLower(r.TaskName), filter) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// CalculateStats computes statistics from evaluation results.
func CalculateStats(resultsFile string, results []*eval.EvalResult) Stats {
	stats := Stats{
		ResultsFile: resultsFile,
		TasksTotal:  len(results),
	}

	for _, result := range results {
		if result.TaskPassed {
			stats.TasksPassed++
		}

		if result.AssertionResults != nil {
			stats.AssertionsTotal += result.AssertionResults.TotalAssertions()
			stats.AssertionsPassed += result.AssertionResults.PassedAssertions()
		}

		if result.TokenEstimate != nil {
			stats.TotalTokens += result.TokenEstimate.TotalTokens
			stats.McpSchemaTokens += result.TokenEstimate.McpSchemaTokens
			stats.TasksWithTokens++
		}
	}

	// Calculate pass rates
	if stats.TasksTotal > 0 {
		stats.TaskPassRate = float64(stats.TasksPassed) / float64(stats.TasksTotal)
	}
	if stats.AssertionsTotal > 0 {
		stats.AssertionPassRate = float64(stats.AssertionsPassed) / float64(stats.AssertionsTotal)
	}

	return stats
}

// PassedAssertions returns the number of passed assertions for a result.
func PassedAssertions(r *eval.EvalResult) int {
	if r.AssertionResults == nil {
		return 0
	}
	return r.AssertionResults.PassedAssertions()
}

// TotalAssertions returns the total number of assertions for a result.
func TotalAssertions(r *eval.EvalResult) int {
	if r.AssertionResults == nil {
		return 0
	}
	return r.AssertionResults.TotalAssertions()
}

// FailureReason returns the first failure reason from a result's assertions.
func FailureReason(r *eval.EvalResult) string {
	if r.TaskError != "" {
		return r.TaskError
	}
	if r.AssertionResults == nil {
		return ""
	}
	a := r.AssertionResults
	if a.ToolsUsed != nil && !a.ToolsUsed.Passed {
		return a.ToolsUsed.Reason
	}
	if a.RequireAny != nil && !a.RequireAny.Passed {
		return a.RequireAny.Reason
	}
	if a.ToolsNotUsed != nil && !a.ToolsNotUsed.Passed {
		return a.ToolsNotUsed.Reason
	}
	if a.MinToolCalls != nil && !a.MinToolCalls.Passed {
		return a.MinToolCalls.Reason
	}
	if a.MaxToolCalls != nil && !a.MaxToolCalls.Passed {
		return a.MaxToolCalls.Reason
	}
	if a.ResourcesRead != nil && !a.ResourcesRead.Passed {
		return a.ResourcesRead.Reason
	}
	if a.ResourcesNotRead != nil && !a.ResourcesNotRead.Passed {
		return a.ResourcesNotRead.Reason
	}
	if a.PromptsUsed != nil && !a.PromptsUsed.Passed {
		return a.PromptsUsed.Reason
	}
	if a.PromptsNotUsed != nil && !a.PromptsNotUsed.Passed {
		return a.PromptsNotUsed.Reason
	}
	if a.CallOrder != nil && !a.CallOrder.Passed {
		return a.CallOrder.Reason
	}
	if a.NoDuplicateCalls != nil && !a.NoDuplicateCalls.Passed {
		return a.NoDuplicateCalls.Reason
	}
	return ""
}

// CollectFailedAssertions returns a list of formatted failure messages.
func CollectFailedAssertions(results *eval.CompositeAssertionResult) []string {
	var failures []string

	addFailure := func(name string, result *eval.SingleAssertionResult) {
		if result != nil && !result.Passed {
			failures = append(failures, fmt.Sprintf("%s: %s", name, result.Reason))
		}
	}

	addFailure("ToolsUsed", results.ToolsUsed)
	addFailure("RequireAny", results.RequireAny)
	addFailure("ToolsNotUsed", results.ToolsNotUsed)
	addFailure("MinToolCalls", results.MinToolCalls)
	addFailure("MaxToolCalls", results.MaxToolCalls)
	addFailure("ResourcesRead", results.ResourcesRead)
	addFailure("ResourcesNotRead", results.ResourcesNotRead)
	addFailure("PromptsUsed", results.PromptsUsed)
	addFailure("PromptsNotUsed", results.PromptsNotUsed)
	addFailure("CallOrder", results.CallOrder)
	addFailure("NoDuplicateCalls", results.NoDuplicateCalls)

	return failures
}
