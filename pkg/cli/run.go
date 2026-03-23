package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/mcpchecker/mcpchecker/pkg/eval"
	"github.com/mcpchecker/mcpchecker/pkg/util"
	"github.com/spf13/cobra"
)

// NewEvalCmd creates the run command
func NewEvalCmd() *cobra.Command {
	var outputFormat string
	var verbose bool
	var run string
	var labelSelector string
	var parallelWorkers int
	var runs int
	var mcpConfigFile string

	cmd := &cobra.Command{
		Use:   "check [eval-config-file]",
		Short: "Run an evaluation",
		Long:  `Run an evaluation using the specified eval configuration file.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			startTime := time.Now()
			configFile := args[0]

			// Load eval spec
			spec, err := eval.FromFile(configFile)
			if err != nil {
				return fmt.Errorf("failed to load eval config: %w", err)
			}

			overrideFile := func(specFile *string, fileName string) error {
				if !filepath.IsAbs(fileName) {
					absPath, err := filepath.Abs(fileName)
					if err != nil {
						return err
					}
					fileName = absPath
				}
				*specFile = fileName
				return nil
			}

			// Override MCP config file if flag is specified
			if mcpConfigFile != "" {
				err = overrideFile(&spec.Config.McpConfigFile, mcpConfigFile)
				if err != nil {
					return fmt.Errorf("failed to resolve mcp config file: %w", err)
				}
			}

			// Apply label selector filter if provided
			if labelSelector != "" {
				if err := eval.ApplyLabelSelectorFilter(spec, labelSelector); err != nil {
					return fmt.Errorf("failed to apply label selector: %w", err)
				}
			}

			// Create runner
			runner, err := eval.NewRunner(spec, eval.RunnerOptions{
				ParallelWorkers:   parallelWorkers,
				Runs:              runs,
				RunsExplicitlySet: cmd.Flags().Changed("runs"),
			})
			if err != nil {
				return fmt.Errorf("failed to create eval runner: %w", err)
			}

			// Create progress display
			display := newProgressDisplay(verbose)

			// Run with progress
			ctx := context.Background()
			ctx = util.WithVerbose(ctx, verbose)
			results, err := runner.RunWithProgress(ctx, run, display.handleProgress)
			if err != nil {
				return fmt.Errorf("eval failed: %w", err)
			}

			// Save results to JSON file
			outputFile := fmt.Sprintf("mcpchecker-%s-out.json", spec.Metadata.Name)
			if err := saveResultsToFile(results, outputFile); err != nil {
				return fmt.Errorf("failed to save results to file: %w", err)
			}
			if outputFormat == "text" {
				fmt.Printf("\n📄 Results saved to: %s\n", outputFile)
			}

			// Display results
			if err := displayResults(results, outputFormat); err != nil {
				return fmt.Errorf("failed to display results: %w", err)
			}

			// Print elapsed time (only for text output to keep JSON machine-readable)
			if outputFormat == "text" {
				elapsed := time.Since(startTime)
				fmt.Printf("⏱️  Completed in %s\n", formatDuration(elapsed))
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text, json)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	cmd.Flags().StringVarP(&run, "run", "r", "", "Regular expression to match task names to run (unanchored, like go test -run)")
	cmd.Flags().StringVarP(&labelSelector, "label-selector", "l", "", "Filter taskSets by label (format: key=value, e.g., suite=kubernetes)")
	cmd.Flags().IntVarP(&parallelWorkers, "parallel", "p", 1, "Number of parallel workers for tasks marked as parallel (1 = sequential)")
	cmd.Flags().IntVarP(&runs, "runs", "n", 1, "Number of times to run each task (for consistency testing)")
	cmd.Flags().StringVar(&mcpConfigFile, "mcp-config-file", "", "Path to MCP config file (overrides value in eval config)")

	return cmd
}

// progressDisplay handles interactive progress display
type progressDisplay struct {
	mu      sync.Mutex
	verbose bool
	green   *color.Color
	red     *color.Color
	yellow  *color.Color
	cyan    *color.Color
	bold    *color.Color
}

func newProgressDisplay(verbose bool) *progressDisplay {
	return &progressDisplay{
		verbose: verbose,
		green:   color.New(color.FgGreen),
		red:     color.New(color.FgRed),
		yellow:  color.New(color.FgYellow),
		cyan:    color.New(color.FgCyan),
		bold:    color.New(color.Bold),
	}
}

// taskPrefix returns a prefix for progress output. For parallel tasks, includes task name.
func taskPrefix(task *eval.EvalResult) string {
	if task != nil && task.Parallel {
		return fmt.Sprintf("[%s] ", task.TaskName)
	}
	return "  "
}

func (d *progressDisplay) handleProgress(event eval.ProgressEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()

	prefix := taskPrefix(event.Task)

	switch event.Type {
	case eval.EventEvalStart:
		d.bold.Println("\n=== Starting Evaluation ===")

	case eval.EventTaskStart:
		fmt.Println()
		runInfo := ""
		if event.Task.TotalRuns > 1 {
			runInfo = fmt.Sprintf(" [run %d/%d]", event.Task.RunIndex+1, event.Task.TotalRuns)
		}
		if event.Task.Parallel {
			if event.Task.Difficulty != "" {
				d.cyan.Printf("[%s]%s Starting (parallel, %s)\n", event.Task.TaskName, runInfo, event.Task.Difficulty)
			} else {
				d.cyan.Printf("[%s]%s Starting (parallel)\n", event.Task.TaskName, runInfo)
			}
		} else {
			d.cyan.Printf("Task: %s%s\n", event.Task.TaskName, runInfo)
			if event.Task.Difficulty != "" {
				fmt.Printf("  Difficulty: %s\n", event.Task.Difficulty)
			}
		}

	case eval.EventTaskSetup:
		if d.verbose {
			fmt.Printf("%s→ Setting up task environment...\n", prefix)
		}

	case eval.EventTaskRunning:
		fmt.Printf("%s→ Running agent...\n", prefix)

	case eval.EventTaskVerifying:
		fmt.Printf("%s→ Verifying results...\n", prefix)

	case eval.EventTaskAssertions:
		if d.verbose {
			fmt.Printf("%s→ Evaluating assertions...\n", prefix)
		}

	case eval.EventTaskError:
		task := event.Task
		d.red.Printf("%s✗ Task failed during setup\n", prefix)
		if task.TaskError != "" {
			fmt.Printf("%s  Error: %s\n", prefix, task.TaskError)
		}

	case eval.EventTaskComplete:
		task := event.Task
		if task.TaskPassed && task.AllAssertionsPassed {
			d.green.Printf("%s✓ Task passed\n", prefix)
		} else if task.TaskPassed && !task.AllAssertionsPassed {
			d.yellow.Printf("%s~ Task passed but assertions failed\n", prefix)
		} else {
			if task.AgentExecutionError {
				d.red.Printf("%s✗ Agent failed to run\n", prefix)
				if task.TaskError != "" || task.TaskOutput != "" {
					errorFile, err := saveErrorToFile(task.TaskName, task.TaskError, task.TaskOutput)
					if err != nil {
						fmt.Printf("%s  Error: %s\n", prefix, task.TaskError)
					} else {
						fmt.Printf("%s  Error details saved to: %s\n", prefix, errorFile)
					}
				}
			} else {
				d.red.Printf("%s✗ Task failed\n", prefix)
				if task.TaskError != "" {
					fmt.Printf("%s  Error: %s\n", prefix, task.TaskError)
				}
			}
		}

	case eval.EventEvalComplete:
		fmt.Println()
		d.bold.Println("=== Evaluation Complete ===")
	}
}

func displayResults(results []*eval.EvalResult, format string) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(results)

	case "text":
		return displayTextResults(results)

	default:
		return fmt.Errorf("unknown output format: %s", format)
	}
}

func displayTextResults(results []*eval.EvalResult) error {
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)
	bold := color.New(color.Bold)

	fmt.Println()
	bold.Println("=== Results Summary ===")
	fmt.Println()

	totalTasks := len(results)
	tasksPassed := 0
	totalAssertions := 0
	passedAssertions := 0
	verificationFailedButAssertionsPassed := 0
	verificationFailedButAssertionsPassedTotal := 0
	verificationFailedButAssertionsPassedCount := 0

	for _, result := range results {
		if result.TaskPassed {
			tasksPassed++
		}

		// Track cases where verification failed but assertions passed
		if !result.TaskPassed && result.AllAssertionsPassed && !result.AgentExecutionError {
			verificationFailedButAssertionsPassed++
		}

		// Count individual assertions
		if result.AssertionResults != nil {
			totalAssertions += result.AssertionResults.TotalAssertions()
			passedAssertions += result.AssertionResults.PassedAssertions()

			// Track assertions for verification-failed tasks
			if !result.TaskPassed && !result.AgentExecutionError {
				verificationFailedButAssertionsPassedTotal += result.AssertionResults.TotalAssertions()
				verificationFailedButAssertionsPassedCount += result.AssertionResults.PassedAssertions()
			}
		}

		// Display individual result
		fmt.Printf("Task: %s\n", result.TaskName)
		fmt.Printf("  Path: %s\n", result.TaskPath)
		if result.Difficulty != "" {
			fmt.Printf("  Difficulty: %s\n", result.Difficulty)
		}

		if result.TaskPassed {
			green.Printf("  Task Status: PASSED\n")
		} else {
			if result.AgentExecutionError {
				red.Printf("  Task Status: FAILED (Agent execution error)\n")
				if result.TaskError != "" || result.TaskOutput != "" {
					errorFile, err := saveErrorToFile(result.TaskName, result.TaskError, result.TaskOutput)
					if err != nil {
						// If we can't save to file, fall back to printing inline
						fmt.Printf("  Error: %s\n", result.TaskError)
					} else {
						fmt.Printf("  Error details saved to: %s\n", errorFile)
					}
				}
			} else {
				// Check if assertions passed but verification failed
				if result.AllAssertionsPassed {
					yellow.Printf("  Task Status: FAILED (Verification failed, but assertions passed)\n")
				} else {
					red.Printf("  Task Status: FAILED\n")
				}
				if result.TaskError != "" {
					fmt.Printf("  Error: %s\n", result.TaskError)
				}
			}
		}

		if result.AssertionResults != nil {
			passed := result.AssertionResults.PassedAssertions()
			total := result.AssertionResults.TotalAssertions()
			if result.AllAssertionsPassed {
				green.Printf("  Assertions: PASSED (%d/%d)\n", passed, total)
			} else {
				yellow.Printf("  Assertions: FAILED (%d/%d)\n", passed, total)
				printFailedAssertions(result.AssertionResults)
			}
		}

		fmt.Println()
	}

	bold.Println("=== Overall Statistics ===")
	fmt.Printf("Total Tasks: %d\n", totalTasks)

	if tasksPassed == totalTasks {
		green.Printf("Tasks Passed: %d/%d\n", tasksPassed, totalTasks)
	} else {
		yellow.Printf("Tasks Passed: %d/%d\n", tasksPassed, totalTasks)
	}

	if totalAssertions > 0 {
		if passedAssertions == totalAssertions {
			green.Printf("Assertions Passed: %d/%d\n", passedAssertions, totalAssertions)
		} else {
			yellow.Printf("Assertions Passed: %d/%d\n", passedAssertions, totalAssertions)
		}
	}

	// Show stats for verification-failed tasks
	if verificationFailedButAssertionsPassed > 0 {
		fmt.Println()
		yellow.Printf("Tasks where verification failed but assertions passed: %d\n", verificationFailedButAssertionsPassed)
		if verificationFailedButAssertionsPassedTotal > 0 {
			yellow.Printf("  Assertions in these tasks: %d/%d\n",
				verificationFailedButAssertionsPassedCount,
				verificationFailedButAssertionsPassedTotal)
		}
	}

	// Display token estimates
	var totalTokens int64
	var totalMcpSchemaTokens int64
	hasTokenErrors := false
	for _, result := range results {
		if result.TokenEstimate != nil {
			totalTokens += result.TokenEstimate.TotalTokens
			totalMcpSchemaTokens += result.TokenEstimate.McpSchemaTokens
			if result.TokenEstimate.Error != "" {
				hasTokenErrors = true
			}
		}
	}
	printTokenSummary(totalTokens, totalMcpSchemaTokens, hasTokenErrors)

	// Group by difficulty
	fmt.Println()
	bold.Println("=== Statistics by Difficulty ===")
	displayStatsByDifficulty(results, green, yellow)

	// Show consistency summary for multi-run
	displayConsistencySummary(results)

	return nil
}

func displayStatsByDifficulty(results []*eval.EvalResult, green *color.Color, yellow *color.Color) {
	// Group results by difficulty
	type difficultyStats struct {
		totalTasks       int
		tasksPassed      int
		totalAssertions  int
		passedAssertions int
	}

	statsByDifficulty := make(map[string]*difficultyStats)

	for _, result := range results {
		difficulty := result.Difficulty
		if difficulty == "" {
			difficulty = "unspecified"
		}

		if statsByDifficulty[difficulty] == nil {
			statsByDifficulty[difficulty] = &difficultyStats{}
		}

		stats := statsByDifficulty[difficulty]
		stats.totalTasks++

		if result.TaskPassed {
			stats.tasksPassed++
		}

		if result.AssertionResults != nil {
			stats.totalAssertions += result.AssertionResults.TotalAssertions()
			stats.passedAssertions += result.AssertionResults.PassedAssertions()
		}
	}

	// Display stats in order: easy, medium, hard, then any others
	orderedDifficulties := []string{"easy", "medium", "hard"}

	for _, difficulty := range orderedDifficulties {
		stats, exists := statsByDifficulty[difficulty]
		if !exists {
			continue
		}

		fmt.Printf("\n%s:\n", difficulty)

		if stats.tasksPassed == stats.totalTasks {
			green.Printf("  Tasks: %d/%d\n", stats.tasksPassed, stats.totalTasks)
		} else {
			yellow.Printf("  Tasks: %d/%d\n", stats.tasksPassed, stats.totalTasks)
		}

		if stats.totalAssertions > 0 {
			if stats.passedAssertions == stats.totalAssertions {
				green.Printf("  Assertions: %d/%d\n", stats.passedAssertions, stats.totalAssertions)
			} else {
				yellow.Printf("  Assertions: %d/%d\n", stats.passedAssertions, stats.totalAssertions)
			}
		}
	}

	// Display any other difficulties (e.g., "unspecified") that weren't in the main list
	for difficulty, stats := range statsByDifficulty {
		isStandard := false
		for _, d := range orderedDifficulties {
			if d == difficulty {
				isStandard = true
				break
			}
		}
		if isStandard {
			continue
		}

		fmt.Printf("\n%s:\n", difficulty)

		if stats.tasksPassed == stats.totalTasks {
			green.Printf("  Tasks: %d/%d\n", stats.tasksPassed, stats.totalTasks)
		} else {
			fmt.Printf("  Tasks: %d/%d\n", stats.tasksPassed, stats.totalTasks)
		}

		if stats.totalAssertions > 0 {
			if stats.passedAssertions == stats.totalAssertions {
				green.Printf("  Assertions: %d/%d\n", stats.passedAssertions, stats.totalAssertions)
			} else {
				fmt.Printf("  Assertions: %d/%d\n", stats.passedAssertions, stats.totalAssertions)
			}
		}
	}
}

func printFailedAssertions(results *eval.CompositeAssertionResult) {
	printSingleAssertion("ToolsUsed", results.ToolsUsed)
	printSingleAssertion("RequireAny", results.RequireAny)
	printSingleAssertion("ToolsNotUsed", results.ToolsNotUsed)
	printSingleAssertion("MinToolCalls", results.MinToolCalls)
	printSingleAssertion("MaxToolCalls", results.MaxToolCalls)
	printSingleAssertion("ResourcesRead", results.ResourcesRead)
	printSingleAssertion("ResourcesNotRead", results.ResourcesNotRead)
	printSingleAssertion("PromptsUsed", results.PromptsUsed)
	printSingleAssertion("PromptsNotUsed", results.PromptsNotUsed)
	printSingleAssertion("CallOrder", results.CallOrder)
	printSingleAssertion("NoDuplicateCalls", results.NoDuplicateCalls)
}

func printSingleAssertion(name string, result *eval.SingleAssertionResult) {
	if result != nil && !result.Passed {
		fmt.Printf("    - %s: %s\n", name, result.Reason)
		for _, detail := range result.Details {
			fmt.Printf("      %s\n", detail)
		}
	}
}

func saveResultsToFile(results []*eval.EvalResult, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		return fmt.Errorf("failed to encode results: %w", err)
	}

	return nil
}

// saveErrorToFile saves task error and output to a file and returns the filename
func saveErrorToFile(taskName, taskError, taskOutput string) (string, error) {
	// Create a safe filename from task name
	safeTaskName := strings.ReplaceAll(taskName, "/", "-")
	safeTaskName = strings.ReplaceAll(safeTaskName, " ", "-")
	filename := fmt.Sprintf("%s-error.txt", safeTaskName)

	content := ""
	if taskError != "" {
		content += fmt.Sprintf("=== Error ===\n%s\n", taskError)
	}
	if taskOutput != "" {
		content += fmt.Sprintf("\n=== Output ===\n%s\n", taskOutput)
	}

	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write error file: %w", err)
	}

	absPath, err := filepath.Abs(filename)
	if err != nil {
		return filename, nil // Return relative path if we can't get absolute
	}

	return absPath, nil
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if minutes < 60 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	hours := minutes / 60
	minutes = minutes % 60
	return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
}

// displayConsistencySummary shows pass rates when tasks are run multiple times
func displayConsistencySummary(results []*eval.EvalResult) {
	// Check if any task has multiple runs
	hasMultiRun := false
	for _, r := range results {
		if r.TotalRuns > 1 {
			hasMultiRun = true
			break
		}
	}

	if !hasMultiRun {
		return
	}

	bold := color.New(color.Bold)
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)

	// Aggregate by task path
	type taskAgg struct {
		taskName  string
		passCount int
		totalRuns int
	}
	agg := make(map[string]*taskAgg)

	for _, r := range results {
		key := r.TaskPath
		if agg[key] == nil {
			agg[key] = &taskAgg{taskName: r.TaskName}
		}
		a := agg[key]
		a.totalRuns++
		if r.TaskPassed {
			a.passCount++
		}
	}

	fmt.Println()
	bold.Println("=== Consistency Summary ===")
	fmt.Printf("%-40s %s\n", "Task", "Pass Rate")
	fmt.Println(strings.Repeat("-", 55))

	for _, a := range agg {
		passRate := float64(a.passCount) / float64(a.totalRuns) * 100
		status := fmt.Sprintf("%d/%d (%.1f%%)", a.passCount, a.totalRuns, passRate)
		if a.passCount == a.totalRuns {
			fmt.Printf("%-40s ", a.taskName)
			green.Printf("%s\n", status)
		} else if a.passCount == 0 {
			fmt.Printf("%-40s ", a.taskName)
			yellow.Printf("%s\n", status)
		} else {
			fmt.Printf("%-40s %s\n", a.taskName, status)
		}
	}
}
