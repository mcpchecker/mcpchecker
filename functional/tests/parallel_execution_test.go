//go:build functional

package tests

import (
	"testing"

	"github.com/mcpchecker/mcpchecker/functional/testcase"
)

// TestParallelTasksExecution verifies that tasks marked as parallel
// run correctly when --parallel flag is set.
func TestParallelTasksExecution(t *testing.T) {
	testcase.New(t, "parallel-tasks").
		WithMCPServer("server1", func(s *testcase.MCPServerBuilder) {
			s.Tool("tool_a", func(tool *testcase.ToolDef) {
				tool.WithDescription("Tool A").
					WithStringParam("input", "Input value", true).
					ReturnsText("Result from tool A")
			})
		}).
		WithAgent(func(a *testcase.AgentBuilder) {
			a.OnPromptContaining("task").
				CallTool("tool_a", map[string]any{"input": "test"}).
				ThenRespond("Completed the task")
		}).
		WithTasks(
			func(task *testcase.TaskConfig) {
				task.Name("parallel-task-1").
					Easy().
					Parallel().
					Prompt("Run parallel task 1").
					VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("parallel-task-2").
					Easy().
					Parallel().
					Prompt("Run parallel task 2").
					VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("parallel-task-3").
					Easy().
					Parallel().
					Prompt("Run parallel task 3").
					VerifyScript("exit 0")
			},
		).
		WithEval(func(eval *testcase.EvalConfig) {
			eval.Name("parallel-eval")
		}).
		WithParallelWorkers(2).
		ExpectResultCount(3).
		ExpectPassedCount(3).
		ExpectTaskPassedByName("parallel-task-1").
		ExpectTaskPassedByName("parallel-task-2").
		ExpectTaskPassedByName("parallel-task-3").
		Run()
}

// TestMixedSequentialAndParallelTasks verifies that sequential tasks
// run first, then parallel tasks run together.
func TestMixedSequentialAndParallelTasks(t *testing.T) {
	testcase.New(t, "mixed-seq-parallel").
		WithMCPServer("server1", func(s *testcase.MCPServerBuilder) {
			s.Tool("tool_a", func(tool *testcase.ToolDef) {
				tool.WithDescription("Tool A").
					WithStringParam("input", "Input value", true).
					ReturnsText("Result from tool A")
			})
		}).
		WithAgent(func(a *testcase.AgentBuilder) {
			a.OnPromptContaining("task").
				CallTool("tool_a", map[string]any{"input": "test"}).
				ThenRespond("Completed the task")
		}).
		WithTasks(
			func(task *testcase.TaskConfig) {
				task.Name("seq-task-1").
					Easy().
					Prompt("Run sequential task 1").
					VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("parallel-task-1").
					Easy().
					Parallel().
					Prompt("Run parallel task 1").
					VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("seq-task-2").
					Medium().
					Prompt("Run sequential task 2").
					VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("parallel-task-2").
					Easy().
					Parallel().
					Prompt("Run parallel task 2").
					VerifyScript("exit 0")
			},
		).
		WithEval(func(eval *testcase.EvalConfig) {
			eval.Name("mixed-eval")
		}).
		WithParallelWorkers(2).
		ExpectResultCount(4).
		ExpectPassedCount(4).
		Run()
}

// TestParallelTasksWithoutFlag verifies that parallel tasks still run
// sequentially when --parallel is not set (default behavior).
func TestParallelTasksWithoutFlag(t *testing.T) {
	testcase.New(t, "parallel-tasks-no-flag").
		WithMCPServer("server1", func(s *testcase.MCPServerBuilder) {
			s.Tool("tool_a", func(tool *testcase.ToolDef) {
				tool.WithDescription("Tool A").
					WithStringParam("input", "Input value", true).
					ReturnsText("Result from tool A")
			})
		}).
		WithAgent(func(a *testcase.AgentBuilder) {
			a.OnPromptContaining("task").
				CallTool("tool_a", map[string]any{"input": "test"}).
				ThenRespond("Completed the task")
		}).
		WithTasks(
			func(task *testcase.TaskConfig) {
				task.Name("parallel-task-1").
					Easy().
					Parallel().
					Prompt("Run parallel task 1").
					VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("parallel-task-2").
					Easy().
					Parallel().
					Prompt("Run parallel task 2").
					VerifyScript("exit 0")
			},
		).
		WithEval(func(eval *testcase.EvalConfig) {
			eval.Name("parallel-no-flag-eval")
		}).
		// No WithParallelWorkers - should run sequentially
		ExpectResultCount(2).
		ExpectPassedCount(2).
		Run()
}
