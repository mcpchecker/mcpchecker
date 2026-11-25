package testcase

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mcpchecker/mcpchecker/functional/servers/mcp"
	"github.com/mcpchecker/mcpchecker/functional/servers/openai"
	"github.com/mcpchecker/mcpchecker/pkg/eval"
)

// RunContext contains runtime data needed for assertions.
// It holds both the parsed eval results and captured mock server data.
type RunContext struct {
	OutputFile string
	// Parsed eval results from the output JSON file
	EvalResults []*eval.EvalResult

	// Command execution results (for lower-level checks)
	CommandOutput string
	ExitCode      int
	CommandError  error

	// Captured data from mock servers (for detailed checks)
	MCPServers  map[string]*mcp.MockMCPServer
	JudgeServer *openai.MockOpenAIServer
}

// FirstResult returns the first eval result, or nil if none
func (ctx *RunContext) FirstResult() *eval.EvalResult {
	if len(ctx.EvalResults) == 0 {
		return nil
	}
	return ctx.EvalResults[0]
}

// ResultForTask returns the eval result for a specific task name
func (ctx *RunContext) ResultForTask(name string) *eval.EvalResult {
	for _, r := range ctx.EvalResults {
		if r.TaskName == name {
			return r
		}
	}
	return nil
}

// Assertion defines an expectation that can be checked after a test runs
type Assertion interface {
	Assert(t *testing.T, ctx *RunContext)
}

// TaskPassedAssertion asserts that the task passed
type TaskPassedAssertion struct {
	TaskName string // Optional: specific task name, empty means first/only task
}

func (a *TaskPassedAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	result := a.getResult(ctx)
	if result == nil {
		t.Errorf("no eval result found")
		return
	}
	if !result.TaskPassed {
		t.Errorf("expected task %q to pass, but it failed", result.TaskName)
		if result.TaskError != "" {
			t.Errorf("task error: %s", result.TaskError)
		}
		if result.TaskJudgeError != "" {
			t.Errorf("judge error: %s", result.TaskJudgeError)
		}
	}
}

func (a *TaskPassedAssertion) getResult(ctx *RunContext) *eval.EvalResult {
	if a.TaskName != "" {
		return ctx.ResultForTask(a.TaskName)
	}
	return ctx.FirstResult()
}

// TaskFailedAssertion asserts that the task failed
type TaskFailedAssertion struct {
	TaskName string // Optional: specific task name
}

func (a *TaskFailedAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	result := a.getResult(ctx)
	if result == nil {
		t.Errorf("no eval result found")
		return
	}
	if result.TaskPassed {
		t.Errorf("expected task %q to fail, but it passed", result.TaskName)
	}
}

func (a *TaskFailedAssertion) getResult(ctx *RunContext) *eval.EvalResult {
	if a.TaskName != "" {
		return ctx.ResultForTask(a.TaskName)
	}
	return ctx.FirstResult()
}

// TaskFailedWithErrorAssertion asserts that the task failed with a specific error message
type TaskFailedWithErrorAssertion struct {
	Contains string
	TaskName string // Optional: specific task name
}

func (a *TaskFailedWithErrorAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	result := a.getResult(ctx)
	if result == nil {
		t.Errorf("no eval result found")
		return
	}
	if result.TaskPassed {
		t.Errorf("expected task %q to fail, but it passed", result.TaskName)
		return
	}
	if !strings.Contains(result.TaskError, a.Contains) {
		t.Errorf("expected task error to contain %q, got: %s", a.Contains, result.TaskError)
	}
}

func (a *TaskFailedWithErrorAssertion) getResult(ctx *RunContext) *eval.EvalResult {
	if a.TaskName != "" {
		return ctx.ResultForTask(a.TaskName)
	}
	return ctx.FirstResult()
}

// AllAssertionsPassedAssertion asserts that all eval assertions passed
type AllAssertionsPassedAssertion struct {
	TaskName string // Optional: specific task name
}

func (a *AllAssertionsPassedAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	result := a.getResult(ctx)
	if result == nil {
		t.Errorf("no eval result found")
		return
	}
	if !result.AllAssertionsPassed {
		t.Errorf("expected all assertions to pass for task %q", result.TaskName)
		if result.AssertionResults != nil {
			if result.AssertionResults.ToolsUsed != nil && !result.AssertionResults.ToolsUsed.Passed {
				t.Errorf("toolsUsed failed: %s", result.AssertionResults.ToolsUsed.Reason)
			}
			if result.AssertionResults.ToolsNotUsed != nil && !result.AssertionResults.ToolsNotUsed.Passed {
				t.Errorf("toolsNotUsed failed: %s", result.AssertionResults.ToolsNotUsed.Reason)
			}
			if result.AssertionResults.MinToolCalls != nil && !result.AssertionResults.MinToolCalls.Passed {
				t.Errorf("minToolCalls failed: %s", result.AssertionResults.MinToolCalls.Reason)
			}
			if result.AssertionResults.MaxToolCalls != nil && !result.AssertionResults.MaxToolCalls.Passed {
				t.Errorf("maxToolCalls failed: %s", result.AssertionResults.MaxToolCalls.Reason)
			}
		}
	}
}

func (a *AllAssertionsPassedAssertion) getResult(ctx *RunContext) *eval.EvalResult {
	if a.TaskName != "" {
		return ctx.ResultForTask(a.TaskName)
	}
	return ctx.FirstResult()
}

// AssertionsFailedAssertion asserts that eval assertions failed
type AssertionsFailedAssertion struct {
	TaskName string // Optional: specific task name
}

func (a *AssertionsFailedAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	result := a.getResult(ctx)
	if result == nil {
		t.Errorf("no eval result found")
		return
	}
	if result.AllAssertionsPassed {
		t.Errorf("expected assertions to fail for task %q, but they passed", result.TaskName)
	}
}

func (a *AssertionsFailedAssertion) getResult(ctx *RunContext) *eval.EvalResult {
	if a.TaskName != "" {
		return ctx.ResultForTask(a.TaskName)
	}
	return ctx.FirstResult()
}

// JudgePassedAssertion asserts that the LLM judge passed
type JudgePassedAssertion struct {
	TaskName string // Optional: specific task name
}

func (a *JudgePassedAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	result := a.getResult(ctx)
	if result == nil {
		t.Errorf("no eval result found")
		return
	}
	if result.TaskJudgeError != "" {
		t.Errorf("judge had an error for task %q: %s", result.TaskName, result.TaskJudgeError)
		return
	}
	// TaskPassed indicates the judge passed (for LLM judge verification)
	if !result.TaskPassed {
		t.Errorf("expected judge to pass for task %q, but it failed", result.TaskName)
		if result.TaskJudgeReason != "" {
			t.Errorf("judge reason: %s", result.TaskJudgeReason)
		}
	}
}

func (a *JudgePassedAssertion) getResult(ctx *RunContext) *eval.EvalResult {
	if a.TaskName != "" {
		return ctx.ResultForTask(a.TaskName)
	}
	return ctx.FirstResult()
}

// JudgeFailedAssertion asserts that the LLM judge failed
type JudgeFailedAssertion struct {
	TaskName string // Optional: specific task name
}

func (a *JudgeFailedAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	result := a.getResult(ctx)
	if result == nil {
		t.Errorf("no eval result found")
		return
	}
	if result.TaskPassed {
		t.Errorf("expected judge to fail for task %q, but it passed", result.TaskName)
	}
}

func (a *JudgeFailedAssertion) getResult(ctx *RunContext) *eval.EvalResult {
	if a.TaskName != "" {
		return ctx.ResultForTask(a.TaskName)
	}
	return ctx.FirstResult()
}

// JudgeReasonContainsAssertion asserts that the judge reason contains a substring
type JudgeReasonContainsAssertion struct {
	Substring string
	TaskName  string // Optional: specific task name
}

func (a *JudgeReasonContainsAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	result := a.getResult(ctx)
	if result == nil {
		t.Errorf("no eval result found")
		return
	}
	if !strings.Contains(result.TaskJudgeReason, a.Substring) {
		t.Errorf("expected judge reason to contain %q, got: %s", a.Substring, result.TaskJudgeReason)
	}
}

func (a *JudgeReasonContainsAssertion) getResult(ctx *RunContext) *eval.EvalResult {
	if a.TaskName != "" {
		return ctx.ResultForTask(a.TaskName)
	}
	return ctx.FirstResult()
}

// AgentExecutionErrorAssertion asserts that the agent had an execution error
type AgentExecutionErrorAssertion struct {
	TaskName string // Optional: specific task name
}

func (a *AgentExecutionErrorAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	result := a.getResult(ctx)
	if result == nil {
		t.Errorf("no eval result found")
		return
	}
	if !result.AgentExecutionError {
		t.Errorf("expected agent execution error for task %q, but there was none", result.TaskName)
	}
}

func (a *AgentExecutionErrorAssertion) getResult(ctx *RunContext) *eval.EvalResult {
	if a.TaskName != "" {
		return ctx.ResultForTask(a.TaskName)
	}
	return ctx.FirstResult()
}

// ExitCodeAssertion asserts a specific command exit code
type ExitCodeAssertion struct {
	Expected int
}

func (a *ExitCodeAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	if ctx.ExitCode != a.Expected {
		t.Errorf("expected exit code %d, got %d", a.Expected, ctx.ExitCode)
	}
}

// ToolCalledAssertion asserts that a specific tool was called (via mock server capture)
type ToolCalledAssertion struct {
	Server string
	Tool   string
}

func (a *ToolCalledAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	server, ok := ctx.MCPServers[a.Server]
	if !ok {
		t.Errorf("MCP server %q not found", a.Server)
		return
	}

	calls := server.CallsForTool(a.Tool)
	if len(calls) == 0 {
		t.Errorf("expected tool %q to be called on server %q, but it was not called", a.Tool, a.Server)
	}
}

// ToolCalledTimesAssertion asserts that a tool was called a specific number of times
type ToolCalledTimesAssertion struct {
	Server string
	Tool   string
	Times  int
}

func (a *ToolCalledTimesAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	server, ok := ctx.MCPServers[a.Server]
	if !ok {
		t.Errorf("MCP server %q not found", a.Server)
		return
	}

	calls := server.CallsForTool(a.Tool)
	if len(calls) != a.Times {
		t.Errorf("expected tool %q to be called %d times on server %q, got %d", a.Tool, a.Times, a.Server, len(calls))
	}
}

// ToolCalledWithArgsAssertion asserts that a tool was called with matching arguments
type ToolCalledWithArgsAssertion struct {
	Server  string
	Tool    string
	Matcher func(map[string]any) bool
}

func (a *ToolCalledWithArgsAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	server, ok := ctx.MCPServers[a.Server]
	if !ok {
		t.Errorf("MCP server %q not found", a.Server)
		return
	}

	calls := server.CallsForTool(a.Tool)
	if len(calls) == 0 {
		t.Errorf("expected tool %q to be called on server %q, but it was not called", a.Tool, a.Server)
		return
	}

	for _, call := range calls {
		if a.Matcher(call.Arguments) {
			return // Found a matching call
		}
	}
	t.Errorf("tool %q was called on server %q but no call matched the expected arguments", a.Tool, a.Server)
}

// ToolNotCalledAssertion asserts that a tool was NOT called
type ToolNotCalledAssertion struct {
	Server string
	Tool   string
}

func (a *ToolNotCalledAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	server, ok := ctx.MCPServers[a.Server]
	if !ok {
		return // Server not found means tool definitely wasn't called
	}

	calls := server.CallsForTool(a.Tool)
	if len(calls) > 0 {
		t.Errorf("expected tool %q to NOT be called on server %q, but it was called %d times", a.Tool, a.Server, len(calls))
	}
}

// JudgeCalledAssertion asserts that the judge was called (via mock server capture)
type JudgeCalledAssertion struct{}

func (a *JudgeCalledAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	if ctx.JudgeServer == nil {
		t.Errorf("judge server not configured")
		return
	}
	if ctx.JudgeServer.RequestCount() == 0 {
		t.Errorf("expected judge to be called, but it was not called")
	}
}

// JudgeNotCalledAssertion asserts that the judge was NOT called
type JudgeNotCalledAssertion struct{}

func (a *JudgeNotCalledAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	if ctx.JudgeServer == nil {
		return // No judge configured, so it definitely wasn't called
	}
	if ctx.JudgeServer.RequestCount() > 0 {
		t.Errorf("expected judge to NOT be called, but it was called %d times", ctx.JudgeServer.RequestCount())
	}
}

// OutputContainsAssertion asserts that the task output contains a substring
type OutputContainsAssertion struct {
	Substring string
	TaskName  string // Optional: specific task name
}

func (a *OutputContainsAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	result := a.getResult(ctx)
	if result == nil {
		t.Errorf("no eval result found")
		return
	}
	if !strings.Contains(result.TaskOutput, a.Substring) {
		t.Errorf("expected task output to contain %q, got: %s", a.Substring, result.TaskOutput)
	}
}

func (a *OutputContainsAssertion) getResult(ctx *RunContext) *eval.EvalResult {
	if a.TaskName != "" {
		return ctx.ResultForTask(a.TaskName)
	}
	return ctx.FirstResult()
}

// OutputMatchesAssertion asserts that the task output matches a regex pattern
type OutputMatchesAssertion struct {
	Pattern  string
	TaskName string // Optional: specific task name
}

func (a *OutputMatchesAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	result := a.getResult(ctx)
	if result == nil {
		t.Errorf("no eval result found")
		return
	}
	re, err := regexp.Compile(a.Pattern)
	if err != nil {
		t.Errorf("invalid regex pattern %q: %v", a.Pattern, err)
		return
	}
	if !re.MatchString(result.TaskOutput) {
		t.Errorf("expected task output to match pattern %q, got: %s", a.Pattern, result.TaskOutput)
	}
}

func (a *OutputMatchesAssertion) getResult(ctx *RunContext) *eval.EvalResult {
	if a.TaskName != "" {
		return ctx.ResultForTask(a.TaskName)
	}
	return ctx.FirstResult()
}

// TaskCountAssertion asserts the number of tasks that were run
type TaskCountAssertion struct {
	Expected int
}

func (a *TaskCountAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	if len(ctx.EvalResults) != a.Expected {
		t.Errorf("expected %d tasks, got %d", a.Expected, len(ctx.EvalResults))
	}
}

// AllTasksPassedAssertion asserts that all tasks passed
type AllTasksPassedAssertion struct{}

func (a *AllTasksPassedAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	for _, result := range ctx.EvalResults {
		if !result.TaskPassed {
			t.Errorf("expected all tasks to pass, but task %q failed", result.TaskName)
			if result.TaskError != "" {
				t.Errorf("task error: %s", result.TaskError)
			}
		}
	}
}

// CustomAssertion allows for custom assertion logic
type CustomAssertion struct {
	Name string
	Fn   func(t *testing.T, ctx *RunContext)
}

func (a *CustomAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	a.Fn(t, ctx)
}

// AssertFunc creates a custom assertion from a function
func AssertFunc(name string, fn func(t *testing.T, ctx *RunContext)) *CustomAssertion {
	return &CustomAssertion{Name: name, Fn: fn}
}

// ResultsInOrderAssertion asserts that results appear in a specific order by task name
type ResultsInOrderAssertion struct {
	TaskNames []string
}

func (a *ResultsInOrderAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	if len(ctx.EvalResults) != len(a.TaskNames) {
		t.Errorf("expected %d results, got %d", len(a.TaskNames), len(ctx.EvalResults))
		return
	}
	for i, expectedName := range a.TaskNames {
		if ctx.EvalResults[i].TaskName != expectedName {
			t.Errorf("result[%d]: expected task name %q, got %q", i, expectedName, ctx.EvalResults[i].TaskName)
		}
	}
}

// TaskPassedByNameAssertion asserts that a specific task passed
type TaskPassedByNameAssertion struct {
	TaskName string
}

func (a *TaskPassedByNameAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	result := ctx.ResultForTask(a.TaskName)
	if result == nil {
		t.Errorf("no result found for task %q", a.TaskName)
		return
	}
	if !result.TaskPassed {
		t.Errorf("expected task %q to pass, but it failed", a.TaskName)
		if result.TaskError != "" {
			t.Errorf("task error: %s", result.TaskError)
		}
	}
}

// TaskFailedByNameAssertion asserts that a specific task failed
type TaskFailedByNameAssertion struct {
	TaskName string
}

func (a *TaskFailedByNameAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	result := ctx.ResultForTask(a.TaskName)
	if result == nil {
		t.Errorf("no result found for task %q", a.TaskName)
		return
	}
	if result.TaskPassed {
		t.Errorf("expected task %q to fail, but it passed", a.TaskName)
	}
}

// PassedCountAssertion asserts the number of tasks that passed
type PassedCountAssertion struct {
	Expected int
}

func (a *PassedCountAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	passed := 0
	for _, r := range ctx.EvalResults {
		if r.TaskPassed {
			passed++
		}
	}
	if passed != a.Expected {
		t.Errorf("expected %d tasks to pass, got %d", a.Expected, passed)
	}
}

// FailedCountAssertion asserts the number of tasks that failed
type FailedCountAssertion struct {
	Expected int
}

func (a *FailedCountAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	failed := 0
	for _, r := range ctx.EvalResults {
		if !r.TaskPassed {
			failed++
		}
	}
	if failed != a.Expected {
		t.Errorf("expected %d tasks to fail, got %d", a.Expected, failed)
	}
}

// DifficultyCountAssertion asserts the number of tasks by difficulty
type DifficultyCountAssertion struct {
	Difficulty string
	Expected   int
}

func (a *DifficultyCountAssertion) Assert(t *testing.T, ctx *RunContext) {
	t.Helper()
	count := 0
	for _, r := range ctx.EvalResults {
		if r.Difficulty == a.Difficulty {
			count++
		}
	}
	if count != a.Expected {
		t.Errorf("expected %d tasks with difficulty %q, got %d", a.Expected, a.Difficulty, count)
	}
}

// Re-export EvalResult for convenience
type EvalResult = eval.EvalResult
