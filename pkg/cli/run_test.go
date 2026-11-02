package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mcpchecker/mcpchecker/pkg/eval"
	"github.com/stretchr/testify/require"
)

func TestPrintRunAssertionSummaryApps(t *testing.T) {
	result := &eval.CompositeAssertionResult{Apps: &eval.SingleAssertionResult{
		Reason: "missing_ui_resource", Details: []string{"fixture/view app-1"},
	}}
	var output bytes.Buffer
	printRunAssertionSummary(&output, result, false)
	require.Contains(t, output.String(), "Assertions: FAILED (0/1)")
	require.Contains(t, output.String(), "- Apps: missing_ui_resource")
	require.Contains(t, output.String(), "fixture/view app-1")
}

func TestPrintRunAssertionSummarySkippedAssertion(t *testing.T) {
	result := &eval.CompositeAssertionResult{
		ToolsUsed:         &eval.SingleAssertionResult{Passed: true},
		SkippedAssertions: []eval.AssertionPresenceResult{{Type: "toolsUsed", Server: "kubernetes", Target: "pods_create", Reason: "tool not present"}},
	}
	var output bytes.Buffer
	printRunAssertionSummary(&output, result, true)
	if !strings.Contains(output.String(), "SKIP toolsUsed kubernetes/pods_create: tool not present") {
		t.Errorf("output should report skipped tool, got:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "Assertions: PASSED (1/1, 1 skipped)") {
		t.Errorf("output should include skipped count, got:\n%s", output.String())
	}
}

func TestPrintRunAssertionSummaryStrictMissingTool(t *testing.T) {
	result := &eval.CompositeAssertionResult{
		PresenceFailures: []eval.AssertionPresenceResult{{Type: "toolsUsed", Server: "kubernetes", Target: "pods_create", Reason: "tool not present"}},
	}
	var output bytes.Buffer
	printRunAssertionSummary(&output, result, false)
	if !strings.Contains(output.String(), "Presence failure: toolsUsed kubernetes/pods_create: tool not present") {
		t.Errorf("output should report strict presence failure, got:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "Assertions: FAILED (0/1)") {
		t.Errorf("presence failure should count as a failed assertion, got:\n%s", output.String())
	}
}

func TestCountAssertionsPassFailSkipSummary(t *testing.T) {
	results := []*eval.EvalResult{
		{
			TaskName:            "pass-with-skips",
			TaskPassed:          true,
			AllAssertionsPassed: true,
			AssertionResults: &eval.CompositeAssertionResult{
				ToolsUsed: &eval.SingleAssertionResult{Passed: true},
				SkippedAssertions: []eval.AssertionPresenceResult{
					{Type: "toolsUsed", Server: "kubernetes", Target: "pods_create", Reason: "tool not present"},
					{Type: "toolsUsed", Server: "kubernetes", Target: "pods_update", Reason: "tool not present"},
				},
			},
		},
		{
			TaskName:            "failed",
			TaskPassed:          true,
			AllAssertionsPassed: false,
			AssertionResults: &eval.CompositeAssertionResult{
				ToolsUsed: &eval.SingleAssertionResult{Passed: false, Reason: "not called"},
			},
		},
		{
			TaskName:            "passed",
			TaskPassed:          true,
			AllAssertionsPassed: true,
			AssertionResults: &eval.CompositeAssertionResult{
				ToolsUsed: &eval.SingleAssertionResult{Passed: true},
			},
		},
	}

	passed, total, skipped := countAssertions(results)
	if passed != 2 || total != 3 || skipped != 2 {
		t.Fatalf("countAssertions() = %d/%d with %d skipped, want 2/3 with 2 skipped", passed, total, skipped)
	}
	var output bytes.Buffer
	output.WriteString(formatAssertionCountSummary("Assertions Passed", passed, total, skipped))
	if !strings.Contains(output.String(), "Assertions Passed: 2/3 (2 skipped)") {
		t.Errorf("summary should count pass, fail, and skip separately, got:\n%s", output.String())
	}
}

func TestFormatPresenceAssertionLocation(t *testing.T) {
	tests := []struct {
		name   string
		result eval.AssertionPresenceResult
		want   string
	}{
		{
			name:   "server and target",
			result: eval.AssertionPresenceResult{Type: "toolsUsed", Server: "kubernetes", Target: "pods_create", Reason: "tool not present"},
			want:   "SKIP toolsUsed kubernetes/pods_create: tool not present",
		},
		{
			name:   "server only",
			result: eval.AssertionPresenceResult{Type: "toolsUsed", Server: "kubernetes", Reason: "server not present"},
			want:   "SKIP toolsUsed kubernetes: server not present",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPresenceAssertion("SKIP", tt.result); got != tt.want {
				t.Errorf("formatPresenceAssertion() = %q, want %q", got, tt.want)
			}
		})
	}
}
