package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mcpchecker/mcpchecker/pkg/agent"
	"github.com/mcpchecker/mcpchecker/pkg/eval"
)

func TestDiffCommand(t *testing.T) {
	baseResults := sampleResults()
	currentResults := sampleResultsImproved()

	baseFile := createTestResultsFile(t, baseResults)
	currentFile := createTestResultsFile(t, currentResults)

	cmd := NewDiffCmd()
	cmd.SetArgs([]string{"--base", baseFile, "--current", currentFile})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("diff command failed: %v", err)
	}
}

func TestDiffCommandMarkdown(t *testing.T) {
	baseResults := sampleResults()
	currentResults := sampleResultsImproved()

	baseFile := createTestResultsFile(t, baseResults)
	currentFile := createTestResultsFile(t, currentResults)

	cmd := NewDiffCmd()
	cmd.SetArgs([]string{"--base", baseFile, "--current", currentFile, "--output", "markdown"})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("diff command with --output markdown failed: %v", err)
	}
}

func TestDiffCommandBaseNotFound(t *testing.T) {
	currentResults := sampleResults()
	currentFile := createTestResultsFile(t, currentResults)

	cmd := NewDiffCmd()
	cmd.SetArgs([]string{"--base", "/nonexistent/path/base.json", "--current", currentFile})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("diff command should fail with nonexistent base file")
	}
}

func TestDiffCommandCurrentNotFound(t *testing.T) {
	baseResults := sampleResults()
	baseFile := createTestResultsFile(t, baseResults)

	cmd := NewDiffCmd()
	cmd.SetArgs([]string{"--base", baseFile, "--current", "/nonexistent/path/current.json"})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("diff command should fail with nonexistent current file")
	}
}

func TestCalculateDiff(t *testing.T) {
	baseResults := sampleResults()
	headResults := sampleResultsImproved()

	diff := calculateDiff("base.json", "head.json", baseResults, headResults)

	// Check base stats
	if diff.BaseStats.TasksTotal != 3 {
		t.Errorf("BaseStats.TasksTotal = %d, want 3", diff.BaseStats.TasksTotal)
	}

	// Check head stats (improved results have 4 tasks)
	if diff.HeadStats.TasksTotal != 4 {
		t.Errorf("HeadStats.TasksTotal = %d, want 4", diff.HeadStats.TasksTotal)
	}

	// Should have 1 improvement (task-2 passes in head)
	if len(diff.Improvements) != 1 {
		t.Errorf("len(Improvements) = %d, want 1", len(diff.Improvements))
	}

	// Should have 1 new task
	if len(diff.New) != 1 {
		t.Errorf("len(New) = %d, want 1", len(diff.New))
	}
}

func TestCalculateDiffRegressions(t *testing.T) {
	// Swap base and head to test regressions
	baseResults := sampleResultsImproved()
	headResults := sampleResults()

	diff := calculateDiff("base.json", "head.json", baseResults, headResults)

	// Should have 1 regression (task-2 fails in head)
	if len(diff.Regressions) != 1 {
		t.Errorf("len(Regressions) = %d, want 1", len(diff.Regressions))
	}

	// Should have 1 removed task
	if len(diff.Removed) != 1 {
		t.Errorf("len(Removed) = %d, want 1", len(diff.Removed))
	}
}

func TestCalculateDiffNoChanges(t *testing.T) {
	results := sampleResults()

	diff := calculateDiff("base.json", "head.json", results, results)

	if len(diff.Regressions) != 0 {
		t.Errorf("len(Regressions) = %d, want 0", len(diff.Regressions))
	}

	if len(diff.Improvements) != 0 {
		t.Errorf("len(Improvements) = %d, want 0", len(diff.Improvements))
	}

	if len(diff.New) != 0 {
		t.Errorf("len(New) = %d, want 0", len(diff.New))
	}

	if len(diff.Removed) != 0 {
		t.Errorf("len(Removed) = %d, want 0", len(diff.Removed))
	}
}

func TestCalculateDiffEmptyBase(t *testing.T) {
	headResults := sampleResults()

	diff := calculateDiff("base.json", "head.json", []*eval.EvalResult{}, headResults)

	// All tasks in head should be "new"
	if len(diff.New) != 3 {
		t.Errorf("len(New) = %d, want 3", len(diff.New))
	}
}

func TestCalculateDiffEmptyHead(t *testing.T) {
	baseResults := sampleResults()

	diff := calculateDiff("base.json", "head.json", baseResults, []*eval.EvalResult{})

	// All tasks in base should be "removed"
	if len(diff.Removed) != 3 {
		t.Errorf("len(Removed) = %d, want 3", len(diff.Removed))
	}
}

func TestFormatChangeMarkdown(t *testing.T) {
	tests := []struct {
		change   float64
		contains string
	}{
		{0.1, "🟢"},
		{-0.1, "🔴"},
		{0.0, "➖"},
	}

	for _, tt := range tests {
		result := formatChangeMarkdown(tt.change)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("formatChangeMarkdown(%f) = %q, want to contain %q", tt.change, result, tt.contains)
		}
	}
}

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		tokens int64
		want   string
	}{
		{0, "0"},
		{500, "500"},
		{999, "999"},
		{1000, "1.0K"},
		{1500, "1.5K"},
		{10000, "10.0K"},
		{999999, "1000.0K"},
		{1000000, "1.0M"},
		{1500000, "1.5M"},
		{10000000, "10.0M"},
		// Negative values
		{-500, "-500"},
		{-1500, "-1.5K"},
		{-1500000, "-1.5M"},
	}

	for _, tt := range tests {
		got := formatTokenCount(tt.tokens)
		if got != tt.want {
			t.Errorf("formatTokenCount(%d) = %q, want %q", tt.tokens, got, tt.want)
		}
	}
}

func TestFormatTokenChangeMarkdown(t *testing.T) {
	tests := []struct {
		base     int64
		head     int64
		contains string
	}{
		{1000, 1500, "🔴"},  // increase is bad
		{1500, 1000, "🟢"},  // decrease is good
		{1000, 1000, "➖"},  // no change
		{0, 1000, "🔴"},     // base is 0, increase still bad
		{0, 0, "➖"},        // both 0
	}

	for _, tt := range tests {
		result := formatTokenChangeMarkdown(tt.base, tt.head)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("formatTokenChangeMarkdown(%d, %d) = %q, want to contain %q", tt.base, tt.head, result, tt.contains)
		}
	}
}

func TestFormatTokenChangeMarkdownWithCoverage(t *testing.T) {
	tests := []struct {
		name                string
		baseTokens          int64
		baseTasksWithTokens int
		headTokens          int64
		headTasksWithTokens int
		want                string
	}{
		{"neither has data", 0, 0, 0, 0, "➖"},
		{"only head has data", 0, 0, 1000, 2, "(no base data)"},
		{"only base has data", 1000, 2, 0, 0, "(no head data)"},
		{"both have data - increase", 1000, 2, 1500, 2, "🔴"},
		{"both have data - decrease", 1500, 2, 1000, 2, "🟢"},
		{"both have data - no change", 1000, 2, 1000, 2, "➖"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTokenChangeMarkdownWithCoverage(tt.baseTokens, tt.baseTasksWithTokens, tt.headTokens, tt.headTasksWithTokens)
			if !strings.Contains(result, tt.want) {
				t.Errorf("formatTokenChangeMarkdownWithCoverage(%d, %d, %d, %d) = %q, want to contain %q",
					tt.baseTokens, tt.baseTasksWithTokens, tt.headTokens, tt.headTasksWithTokens, result, tt.want)
			}
		})
	}
}

func TestCalculateDiffWithTokens(t *testing.T) {
	baseResults := []*eval.EvalResult{
		{
			TaskName:   "task-1",
			TaskPassed: true,
			TokenEstimate: &agent.TokenEstimate{
				TotalTokens:     10000,
				McpSchemaTokens: 2000,
			},
		},
		{
			TaskName:   "task-2",
			TaskPassed: true,
			TokenEstimate: &agent.TokenEstimate{
				TotalTokens:     15000,
				McpSchemaTokens: 3000,
			},
		},
	}

	headResults := []*eval.EvalResult{
		{
			TaskName:   "task-1",
			TaskPassed: true,
			TokenEstimate: &agent.TokenEstimate{
				TotalTokens:     8000,
				McpSchemaTokens: 1500,
			},
		},
		{
			TaskName:   "task-2",
			TaskPassed: true,
			TokenEstimate: &agent.TokenEstimate{
				TotalTokens:     12000,
				McpSchemaTokens: 2500,
			},
		},
	}

	diff := calculateDiff("base.json", "head.json", baseResults, headResults)

	// Base: 10000 + 15000 = 25000
	if diff.BaseStats.TotalTokens != 25000 {
		t.Errorf("BaseStats.TotalTokens = %d, want 25000", diff.BaseStats.TotalTokens)
	}
	if diff.BaseStats.McpSchemaTokens != 5000 {
		t.Errorf("BaseStats.McpSchemaTokens = %d, want 5000", diff.BaseStats.McpSchemaTokens)
	}

	// Head: 8000 + 12000 = 20000
	if diff.HeadStats.TotalTokens != 20000 {
		t.Errorf("HeadStats.TotalTokens = %d, want 20000", diff.HeadStats.TotalTokens)
	}
	if diff.HeadStats.McpSchemaTokens != 4000 {
		t.Errorf("HeadStats.McpSchemaTokens = %d, want 4000", diff.HeadStats.McpSchemaTokens)
	}
}

// sampleResultsImproved returns improved results for diff testing
func sampleResultsImproved() []*eval.EvalResult {
	return []*eval.EvalResult{
		{
			TaskName:   "task-1",
			TaskPath:   "/path/to/task-1",
			TaskPassed: true,
			Difficulty: "easy",
			AssertionResults: &eval.CompositeAssertionResult{
				ToolsUsed:    &eval.SingleAssertionResult{Passed: true},
				MinToolCalls: &eval.SingleAssertionResult{Passed: true},
			},
			AllAssertionsPassed: true,
		},
		{
			TaskName:   "task-2",
			TaskPath:   "/path/to/task-2",
			TaskPassed: true,
			Difficulty: "medium",
			AssertionResults: &eval.CompositeAssertionResult{
				ToolsUsed:     &eval.SingleAssertionResult{Passed: true},
				ResourcesRead: &eval.SingleAssertionResult{Passed: true}, // Now passes
			},
			AllAssertionsPassed: true, // Now passes
		},
		{
			TaskName:   "task-3",
			TaskPath:   "/path/to/task-3",
			TaskPassed: false,
			TaskError:  "verification failed",
			Difficulty: "hard",
			AssertionResults: &eval.CompositeAssertionResult{
				ToolsUsed: &eval.SingleAssertionResult{Passed: false, Reason: "Tool not called"},
			},
			AllAssertionsPassed: false,
		},
		{
			TaskName:   "task-4",
			TaskPath:   "/path/to/task-4",
			TaskPassed: true,
			Difficulty: "easy",
			AssertionResults: &eval.CompositeAssertionResult{
				ToolsUsed: &eval.SingleAssertionResult{Passed: true},
			},
			AllAssertionsPassed: true,
		},
	}
}
