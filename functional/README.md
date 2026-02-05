# Functional Tests

This directory contains functional tests that exercise the `mcpchecker` binary against mock MCP servers, agents, and LLM judges.

## Running Tests

### Subprocess Mode (Default)

Run functional tests using the compiled `mcpchecker` binary as a subprocess:

```bash
make functional
```

This builds the `mcpchecker` binary and runs all functional tests against it.

### In-Process Mode (Coverage & Debugging)

Run functional tests in-process to enable code coverage collection and IDE debugging:

```bash
make functional-coverage
```

This produces a coverage report at `coverage.html` showing which production code lines are covered by functional tests.

#### IDE Debugging

To debug a specific test with breakpoints in production code:

```bash
MCPCHECKER_TEST_INPROCESS=true go test -v -tags functional ./functional/tests -run TestTaskPassesWithToolCallAndJudge
```

Set breakpoints in `pkg/cli/`, `pkg/eval/`, etc. and they will be hit during test execution.

## Writing Functional Tests

Functional tests use a fluent API defined in `functional/testcase/`. Here's a basic example:

```go
//go:build functional

package tests

import (
    "testing"
    "github.com/mcpchecker/mcpchecker/functional/testcase"
)

func TestMyFeature(t *testing.T) {
    testcase.New(t, "my-test-case").
        // Configure mock MCP server with tools
        WithMCPServer("my-server", func(s *testcase.MCPServerBuilder) {
            s.Tool("my_tool", func(tool *testcase.ToolDef) {
                tool.WithDescription("Does something useful").
                    WithStringParam("param", "A parameter", true).
                    ReturnsText("success")
            })
        }).
        // Configure mock agent behavior
        WithAgent(func(a *testcase.AgentBuilder) {
            a.OnPromptContaining("do something").
                CallTool("my_tool", map[string]any{"param": "value"}).
                ThenRespond("I did the thing successfully.")
        }).
        // Configure mock LLM judge (optional)
        WithJudge(func(j *testcase.JudgeBuilder) {
            j.Always().Pass("Task completed successfully")
        }).
        // Configure task
        AddTask(func(task *testcase.TaskConfig) {
            task.Name("my-task").
                Easy().
                Prompt("Do something with my_tool").
                VerifyContains("success")
        }).
        // Configure eval
        WithEval(func(eval *testcase.EvalConfig) {
            eval.Name("my-eval")
        }).
        // Add assertions
        ExpectTaskPassed().
        ExpectToolCalled("my-server", "my_tool").
        ExpectJudgeCalled().
        // Run the test
        Run()
}
```

### Per-Test In-Process Mode

Force a specific test to run in-process (useful for debugging):

```go
testcase.New(t, "debug-this-test").
    WithInProcess().  // Force in-process execution
    WithMCPServer(...).
    Run()
```

## Directory Structure

```
functional/
├── README.md              # This file
├── servers/
│   ├── agent/             # Mock agent implementation
│   │   └── cmd/           # Mock agent binary
│   ├── mcp/               # Mock MCP server
│   └── openai/            # Mock OpenAI API (for LLM judge)
├── testcase/
│   ├── case.go            # TestCase fluent API
│   ├── run.go             # Test runner (subprocess & in-process)
│   ├── inprocess.go       # In-process executor
│   ├── assertions.go      # Test assertions
│   ├── builders.go        # Mock server builders
│   └── generator.go       # Config file generation
└── tests/
    └── happy_path_test.go # Functional test cases
```

## Available Assertions

| Method                                            | Description                           |
|---------------------------------------------------|---------------------------------------|
| `ExpectTaskPassed()`                              | Assert the task passed                |
| `ExpectTaskFailed()`                              | Assert the task failed                |
| `ExpectTaskFailedWithError(contains)`             | Assert failure with specific error    |
| `ExpectExitCode(code)`                            | Assert command exit code              |
| `ExpectToolCalled(server, tool)`                  | Assert a tool was called              |
| `ExpectToolCalledTimes(server, tool, n)`          | Assert tool called exactly n times    |
| `ExpectToolCalledWithArgs(server, tool, matcher)` | Assert tool called with specific args |
| `ExpectToolNotCalled(server, tool)`               | Assert a tool was not called          |
| `ExpectJudgeCalled()`                             | Assert the LLM judge was invoked      |
| `ExpectJudgeNotCalled()`                          | Assert the LLM judge was not invoked  |
| `ExpectOutputContains(substring)`                 | Assert command output contains text   |
| `ExpectOutputMatches(pattern)`                    | Assert command output matches regex   |

## Environment Variables

| Variable                | Description                             |
|-------------------------|-----------------------------------------|
| `MCPCHECKER_BINARY`         | Path to mcpchecker binary (subprocess mode) |
| `MOCK_AGENT_BINARY`         | Path to mock agent binary                   |
| `MCPCHECKER_TEST_INPROCESS` | Set to `true` to enable in-process mode     |

## Thread Safety Note

In-process mode modifies global state (`os.Args`, `os.Stdout`, working directory). A mutex ensures only one in-process test runs at a time. The `-p=1` flag in `make functional-coverage` enforces sequential test execution for safety.
