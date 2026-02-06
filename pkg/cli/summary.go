package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/mcpchecker/mcpchecker/pkg/eval"
	"github.com/mcpchecker/mcpchecker/pkg/results"
	"github.com/spf13/cobra"
)

type SummaryOutput struct {
	ResultsFile         string        `json:"resultsFile"`
	Tasks               []TaskSummary `json:"tasks"`
	TasksTotal          int           `json:"tasksTotal"`
	TasksPassed         int           `json:"tasksPassed"`
	TaskPassRate        float64       `json:"taskPassRate"`
	AssertionsTotal     int           `json:"assertionsTotal"`
	AssertionsPassed    int           `json:"assertionsPassed"`
	AssertionPassRate   float64       `json:"assertionPassRate"`
	TotalTokensEstimate int64         `json:"totalTokensEstimate"`
}

type TaskSummary struct {
	Name             string   `json:"name"`
	TaskPassed       bool     `json:"taskPassed"`
	AssertionsPassed bool     `json:"assertionsPassed"`
	TaskError        string   `json:"taskError,omitempty"`
	FailedAssertions []string `json:"failedAssertions,omitempty"`
	TokensEstimated  int64    `json:"tokensEstimated,omitempty"`
	TokenError       string   `json:"tokenError,omitempty"`
}

func NewSummaryCmd() *cobra.Command {
	var taskFilter string
	var outputFormat string
	var githubOutput bool

	cmd := &cobra.Command{
		Use:   "summary <results-file>",
		Short: "Show a compact summary of evaluation results",
		Long: `Display a concise summary of evaluation results showing pass/fail status per task.

Supports multiple output formats:
  - text (default): Human-readable summary with colors
  - json: Machine-readable JSON output
  - --github-output: GitHub Actions format (key=value)`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resultsFile := args[0]

			evalResults, err := results.Load(resultsFile)
			if err != nil {
				return fmt.Errorf("failed to load results file: %w", err)
			}

			if taskFilter != "" {
				evalResults = results.Filter(evalResults, taskFilter)
			}

			summary := buildSummaryOutput(resultsFile, evalResults)

			if githubOutput {
				outputGitHubSummary(summary)
				return nil
			}

			switch outputFormat {
			case "json":
				return outputJSONSummary(summary)
			case "text":
				outputTextSummary(evalResults, summary)
			default:
				return fmt.Errorf("unknown output format: %s", outputFormat)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&taskFilter, "task", "", "Filter results by task name")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text, json)")
	cmd.Flags().BoolVar(&githubOutput, "github-output", false, "Output in GitHub Actions format (key=value)")

	return cmd
}

func buildSummaryOutput(resultsFile string, evalResults []*eval.EvalResult) SummaryOutput {
	summary := SummaryOutput{
		ResultsFile: resultsFile,
		Tasks:       make([]TaskSummary, 0, len(evalResults)),
		TasksTotal:  len(evalResults),
	}

	for _, result := range evalResults {
		taskSummary := TaskSummary{
			Name:             result.TaskName,
			TaskPassed:       result.TaskPassed,
			AssertionsPassed: result.AllAssertionsPassed,
		}

		if result.TaskPassed {
			summary.TasksPassed++
		}

		// Collect task error
		if !result.TaskPassed {
			if result.AgentExecutionError {
				taskSummary.TaskError = "Agent execution failed"
			} else if result.TaskError != "" {
				taskSummary.TaskError = result.TaskError
			}
		}

		// Count assertions and collect failures
		if result.AssertionResults != nil {
			summary.AssertionsTotal += result.AssertionResults.TotalAssertions()
			summary.AssertionsPassed += result.AssertionResults.PassedAssertions()

			if !result.AllAssertionsPassed {
				taskSummary.FailedAssertions = results.CollectFailedAssertions(result.AssertionResults)
			}
		}

		// Collect token estimates
		if result.TokenEstimate != nil {
			taskSummary.TokensEstimated = result.TokenEstimate.TotalTokens
			taskSummary.TokenError = result.TokenEstimate.Error
			summary.TotalTokensEstimate += result.TokenEstimate.TotalTokens
		}

		summary.Tasks = append(summary.Tasks, taskSummary)
	}

	// Calculate pass rates
	if summary.TasksTotal > 0 {
		summary.TaskPassRate = float64(summary.TasksPassed) / float64(summary.TasksTotal)
	}
	if summary.AssertionsTotal > 0 {
		summary.AssertionPassRate = float64(summary.AssertionsPassed) / float64(summary.AssertionsTotal)
	}

	return summary
}

func outputTextSummary(evalResults []*eval.EvalResult, summary SummaryOutput) {
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)
	bold := color.New(color.Bold)

	bold.Println("=== Evaluation Summary ===")
	fmt.Println()

	for i, result := range evalResults {
		taskSummary := summary.Tasks[i]

		// Determine overall status
		passed := result.TaskPassed && result.AllAssertionsPassed

		// Count task assertions
		var taskAssertionsPassed, taskAssertionsTotal int
		if result.AssertionResults != nil {
			taskAssertionsPassed = result.AssertionResults.PassedAssertions()
			taskAssertionsTotal = result.AssertionResults.TotalAssertions()
		}

		// Print task line
		if passed {
			green.Printf("  ✓ %s", result.TaskName)
		} else if result.TaskPassed && !result.AllAssertionsPassed {
			yellow.Printf("  ~ %s", result.TaskName)
		} else {
			red.Printf("  ✗ %s", result.TaskName)
		}

		// Print assertion count if any
		if taskAssertionsTotal > 0 {
			fmt.Printf(" (assertions: %d/%d)", taskAssertionsPassed, taskAssertionsTotal)
		}
		fmt.Println()

		// Print failure details
		if taskSummary.TaskError != "" {
			fmt.Printf("      %s\n", taskSummary.TaskError)
		}

		// Print failed assertions
		for _, failure := range taskSummary.FailedAssertions {
			red.Printf("      - %s\n", failure)
		}
	}

	// Print totals
	fmt.Println()
	fmt.Printf("Tasks:      %d/%d passed (%.2f%%)\n",
		summary.TasksPassed, summary.TasksTotal, summary.TaskPassRate*100)
	fmt.Printf("Assertions: %d/%d passed (%.2f%%)\n",
		summary.AssertionsPassed, summary.AssertionsTotal, summary.AssertionPassRate*100)
	if summary.TotalTokensEstimate > 0 {
		// Check if any task had token errors
		hasTokenErrors := false
		for _, task := range summary.Tasks {
			if task.TokenError != "" {
				hasTokenErrors = true
				break
			}
		}
		if hasTokenErrors {
			fmt.Printf("Tokens:     ~%d (incomplete - some counts failed)\n", summary.TotalTokensEstimate)
		} else {
			fmt.Printf("Tokens:     ~%d (estimate - excludes system prompt & cache)\n", summary.TotalTokensEstimate)
		}
	}
}

func outputJSONSummary(summary SummaryOutput) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(summary)
}

func outputGitHubSummary(summary SummaryOutput) {
	fmt.Printf("results-file=%s\n", summary.ResultsFile)
	fmt.Printf("tasks-total=%d\n", summary.TasksTotal)
	fmt.Printf("tasks-passed=%d\n", summary.TasksPassed)
	fmt.Printf("task-pass-rate=%.4f\n", summary.TaskPassRate)
	fmt.Printf("assertions-total=%d\n", summary.AssertionsTotal)
	fmt.Printf("assertions-passed=%d\n", summary.AssertionsPassed)
	fmt.Printf("assertion-pass-rate=%.4f\n", summary.AssertionPassRate)
	fmt.Printf("tokens-estimated=%d\n", summary.TotalTokensEstimate)
}
