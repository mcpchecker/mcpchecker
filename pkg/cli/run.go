package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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
	var parallel int

	cmd := &cobra.Command{
		Use:   "check [eval-config-file]",
		Short: "Run an evaluation",
		Long:  `Run an evaluation using the specified eval configuration file.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configFile := args[0]

			// Load eval spec
			spec, err := eval.FromFile(configFile)
			if err != nil {
				return fmt.Errorf("failed to load eval config: %w", err)
			}

			// Apply label selector filter if provided
			if labelSelector != "" {
				if err := eval.ApplyLabelSelectorFilter(spec, labelSelector); err != nil {
					return fmt.Errorf("failed to apply label selector: %w", err)
				}
			}

			// Set parallelism (0 = auto-detect based on CPU count)
			if parallel == 0 {
				parallel = runtime.NumCPU()
			}

			// Create runner with parallelism
			runner, err := eval.NewRunnerWithParallelism(spec, parallel)
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
			fmt.Printf("\n📄 Results saved to: %s\n", outputFile)

			// Display results
			if err := displayResults(results, outputFormat); err != nil {
				return fmt.Errorf("failed to display results: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text, json)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	cmd.Flags().StringVarP(&run, "run", "r", "", "Regular expression to match task names to run (unanchored, like go test -run)")
	cmd.Flags().StringVarP(&labelSelector, "label-selector", "l", "", "Filter taskSets by label (format: key=value, e.g., suite=kubernetes)")
	cmd.Flags().IntVarP(&parallel, "parallel", "p", 1, "Number of parallel tasks (0 = auto-detect CPU count, 1 = sequential)")

	return cmd
}

// progressDisplay handles interactive progress display
type progressDisplay struct {
	verbose    bool
	mu         sync.Mutex
	results    map[string]*eval.EvalResult
	running    int
	passed     int
	failed     int
	total      int
	startTime  time.Time
	ticker     *time.Ticker
	stopTicker chan bool
}

func newProgressDisplay(verbose bool) *progressDisplay {
	return &progressDisplay{
		verbose: verbose,
		results: make(map[string]*eval.EvalResult),
	}
}

func (d *progressDisplay) renderProgress() {
	if d.ticker == nil {
		return
	}

	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	elapsed := time.Since(d.startTime)
	frame := spinnerFrames[int(elapsed.Milliseconds()/100)%len(spinnerFrames)]
	completed := d.passed + d.failed

	fmt.Fprintf(os.Stderr, "\r%s Running: %d | Passed: %d | Failed: %d | Completed: %d/%d | Elapsed: %ds\033[K",
		frame, d.running, d.passed, d.failed, completed, d.total, int(elapsed.Seconds()))
}

func (d *progressDisplay) handleProgress(event eval.ProgressEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch event.Type {
	case eval.EventSetupStart:
		fmt.Println("\n=== Initializing Evaluation ===")

	case eval.EventSetupStep:
		fmt.Printf("  → %s\n", event.Message)

	case eval.EventSetupComplete:

	case eval.EventEvalStart:
		d.total = event.TotalTasks / 4

	case eval.EventTaskStart:
		if event.Task != nil {
			d.results[event.Task.TaskName] = event.Task
			d.running++

			if d.startTime.IsZero() {
				fmt.Println("\n=== Running Tasks ===")
				d.startTime = time.Now()

				d.ticker = time.NewTicker(100 * time.Millisecond)
				d.stopTicker = make(chan bool)
				ticker := d.ticker
				go func() {
					for {
						select {
						case <-ticker.C:
							d.mu.Lock()
							d.renderProgress()
							d.mu.Unlock()
						case <-d.stopTicker:
							return
						}
					}
				}()
			}
			d.renderProgress()
		}

	case eval.EventTaskSetup, eval.EventTaskRunning, eval.EventTaskVerifying, eval.EventTaskAssertions:

	case eval.EventTaskComplete, eval.EventTaskError:
		if event.Task != nil {
			d.results[event.Task.TaskName] = event.Task
			if event.Task.TaskPassed {
				d.passed++
			} else {
				d.failed++
			}
			d.running--
			d.renderProgress()
		}

	case eval.EventEvalComplete:
		if d.ticker != nil {
			d.ticker.Stop()
			close(d.stopTicker)
			d.ticker = nil
		}

		elapsed := time.Since(d.startTime)
		fmt.Fprintf(os.Stderr, "\r✓ Done in %ds (Passed: %d, Failed: %d)\n",
			int(elapsed.Seconds()), d.passed, d.failed)

		d.displayBufferedResults()

		fmt.Println("\n=== Evaluation Complete ===")
	}
}

func (d *progressDisplay) displayBufferedResults() {
	names := make([]string, 0, len(d.results))
	for name := range d.results {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		result := d.results[name]
		d.displayTaskResult(result)
	}
}

func (d *progressDisplay) displayTaskResult(result *eval.EvalResult) {
	fmt.Println()
	fmt.Printf("Task: %s\n", result.TaskName)
	if result.Difficulty != "" {
		fmt.Printf("  Difficulty: %s\n", result.Difficulty)
	}

	if result.TaskPassed && result.AllAssertionsPassed {
		fmt.Printf("  ✓ Task passed\n")
	} else if result.TaskPassed && !result.AllAssertionsPassed {
		fmt.Printf("  ~ Task passed but assertions failed\n")
	} else {
		if result.AgentExecutionError {
			fmt.Printf("  ✗ Agent failed to run\n")
			if result.TaskError != "" || result.TaskOutput != "" {
				errorFile, err := saveErrorToFile(result.TaskName, result.TaskError, result.TaskOutput)
				if err != nil {
					fmt.Printf("    Error: %s\n", result.TaskError)
				} else {
					fmt.Printf("    Error details saved to: %s\n", errorFile)
				}
			}
		} else {
			fmt.Printf("  ✗ Task failed\n")
			if result.TaskError != "" {
				fmt.Printf("    Error: %s\n", result.TaskError)
			}
		}
	}

	if d.verbose && result.TaskOutput != "" {
		fmt.Printf("  Output: %s\n", result.TaskOutput)
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

	// Group by difficulty
	fmt.Println()
	bold.Println("=== Statistics by Difficulty ===")
	displayStatsByDifficulty(results, green, yellow)

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
