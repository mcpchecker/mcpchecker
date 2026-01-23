// Package testcase provides a fluent API for defining functional test cases
// that exercise the mcpchecker binary against mock servers.
package testcase

import (
	"os"
	"testing"
)

// TestCase represents a complete functional test scenario
type TestCase struct {
	t    *testing.T
	name string

	// Mock servers
	mcpServers map[string]*MCPServerBuilder
	judgeMock  *JudgeBuilder
	agentMock  *AgentBuilder

	// Configuration
	tasks []*TaskConfig
	eval  *EvalConfig

	// Assertions to run after the test
	assertions []Assertion

	// Execution mode
	inProcess bool // If true, run mcpchecker in-process instead of subprocess
}

// New creates a new test case with the given name
func New(t *testing.T, name string) *TestCase {
	return &TestCase{
		t:          t,
		name:       name,
		mcpServers: make(map[string]*MCPServerBuilder),
		assertions: make([]Assertion, 0),
	}
}

// WithMCPServer adds a mock MCP server to the test case
func (tc *TestCase) WithMCPServer(name string, configure func(*MCPServerBuilder)) *TestCase {
	builder := NewMCPServerBuilder(name)
	configure(builder)
	tc.mcpServers[name] = builder
	return tc
}

// WithAgent configures the mock agent behavior
func (tc *TestCase) WithAgent(configure func(*AgentBuilder)) *TestCase {
	tc.agentMock = NewAgentBuilder()
	configure(tc.agentMock)
	return tc
}

// WithJudge configures the mock judge behavior.
// The judge is an LLM that evaluates agent output and returns pass/fail decisions.
func (tc *TestCase) WithJudge(configure func(*JudgeBuilder)) *TestCase {
	tc.judgeMock = NewJudgeBuilder()
	configure(tc.judgeMock)
	return tc
}

// WithTasks configures multiple tasks for this test case
func (tc *TestCase) WithTasks(configureFuncs ...func(*TaskConfig)) *TestCase {
	tc.tasks = make([]*TaskConfig, 0, len(configureFuncs))
	for _, configure := range configureFuncs {
		task := NewTaskConfig()
		configure(task)
		tc.tasks = append(tc.tasks, task)
	}
	return tc
}

// AddTask adds a task to the test case (can be called multiple times)
func (tc *TestCase) AddTask(configure func(*TaskConfig)) *TestCase {
	task := NewTaskConfig()
	configure(task)
	tc.tasks = append(tc.tasks, task)
	return tc
}

// WithEval configures the eval settings for this test case
func (tc *TestCase) WithEval(configure func(*EvalConfig)) *TestCase {
	tc.eval = NewEvalConfig()
	configure(tc.eval)
	return tc
}

// Expect adds an assertion to be checked after the test runs
func (tc *TestCase) Expect(a Assertion) *TestCase {
	tc.assertions = append(tc.assertions, a)
	return tc
}

// ExpectTaskPassed asserts that the task passed
func (tc *TestCase) ExpectTaskPassed() *TestCase {
	return tc.Expect(&TaskPassedAssertion{})
}

// ExpectTaskFailed asserts that the task failed
func (tc *TestCase) ExpectTaskFailed() *TestCase {
	return tc.Expect(&TaskFailedAssertion{})
}

// ExpectTaskFailedWithError asserts that the task failed with an error containing the substring
func (tc *TestCase) ExpectTaskFailedWithError(contains string) *TestCase {
	return tc.Expect(&TaskFailedWithErrorAssertion{Contains: contains})
}

// ExpectExitCode asserts the command exit code
func (tc *TestCase) ExpectExitCode(code int) *TestCase {
	return tc.Expect(&ExitCodeAssertion{Expected: code})
}

// ExpectToolCalled asserts that a tool was called on a server
func (tc *TestCase) ExpectToolCalled(server, tool string) *TestCase {
	return tc.Expect(&ToolCalledAssertion{Server: server, Tool: tool})
}

// ExpectToolCalledTimes asserts that a tool was called a specific number of times
func (tc *TestCase) ExpectToolCalledTimes(server, tool string, times int) *TestCase {
	return tc.Expect(&ToolCalledTimesAssertion{Server: server, Tool: tool, Times: times})
}

// ExpectToolCalledWithArgs asserts that a tool was called with specific arguments
func (tc *TestCase) ExpectToolCalledWithArgs(server, tool string, matcher func(map[string]any) bool) *TestCase {
	return tc.Expect(&ToolCalledWithArgsAssertion{Server: server, Tool: tool, Matcher: matcher})
}

// ExpectToolNotCalled asserts that a tool was not called
func (tc *TestCase) ExpectToolNotCalled(server, tool string) *TestCase {
	return tc.Expect(&ToolNotCalledAssertion{Server: server, Tool: tool})
}

// ExpectJudgeCalled asserts that the judge was called
func (tc *TestCase) ExpectJudgeCalled() *TestCase {
	return tc.Expect(&JudgeCalledAssertion{})
}

// ExpectJudgeNotCalled asserts that the judge was not called
func (tc *TestCase) ExpectJudgeNotCalled() *TestCase {
	return tc.Expect(&JudgeNotCalledAssertion{})
}

// ExpectOutputContains asserts that the command output contains a substring
func (tc *TestCase) ExpectOutputContains(substring string) *TestCase {
	return tc.Expect(&OutputContainsAssertion{Substring: substring})
}

// ExpectOutputMatches asserts that the command output matches a regex
func (tc *TestCase) ExpectOutputMatches(pattern string) *TestCase {
	return tc.Expect(&OutputMatchesAssertion{Pattern: pattern})
}

// ExpectResultCount asserts the number of task results
func (tc *TestCase) ExpectResultCount(count int) *TestCase {
	return tc.Expect(&TaskCountAssertion{Expected: count})
}

// ExpectResultsInOrder asserts that results appear in the specified order by task name
func (tc *TestCase) ExpectResultsInOrder(taskNames ...string) *TestCase {
	return tc.Expect(&ResultsInOrderAssertion{TaskNames: taskNames})
}

// ExpectTaskPassedByName asserts that a specific task (by name) passed
func (tc *TestCase) ExpectTaskPassedByName(taskName string) *TestCase {
	return tc.Expect(&TaskPassedByNameAssertion{TaskName: taskName})
}

// ExpectTaskFailedByName asserts that a specific task (by name) failed
func (tc *TestCase) ExpectTaskFailedByName(taskName string) *TestCase {
	return tc.Expect(&TaskFailedByNameAssertion{TaskName: taskName})
}

// ExpectPassedCount asserts the number of tasks that passed
func (tc *TestCase) ExpectPassedCount(count int) *TestCase {
	return tc.Expect(&PassedCountAssertion{Expected: count})
}

// ExpectFailedCount asserts the number of tasks that failed
func (tc *TestCase) ExpectFailedCount(count int) *TestCase {
	return tc.Expect(&FailedCountAssertion{Expected: count})
}

// ExpectDifficultyCount asserts the number of tasks with a specific difficulty
func (tc *TestCase) ExpectDifficultyCount(difficulty string, count int) *TestCase {
	return tc.Expect(&DifficultyCountAssertion{Difficulty: difficulty, Expected: count})
}

// Run executes the test case
func (tc *TestCase) Run() {
	tc.t.Helper()
	tc.t.Run(tc.name, func(t *testing.T) {
		runner := &Runner{tc: tc, t: t}
		runner.Run()
	})
}

// Name returns the test case name
func (tc *TestCase) Name() string {
	return tc.name
}

// WithInProcess enables in-process execution mode for this test case.
// In this mode, the mcpchecker CLI is executed in-process instead of spawning
// a subprocess, enabling code coverage collection and IDE debugging.
func (tc *TestCase) WithInProcess() *TestCase {
	tc.inProcess = true
	return tc
}

// IsInProcess returns true if the test should run in in-process mode.
// This is true if either WithInProcess() was called on the test case,
// or if the MCPCHECKER_TEST_INPROCESS environment variable is set to "true".
func (tc *TestCase) IsInProcess() bool {
	if tc.inProcess {
		return true
	}
	return os.Getenv(EnvInProcessMode) == "true"
}
