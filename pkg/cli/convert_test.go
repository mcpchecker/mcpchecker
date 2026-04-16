package cli

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpchecker/mcpchecker/pkg/eval"
)

func TestConvertCommand(t *testing.T) {
	results := sampleResults()
	filePath := createTestResultsFile(t, results)

	cmd := NewConvertCmd()
	cmd.SetArgs([]string{filePath})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("convert command failed: %v", err)
	}
}

func TestConvertCommandWithTaskFilter(t *testing.T) {
	results := sampleResults()
	filePath := createTestResultsFile(t, results)

	cmd := NewConvertCmd()
	cmd.SetArgs([]string{filePath, "--task", "task-1"})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("convert command with --task filter failed: %v", err)
	}
}

func TestConvertCommandNoTaskMatch(t *testing.T) {
	results := sampleResults()
	filePath := createTestResultsFile(t, results)

	cmd := NewConvertCmd()
	cmd.SetArgs([]string{filePath, "--task", "nonexistent-task"})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("convert command should fail when no tasks match filter")
	}
}

func TestConvertCommandFileNotFound(t *testing.T) {
	cmd := NewConvertCmd()
	cmd.SetArgs([]string{"/nonexistent/path/results.json"})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("convert command should fail with nonexistent file")
	}
}

func TestConvertCommandUnsupportedFormat(t *testing.T) {
	results := sampleResults()
	filePath := createTestResultsFile(t, results)

	cmd := NewConvertCmd()
	cmd.SetArgs([]string{filePath, "--output", "yaml"})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("convert command should fail with unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported output format") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConvertCommandOutputFile(t *testing.T) {
	results := sampleResults()
	filePath := createTestResultsFile(t, results)

	outPath := filepath.Join(t.TempDir(), "report.xml")

	cmd := NewConvertCmd()
	cmd.SetArgs([]string{filePath, "--output-file", outPath})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("convert command with --output-file failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "<testsuites>") {
		t.Error("output file should contain JUnit XML")
	}
	if !strings.Contains(content, "task-1") {
		t.Error("output file should contain task names")
	}
}

func TestBuildJUnitSuiteAllPassed(t *testing.T) {
	results := []*eval.EvalResult{
		{
			TaskName:            "passed-task",
			TaskPath:            "/path/to/passed-task.yaml",
			TaskPassed:          true,
			AllAssertionsPassed: true,
			AssertionResults: &eval.CompositeAssertionResult{
				ToolsUsed: &eval.SingleAssertionResult{Passed: true},
			},
		},
	}

	suite := buildJUnitSuite(results, viewOptions{})

	if suite.Name != "mcpchecker" {
		t.Errorf("suite.Name = %q, want %q", suite.Name, "mcpchecker")
	}
	if suite.Tests != 1 {
		t.Errorf("suite.Tests = %d, want 1", suite.Tests)
	}
	if suite.Failures != 0 {
		t.Errorf("suite.Failures = %d, want 0", suite.Failures)
	}
	if suite.Errors != 0 {
		t.Errorf("suite.Errors = %d, want 0", suite.Errors)
	}
	if suite.Cases[0].Failure != nil {
		t.Error("passed test case should not have a Failure element")
	}
	if suite.Cases[0].Error != nil {
		t.Error("passed test case should not have an Error element")
	}
}

func TestBuildJUnitSuiteWithFailure(t *testing.T) {
	results := []*eval.EvalResult{
		{
			TaskName:            "assertion-failed",
			TaskPath:            "/path/to/task.yaml",
			TaskPassed:          true,
			AllAssertionsPassed: false,
			AssertionResults: &eval.CompositeAssertionResult{
				ToolsUsed: &eval.SingleAssertionResult{Passed: false, Reason: "Tool not called"},
			},
		},
	}

	suite := buildJUnitSuite(results, viewOptions{})

	if suite.Failures != 1 {
		t.Errorf("suite.Failures = %d, want 1", suite.Failures)
	}
	if suite.Errors != 0 {
		t.Errorf("suite.Errors = %d, want 0", suite.Errors)
	}
	if suite.Cases[0].Failure == nil {
		t.Fatal("failed assertion test case should have a Failure element")
	}
	if suite.Cases[0].Failure.Type != "AssertionFailure" {
		t.Errorf("Failure.Type = %q, want %q", suite.Cases[0].Failure.Type, "AssertionFailure")
	}
	if !strings.Contains(suite.Cases[0].Failure.Message, "Tool not called") {
		t.Errorf("Failure.Message should contain assertion reason, got %q", suite.Cases[0].Failure.Message)
	}
}

func TestBuildJUnitSuiteWithError(t *testing.T) {
	results := []*eval.EvalResult{
		{
			TaskName:   "exec-error",
			TaskPath:   "/path/to/task.yaml",
			TaskPassed: false,
			TaskError:  "verification failed",
		},
	}

	suite := buildJUnitSuite(results, viewOptions{})

	if suite.Failures != 0 {
		t.Errorf("suite.Failures = %d, want 0", suite.Failures)
	}
	if suite.Errors != 1 {
		t.Errorf("suite.Errors = %d, want 1", suite.Errors)
	}
	if suite.Cases[0].Error == nil {
		t.Fatal("failed test case should have an Error element")
	}
	if suite.Cases[0].Error.Type != "ExecutionError" {
		t.Errorf("Error.Type = %q, want %q", suite.Cases[0].Error.Type, "ExecutionError")
	}
	if suite.Cases[0].Error.Body != "verification failed" {
		t.Errorf("Error.Body = %q, want %q", suite.Cases[0].Error.Body, "verification failed")
	}
}

func TestBuildJUnitSuiteAgentExecutionError(t *testing.T) {
	results := []*eval.EvalResult{
		{
			TaskName:            "agent-crash",
			TaskPath:            "/path/to/task.yaml",
			TaskPassed:          false,
			AgentExecutionError: true,
		},
	}

	suite := buildJUnitSuite(results, viewOptions{})

	if suite.Cases[0].Error == nil {
		t.Fatal("agent error test case should have an Error element")
	}
	if suite.Cases[0].Error.Type != "AgentExecutionError" {
		t.Errorf("Error.Type = %q, want %q", suite.Cases[0].Error.Type, "AgentExecutionError")
	}
}

func TestBuildJUnitSuiteTimeout(t *testing.T) {
	results := []*eval.EvalResult{
		{
			TaskName:   "timed-out",
			TaskPath:   "/path/to/task.yaml",
			TaskPassed: false,
			TimedOut:   true,
			TaskError:  "task exceeded timeout",
		},
	}

	suite := buildJUnitSuite(results, viewOptions{})

	if suite.Cases[0].Error == nil {
		t.Fatal("timed out test case should have an Error element")
	}
	if suite.Cases[0].Error.Type != "Timeout" {
		t.Errorf("Error.Type = %q, want %q", suite.Cases[0].Error.Type, "Timeout")
	}
}

func TestBuildJUnitSuiteNilAssertionResults(t *testing.T) {
	results := []*eval.EvalResult{
		{
			TaskName:            "nil-assertions",
			TaskPath:            "/path/to/task.yaml",
			TaskPassed:          true,
			AllAssertionsPassed: false,
			AssertionResults:    nil,
		},
	}

	suite := buildJUnitSuite(results, viewOptions{})

	if suite.Failures != 1 {
		t.Errorf("suite.Failures = %d, want 1", suite.Failures)
	}
	if suite.Cases[0].Failure == nil {
		t.Fatal("test case with nil assertions but AllAssertionsPassed=false should have Failure")
	}
	if suite.Cases[0].Failure.Message != "Assertions failed" {
		t.Errorf("Failure.Message = %q, want %q", suite.Cases[0].Failure.Message, "Assertions failed")
	}
}

func TestBuildJUnitSuiteMixedResults(t *testing.T) {
	results := sampleResults() // 1 passed, 1 assertion failure, 1 execution error

	suite := buildJUnitSuite(results, viewOptions{})

	if suite.Tests != 3 {
		t.Errorf("suite.Tests = %d, want 3", suite.Tests)
	}
	if suite.Failures != 1 {
		t.Errorf("suite.Failures = %d, want 1", suite.Failures)
	}
	if suite.Errors != 1 {
		t.Errorf("suite.Errors = %d, want 1", suite.Errors)
	}
}

func TestBuildJUnitSuiteSystemOut(t *testing.T) {
	results := []*eval.EvalResult{
		{
			TaskName:            "task-with-output",
			TaskPath:            "/path/to/task.yaml",
			TaskPassed:          true,
			Difficulty:          "easy",
			AllAssertionsPassed: true,
		},
	}

	suite := buildJUnitSuite(results, viewOptions{showTimeline: true})

	if suite.Cases[0].SystemOut == "" {
		t.Error("test case should have system-out content")
	}
	if !strings.Contains(suite.Cases[0].SystemOut, "task-with-output") {
		t.Error("system-out should contain the task name")
	}
	if !strings.Contains(suite.Cases[0].SystemOut, "PASSED") {
		t.Error("system-out should contain the status")
	}
}

func TestOutputJUnitXMLStructure(t *testing.T) {
	results := sampleResults()

	var buf bytes.Buffer
	err := outputJUnit(&buf, results, viewOptions{})
	if err != nil {
		t.Fatalf("outputJUnit failed: %v", err)
	}

	output := buf.String()

	if !strings.HasPrefix(output, xml.Header) {
		t.Error("output should start with XML header")
	}
	if !strings.Contains(output, "<testsuites>") {
		t.Error("output should contain <testsuites>")
	}
	if !strings.Contains(output, `<testsuite name="mcpchecker"`) {
		t.Error("output should contain testsuite with name mcpchecker")
	}
	if !strings.Contains(output, "<testcase") {
		t.Error("output should contain <testcase> elements")
	}

	// Verify it's valid XML by parsing it
	var suites junitTestSuites
	xmlContent := output[len(xml.Header):]
	if err := xml.Unmarshal([]byte(strings.TrimSpace(xmlContent)), &suites); err != nil {
		t.Fatalf("output is not valid XML: %v", err)
	}

	if len(suites.TestSuites) != 1 {
		t.Errorf("expected 1 test suite, got %d", len(suites.TestSuites))
	}
	if suites.TestSuites[0].Tests != 3 {
		t.Errorf("expected 3 tests, got %d", suites.TestSuites[0].Tests)
	}
}

func TestExtractClassname(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/path/to/task.yaml", "task"},
		{"/path/to/my-task.yaml", "my-task"},
		{"/path/to/task.json", "task"},
		{"task.yaml", "task"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractClassname(tt.input)
			if got != tt.want {
				t.Errorf("extractClassname(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
