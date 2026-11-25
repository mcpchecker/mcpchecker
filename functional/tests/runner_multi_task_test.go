//go:build functional

package tests

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/mcpchecker/mcpchecker/functional/testcase"
	"github.com/mcpchecker/mcpchecker/pkg/eval"
	"github.com/stretchr/testify/require"
)

type toolCoverageAssertion struct{}

func (toolCoverageAssertion) Assert(t *testing.T, ctx *testcase.RunContext) {
	t.Helper()
	data, err := os.ReadFile(ctx.OutputFile)
	require.NoError(t, err)
	var output eval.EvalOutput
	require.NoError(t, json.Unmarshal(data, &output))
	require.Len(t, output.Results, 3)
	require.Equal(t, []eval.ServerToolCoverage{{ServerName: "kubernetes", DiscoveryOK: true, Tools: []eval.ToolCoverage{
		{Name: "kubectl_apply", Called: true, SuccessCount: 2},
		{Name: "kubectl_get", Called: true, SuccessCount: 2},
		{Name: "unused"},
	}}}, output.ToolCoverage)
}

// TestMultipleTasksAllPass verifies that multiple tasks can be run
// and all pass correctly. This is the baseline test for multi-task execution.
func TestMultipleTasksAllPass(t *testing.T) {
	testcase.New(t, "multi-task-all-pass").
		WithMCPServer("server1", func(s *testcase.MCPServerBuilder) {
			s.Tool("tool_a", func(tool *testcase.ToolDef) {
				tool.WithDescription("Tool A").
					WithStringParam("input", "Input value", true).
					ReturnsText("Result from tool A")
			})
		}).
		WithAgent(func(a *testcase.AgentBuilder) {
			// Agent responds to any prompt by calling tool_a
			a.OnPromptContaining("task").
				CallTool("tool_a", map[string]any{"input": "test"}).
				ThenRespond("Completed the task using tool A")
		}).
		WithTasks(
			func(task *testcase.TaskConfig) {
				task.Name("task-1").
					Easy().
					Prompt("Run task 1").
					VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("task-2").
					Medium().
					Prompt("Run task 2").
					VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("task-3").
					Hard().
					Prompt("Run task 3").
					VerifyScript("exit 0")
			},
		).
		WithEval(func(eval *testcase.EvalConfig) {
			eval.Name("multi-task-eval")
		}).
		ExpectResultCount(3).
		ExpectResultsInOrder("task-1", "task-2", "task-3").
		ExpectPassedCount(3).
		ExpectFailedCount(0).
		ExpectTaskPassedByName("task-1").
		ExpectTaskPassedByName("task-2").
		ExpectTaskPassedByName("task-3").
		Run()
}

// TestMultipleTasksMixedResults verifies that when some tasks pass and some fail,
// all results are correctly captured and the eval continues after failures.
func TestMultipleTasksMixedResults(t *testing.T) {
	testcase.New(t, "multi-task-mixed-results").
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
				task.Name("passing-task").
					Easy().
					Prompt("Run passing task").
					VerifyScript("exit 0") // Pass
			},
			func(task *testcase.TaskConfig) {
				task.Name("failing-task").
					Medium().
					Prompt("Run failing task").
					VerifyScript("exit 1") // Fail
			},
			func(task *testcase.TaskConfig) {
				task.Name("another-passing-task").
					Hard().
					Prompt("Run another passing task").
					VerifyScript("exit 0") // Pass
			},
		).
		WithEval(func(eval *testcase.EvalConfig) {
			eval.Name("mixed-results-eval")
		}).
		ExpectResultCount(3).
		ExpectResultsInOrder("passing-task", "failing-task", "another-passing-task").
		ExpectPassedCount(2).
		ExpectFailedCount(1).
		ExpectTaskPassedByName("passing-task").
		ExpectTaskFailedByName("failing-task").
		ExpectTaskPassedByName("another-passing-task").
		Run()
}

// TestResultOrderPreserved verifies that results are returned in the same order
// as tasks are defined, regardless of execution order (important for parallel).
func TestResultOrderPreserved(t *testing.T) {
	testcase.New(t, "result-order-preserved").
		WithMCPServer("server1", func(s *testcase.MCPServerBuilder) {
			s.Tool("tool_a", func(tool *testcase.ToolDef) {
				tool.WithDescription("Tool A").
					WithStringParam("input", "Input value", true).
					ReturnsText("Done")
			})
		}).
		WithAgent(func(a *testcase.AgentBuilder) {
			a.OnPromptContaining("task").
				CallTool("tool_a", map[string]any{"input": "test"}).
				ThenRespond("Done")
		}).
		WithTasks(
			func(task *testcase.TaskConfig) {
				task.Name("alpha").Easy().Prompt("Run alpha task").VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("beta").Easy().Prompt("Run beta task").VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("gamma").Easy().Prompt("Run gamma task").VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("delta").Easy().Prompt("Run delta task").VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("epsilon").Easy().Prompt("Run epsilon task").VerifyScript("exit 0")
			},
		).
		WithEval(func(eval *testcase.EvalConfig) {
			eval.Name("order-test-eval")
		}).
		ExpectResultCount(5).
		ExpectResultsInOrder("alpha", "beta", "gamma", "delta", "epsilon").
		Run()
}

// TestDifficultyCategories verifies that difficulty levels are correctly
// preserved across multiple tasks.
func TestDifficultyCategories(t *testing.T) {
	testcase.New(t, "difficulty-categories").
		WithMCPServer("server1", func(s *testcase.MCPServerBuilder) {
			s.Tool("tool_a", func(tool *testcase.ToolDef) {
				tool.WithDescription("Tool A").
					WithStringParam("input", "Input value", true).
					ReturnsText("Done")
			})
		}).
		WithAgent(func(a *testcase.AgentBuilder) {
			a.OnPromptContaining("task").
				CallTool("tool_a", map[string]any{"input": "test"}).
				ThenRespond("Done")
		}).
		WithTasks(
			func(task *testcase.TaskConfig) {
				task.Name("easy-1").Easy().Prompt("Easy task 1").VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("easy-2").Easy().Prompt("Easy task 2").VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("medium-1").Medium().Prompt("Medium task 1").VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("hard-1").Hard().Prompt("Hard task 1").VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("hard-2").Hard().Prompt("Hard task 2").VerifyScript("exit 0")
			},
		).
		WithEval(func(eval *testcase.EvalConfig) {
			eval.Name("difficulty-eval")
		}).
		ExpectResultCount(5).
		ExpectDifficultyCount("easy", 2).
		ExpectDifficultyCount("medium", 1).
		ExpectDifficultyCount("hard", 2).
		Run()
}

// TestToolCallsAcrossTasks verifies that tool calls are correctly tracked
// when multiple tasks call the same or different tools.
func TestToolCallsAcrossTasks(t *testing.T) {
	testcase.New(t, "tool-calls-across-tasks").
		WithMCPServer("kubernetes", func(s *testcase.MCPServerBuilder) {
			s.Tool("kubectl_get", func(tool *testcase.ToolDef) {
				tool.WithDescription("Get resources").
					WithStringParam("resource", "Resource type", true).
					ReturnsText("NAME    READY   STATUS")
			}).
				Tool("kubectl_apply", func(tool *testcase.ToolDef) {
					tool.WithDescription("Apply manifest").
						WithStringParam("manifest", "YAML manifest", true).
						ReturnsText("resource created")
				}).
				Tool("unused", func(tool *testcase.ToolDef) {
					tool.ReturnsText("unused")
				})
		}).
		WithAgent(func(a *testcase.AgentBuilder) {
			// Task 1 uses kubectl_get
			a.OnPromptContaining("list pods").
				CallTool("kubectl_get", map[string]any{"resource": "pods"}).
				ThenRespond("Listed pods")
			// Task 2 uses kubectl_apply
			a.OnPromptContaining("create deployment").
				CallTool("kubectl_apply", map[string]any{"manifest": "kind: Deployment"}).
				ThenRespond("Created deployment")
			// Task 3 uses both
			a.OnPromptContaining("check and deploy").
				CallTool("kubectl_get", map[string]any{"resource": "pods"}).
				CallTool("kubectl_apply", map[string]any{"manifest": "kind: Pod"}).
				ThenRespond("Checked and deployed")
		}).
		WithTasks(
			func(task *testcase.TaskConfig) {
				task.Name("list-pods-task").
					Easy().
					Prompt("list pods in the cluster").
					VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("create-deployment-task").
					Medium().
					Prompt("create deployment in the cluster").
					VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("check-and-deploy-task").
					Hard().
					Prompt("check and deploy to the cluster").
					VerifyScript("exit 0")
			},
		).
		WithEval(func(eval *testcase.EvalConfig) {
			eval.Name("tool-calls-eval")
		}).
		ExpectResultCount(3).
		ExpectPassedCount(3).
		// kubectl_get should be called twice (task 1 and task 3)
		ExpectToolCalledTimes("kubernetes", "kubectl_get", 2).
		// kubectl_apply should be called twice (task 2 and task 3)
		ExpectToolCalledTimes("kubernetes", "kubectl_apply", 2).
		Expect(toolCoverageAssertion{}).
		Run()
}

// TestAllTasksFailVerification verifies behavior when all tasks fail verification
func TestAllTasksFailVerification(t *testing.T) {
	testcase.New(t, "all-tasks-fail-verification").
		WithMCPServer("server1", func(s *testcase.MCPServerBuilder) {
			s.Tool("tool_a", func(tool *testcase.ToolDef) {
				tool.WithDescription("Tool A").
					WithStringParam("input", "Input value", true).
					ReturnsText("Done")
			})
		}).
		WithAgent(func(a *testcase.AgentBuilder) {
			a.OnPromptContaining("task").
				CallTool("tool_a", map[string]any{"input": "test"}).
				ThenRespond("Done")
		}).
		WithTasks(
			func(task *testcase.TaskConfig) {
				task.Name("fail-1").Easy().Prompt("Run task 1").VerifyScript("exit 1")
			},
			func(task *testcase.TaskConfig) {
				task.Name("fail-2").Medium().Prompt("Run task 2").VerifyScript("exit 1")
			},
			func(task *testcase.TaskConfig) {
				task.Name("fail-3").Hard().Prompt("Run task 3").VerifyScript("exit 1")
			},
		).
		WithEval(func(eval *testcase.EvalConfig) {
			eval.Name("all-fail-eval")
		}).
		ExpectResultCount(3).
		ExpectPassedCount(0).
		ExpectFailedCount(3).
		ExpectTaskFailedByName("fail-1").
		ExpectTaskFailedByName("fail-2").
		ExpectTaskFailedByName("fail-3").
		Run()
}

// TestSingleTaskStillWorks verifies that the framework still works correctly
// with a single task (backwards compatibility).
func TestSingleTaskStillWorks(t *testing.T) {
	testcase.New(t, "single-task-compat").
		WithMCPServer("server1", func(s *testcase.MCPServerBuilder) {
			s.Tool("tool_a", func(tool *testcase.ToolDef) {
				tool.WithDescription("Tool A").
					WithStringParam("input", "Input value", true).
					ReturnsText("Done")
			})
		}).
		WithAgent(func(a *testcase.AgentBuilder) {
			a.OnPromptContaining("single").
				CallTool("tool_a", map[string]any{"input": "test"}).
				ThenRespond("Single task completed")
		}).
		AddTask(func(task *testcase.TaskConfig) {
			task.Name("single-task").
				Easy().
				Prompt("Run single task").
				VerifyScript("exit 0")
		}).
		WithEval(func(eval *testcase.EvalConfig) {
			eval.Name("single-task-eval")
		}).
		ExpectResultCount(1).
		ExpectTaskPassed().
		ExpectToolCalled("server1", "tool_a").
		Run()
}

// TestLargeNumberOfTasks verifies the runner handles many tasks correctly.
// This is important for stress-testing the parallel implementation later.
func TestLargeNumberOfTasks(t *testing.T) {
	tc := testcase.New(t, "many-tasks").
		WithMCPServer("server1", func(s *testcase.MCPServerBuilder) {
			s.Tool("tool_a", func(tool *testcase.ToolDef) {
				tool.WithDescription("Tool A").
					WithStringParam("input", "Input value", true).
					ReturnsText("Done")
			})
		}).
		WithAgent(func(a *testcase.AgentBuilder) {
			a.OnPromptContaining("task").
				CallTool("tool_a", map[string]any{"input": "test"}).
				ThenRespond("Task completed")
		}).
		WithEval(func(eval *testcase.EvalConfig) {
			eval.Name("many-tasks-eval")
		})

	// Add 10 tasks using closure capture pattern
	expectedNames := []string{
		"task-00", "task-01", "task-02", "task-03", "task-04",
		"task-05", "task-06", "task-07", "task-08", "task-09",
	}

	for _, name := range expectedNames {
		taskName := name // capture for closure
		tc.AddTask(func(task *testcase.TaskConfig) {
			task.Name(taskName).
				Easy().
				Prompt("Run " + taskName).
				VerifyScript("exit 0")
		})
	}

	tc.ExpectResultCount(10).
		ExpectPassedCount(10).
		ExpectResultsInOrder(expectedNames...).
		Run()
}

// TestMixedDifficultyAndOutcomes verifies correct handling of various combinations
// of difficulty levels and pass/fail outcomes.
func TestMixedDifficultyAndOutcomes(t *testing.T) {
	testcase.New(t, "mixed-difficulty-outcomes").
		WithMCPServer("server1", func(s *testcase.MCPServerBuilder) {
			s.Tool("tool_a", func(tool *testcase.ToolDef) {
				tool.WithDescription("Tool A").
					WithStringParam("input", "Input value", true).
					ReturnsText("Done")
			})
		}).
		WithAgent(func(a *testcase.AgentBuilder) {
			a.OnPromptContaining("task").
				CallTool("tool_a", map[string]any{"input": "test"}).
				ThenRespond("Task completed")
		}).
		WithTasks(
			func(task *testcase.TaskConfig) {
				task.Name("easy-pass").Easy().Prompt("Easy task pass").VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("easy-fail").Easy().Prompt("Easy task fail").VerifyScript("exit 1")
			},
			func(task *testcase.TaskConfig) {
				task.Name("medium-pass").Medium().Prompt("Medium task pass").VerifyScript("exit 0")
			},
			func(task *testcase.TaskConfig) {
				task.Name("hard-fail").Hard().Prompt("Hard task fail").VerifyScript("exit 1")
			},
		).
		WithEval(func(eval *testcase.EvalConfig) {
			eval.Name("mixed-eval")
		}).
		ExpectResultCount(4).
		ExpectPassedCount(2).
		ExpectFailedCount(2).
		ExpectTaskPassedByName("easy-pass").
		ExpectTaskFailedByName("easy-fail").
		ExpectTaskPassedByName("medium-pass").
		ExpectTaskFailedByName("hard-fail").
		Expect(testcase.AssertFunc("verify-difficulty-pass-rates", func(t *testing.T, ctx *testcase.RunContext) {
			// Count passing tasks by difficulty
			easyPass, mediumPass, hardPass := 0, 0, 0
			for _, r := range ctx.EvalResults {
				if r.TaskPassed {
					switch r.Difficulty {
					case "easy":
						easyPass++
					case "medium":
						mediumPass++
					case "hard":
						hardPass++
					}
				}
			}
			if easyPass != 1 {
				t.Errorf("expected 1 easy task to pass, got %d", easyPass)
			}
			if mediumPass != 1 {
				t.Errorf("expected 1 medium task to pass, got %d", mediumPass)
			}
			if hardPass != 0 {
				t.Errorf("expected 0 hard tasks to pass, got %d", hardPass)
			}
		})).
		Run()
}
