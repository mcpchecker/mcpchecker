package cli

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/mcpchecker/mcpchecker/pkg/eval"
	"github.com/mcpchecker/mcpchecker/pkg/results"
	"github.com/spf13/cobra"
)

// DiffResult holds the comparison between two evaluation runs
type DiffResult struct {
	BaseStats           results.Stats
	HeadStats           results.Stats
	Regressions         []TaskDiff
	Improvements        []TaskDiff
	New                 []TaskDiff
	Removed             []TaskDiff
	TokenDataIncomplete bool // true if any task has incomplete token data
}

// TaskDiff holds the diff for a single task
type TaskDiff struct {
	TaskName           string
	BasePassed         bool
	HeadPassed         bool
	BaseAssertions     int
	HeadAssertions     int
	BaseAssertionTotal int
	HeadAssertionTotal int
	FailureReason      string
}

// NewDiffCmd creates the diff command
func NewDiffCmd() *cobra.Command {
	var outputFormat string
	var baseFile string
	var currentFile string

	cmd := &cobra.Command{
		Use:   "diff --base <results-file> --current <results-file>",
		Short: "Compare two evaluation results",
		Long: `Compare evaluation results between two runs (e.g., main vs PR).

Shows regressions, improvements, and overall pass rate changes.
Useful for posting on pull requests to show impact of changes.

Example:
  mcpchecker diff --base results-main.json --current results-pr.json
  mcpchecker diff --base results-main.json --current results-pr.json --output markdown`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseResults, err := results.Load(baseFile)
			if err != nil {
				return fmt.Errorf("failed to load base results: %w", err)
			}

			currentResults, err := results.Load(currentFile)
			if err != nil {
				return fmt.Errorf("failed to load current results: %w", err)
			}

			diff := calculateDiff(baseFile, currentFile, baseResults, currentResults)

			switch outputFormat {
			case "text":
				outputTextDiff(diff)
			case "markdown":
				outputMarkdownDiff(diff)
			default:
				return fmt.Errorf("unknown output format: %s", outputFormat)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&baseFile, "base", "", "Base results file (e.g., main branch)")
	cmd.Flags().StringVar(&currentFile, "current", "", "Current results file (e.g., PR branch)")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text, markdown)")

	_ = cmd.MarkFlagRequired("base")
	_ = cmd.MarkFlagRequired("current")

	return cmd
}

func hasTokenErrors(results []*eval.EvalResult) bool {
	for _, r := range results {
		if r.TokenEstimate != nil && r.TokenEstimate.Error != "" {
			return true
		}
	}
	return false
}

func calculateDiff(baseFile, currentFile string, baseResults, currentResults []*eval.EvalResult) DiffResult {
	diff := DiffResult{
		BaseStats:           results.CalculateStats(baseFile, baseResults),
		HeadStats:           results.CalculateStats(currentFile, currentResults),
		Regressions:         make([]TaskDiff, 0),
		Improvements:        make([]TaskDiff, 0),
		New:                 make([]TaskDiff, 0),
		Removed:             make([]TaskDiff, 0),
		TokenDataIncomplete: hasTokenErrors(baseResults) || hasTokenErrors(currentResults),
	}

	baseMap := make(map[string]*eval.EvalResult)
	for _, r := range baseResults {
		baseMap[r.TaskName] = r
	}

	currentMap := make(map[string]*eval.EvalResult)
	for _, r := range currentResults {
		currentMap[r.TaskName] = r
	}

	for _, current := range currentResults {
		base, exists := baseMap[current.TaskName]
		if !exists {
			diff.New = append(diff.New, TaskDiff{
				TaskName:           current.TaskName,
				HeadPassed:         current.TaskPassed && current.AllAssertionsPassed,
				HeadAssertions:     results.PassedAssertions(current),
				HeadAssertionTotal: results.TotalAssertions(current),
			})
			continue
		}

		basePassed := base.TaskPassed && base.AllAssertionsPassed
		currentPassed := current.TaskPassed && current.AllAssertionsPassed

		taskDiff := TaskDiff{
			TaskName:           current.TaskName,
			BasePassed:         basePassed,
			HeadPassed:         currentPassed,
			BaseAssertions:     results.PassedAssertions(base),
			HeadAssertions:     results.PassedAssertions(current),
			BaseAssertionTotal: results.TotalAssertions(base),
			HeadAssertionTotal: results.TotalAssertions(current),
			FailureReason:      results.FailureReason(current),
		}

		if basePassed && !currentPassed {
			diff.Regressions = append(diff.Regressions, taskDiff)
		} else if !basePassed && currentPassed {
			diff.Improvements = append(diff.Improvements, taskDiff)
		}
	}

	for _, base := range baseResults {
		if _, exists := currentMap[base.TaskName]; !exists {
			diff.Removed = append(diff.Removed, TaskDiff{
				TaskName:           base.TaskName,
				BasePassed:         base.TaskPassed && base.AllAssertionsPassed,
				BaseAssertions:     results.PassedAssertions(base),
				BaseAssertionTotal: results.TotalAssertions(base),
			})
		}
	}

	return diff
}

func outputTextDiff(diff DiffResult) {
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)
	bold := color.New(color.Bold)

	_, _ = bold.Println("=== Evaluation Diff ===")
	fmt.Println()

	// Regressions
	if len(diff.Regressions) > 0 {
		_, _ = red.Printf("Regressions (%d):\n", len(diff.Regressions))
		for _, r := range diff.Regressions {
			_, _ = red.Printf("  ✗ %s: PASSED → FAILED\n", r.TaskName)
			if r.FailureReason != "" {
				fmt.Printf("      %s\n", r.FailureReason)
			}
		}
		fmt.Println()
	}

	// Improvements
	if len(diff.Improvements) > 0 {
		_, _ = green.Printf("Improvements (%d):\n", len(diff.Improvements))
		for _, r := range diff.Improvements {
			_, _ = green.Printf("  ✓ %s: FAILED → PASSED\n", r.TaskName)
		}
		fmt.Println()
	}

	// New tasks
	if len(diff.New) > 0 {
		_, _ = yellow.Printf("New Tasks (%d):\n", len(diff.New))
		for _, r := range diff.New {
			if r.HeadPassed {
				_, _ = green.Printf("  + %s: PASSED\n", r.TaskName)
			} else {
				_, _ = red.Printf("  + %s: FAILED\n", r.TaskName)
			}
		}
		fmt.Println()
	}

	// Removed tasks
	if len(diff.Removed) > 0 {
		_, _ = yellow.Printf("Removed Tasks (%d):\n", len(diff.Removed))
		for _, r := range diff.Removed {
			fmt.Printf("  - %s\n", r.TaskName)
		}
		fmt.Println()
	}

	// Summary table
	_, _ = bold.Println("=== Summary ===")
	fmt.Println()

	taskChange := diff.HeadStats.TaskPassRate - diff.BaseStats.TaskPassRate
	assertionChange := diff.HeadStats.AssertionPassRate - diff.BaseStats.AssertionPassRate

	fmt.Printf("             Base        Head        Change\n")
	fmt.Printf("Tasks:       %d/%-8d %d/%-8d ",
		diff.BaseStats.TasksPassed, diff.BaseStats.TasksTotal,
		diff.HeadStats.TasksPassed, diff.HeadStats.TasksTotal)
	printChange(taskChange)

	fmt.Printf("Assertions:  %d/%-8d %d/%-8d ",
		diff.BaseStats.AssertionsPassed, diff.BaseStats.AssertionsTotal,
		diff.HeadStats.AssertionsPassed, diff.HeadStats.AssertionsTotal)
	printChange(assertionChange)

	// Token stats (only show if at least one side has actual token data)
	if diff.BaseStats.TasksWithTokens > 0 || diff.HeadStats.TasksWithTokens > 0 {
		fmt.Println()
		_, _ = bold.Println("=== Token Usage ===")
		fmt.Println()
		fmt.Printf("             Base        Head        Change\n")
		fmt.Printf("Tokens:      %-11s %-11s ", formatTokenCountOrNA(diff.BaseStats.TotalTokens, diff.BaseStats.TasksWithTokens), formatTokenCountOrNA(diff.HeadStats.TotalTokens, diff.HeadStats.TasksWithTokens))
		printTokenChangeWithCoverage(diff.BaseStats.TotalTokens, diff.BaseStats.TasksWithTokens, diff.HeadStats.TotalTokens, diff.HeadStats.TasksWithTokens)
		fmt.Printf("MCP Schema:  %-11s %-11s ", formatTokenCountOrNA(diff.BaseStats.McpSchemaTokens, diff.BaseStats.TasksWithTokens), formatTokenCountOrNA(diff.HeadStats.McpSchemaTokens, diff.HeadStats.TasksWithTokens))
		printTokenChangeWithCoverage(diff.BaseStats.McpSchemaTokens, diff.BaseStats.TasksWithTokens, diff.HeadStats.McpSchemaTokens, diff.HeadStats.TasksWithTokens)
		if diff.TokenDataIncomplete {
			yellow.Println("\n⚠️  Token counts may be incomplete due to errors during token estimation")
		}
	}
}

func printChange(change float64) {
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)

	if change > 0 {
		_, _ = green.Printf("+%.1f%%\n", change*100)
	} else if change < 0 {
		_, _ = red.Printf("%.1f%%\n", change*100)
	} else {
		fmt.Println("0.0%")
	}
}

func formatTokenCount(tokens int64) string {
	abs := tokens
	sign := ""
	if tokens < 0 {
		abs = -tokens
		sign = "-"
	}

	if abs >= 1_000_000 {
		return fmt.Sprintf("%s%.1fM", sign, float64(abs)/1_000_000)
	}
	if abs >= 1_000 {
		return fmt.Sprintf("%s%.1fK", sign, float64(abs)/1_000)
	}
	return fmt.Sprintf("%d", tokens)
}

// formatTokenCountOrNA returns "N/A" if no tasks have token data, otherwise formats the count.
func formatTokenCountOrNA(tokens int64, tasksWithTokens int) string {
	if tasksWithTokens == 0 {
		return "N/A"
	}
	return formatTokenCount(tokens)
}

// printTokenChangeWithCoverage handles token change display when one or both sides may lack token data.
func printTokenChangeWithCoverage(baseTokens int64, baseTasksWithTokens int, headTokens int64, headTasksWithTokens int) {
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)

	// If neither has token data, nothing to compare
	if baseTasksWithTokens == 0 && headTasksWithTokens == 0 {
		fmt.Println("-")
		return
	}

	// If only head has data, can't compare meaningfully
	if baseTasksWithTokens == 0 {
		fmt.Println("(no base data)")
		return
	}

	// If only base has data, can't compare meaningfully
	if headTasksWithTokens == 0 {
		fmt.Println("(no head data)")
		return
	}

	// Both have data, show normal comparison
	diff := headTokens - baseTokens
	if baseTokens == 0 {
		if headTokens > 0 {
			_, _ = red.Printf("+%s\n", formatTokenCount(headTokens))
		} else {
			fmt.Println("0")
		}
		return
	}

	pctChange := float64(diff) / float64(baseTokens) * 100
	if diff > 0 {
		_, _ = red.Printf("+%s (+%.1f%%)\n", formatTokenCount(diff), pctChange)
	} else if diff < 0 {
		_, _ = green.Printf("%s (%.1f%%)\n", formatTokenCount(diff), pctChange)
	} else {
		fmt.Println("0")
	}
}

func outputMarkdownDiff(diff DiffResult) {
	taskChange := diff.HeadStats.TaskPassRate - diff.BaseStats.TaskPassRate
	assertionChange := diff.HeadStats.AssertionPassRate - diff.BaseStats.AssertionPassRate

	fmt.Println("### 📊 Evaluation Results")
	fmt.Println()
	fmt.Println("| Metric | Base | Head | Change |")
	fmt.Println("|--------|------|------|--------|")
	fmt.Printf("| Tasks | %d/%d (%.1f%%) | %d/%d (%.1f%%) | %s |\n",
		diff.BaseStats.TasksPassed, diff.BaseStats.TasksTotal, diff.BaseStats.TaskPassRate*100,
		diff.HeadStats.TasksPassed, diff.HeadStats.TasksTotal, diff.HeadStats.TaskPassRate*100,
		formatChangeMarkdown(taskChange))
	fmt.Printf("| Assertions | %d/%d (%.1f%%) | %d/%d (%.1f%%) | %s |\n",
		diff.BaseStats.AssertionsPassed, diff.BaseStats.AssertionsTotal, diff.BaseStats.AssertionPassRate*100,
		diff.HeadStats.AssertionsPassed, diff.HeadStats.AssertionsTotal, diff.HeadStats.AssertionPassRate*100,
		formatChangeMarkdown(assertionChange))

	// Token stats (only show if at least one side has token data)
	if diff.BaseStats.TasksWithTokens > 0 || diff.HeadStats.TasksWithTokens > 0 {
		fmt.Printf("| Tokens | %s | %s | %s |\n",
			formatTokenCountOrNA(diff.BaseStats.TotalTokens, diff.BaseStats.TasksWithTokens),
			formatTokenCountOrNA(diff.HeadStats.TotalTokens, diff.HeadStats.TasksWithTokens),
			formatTokenChangeMarkdownWithCoverage(diff.BaseStats.TotalTokens, diff.BaseStats.TasksWithTokens, diff.HeadStats.TotalTokens, diff.HeadStats.TasksWithTokens))
		fmt.Printf("| MCP Schema | %s | %s | %s |\n",
			formatTokenCountOrNA(diff.BaseStats.McpSchemaTokens, diff.BaseStats.TasksWithTokens),
			formatTokenCountOrNA(diff.HeadStats.McpSchemaTokens, diff.HeadStats.TasksWithTokens),
			formatTokenChangeMarkdownWithCoverage(diff.BaseStats.McpSchemaTokens, diff.BaseStats.TasksWithTokens, diff.HeadStats.McpSchemaTokens, diff.HeadStats.TasksWithTokens))
		if diff.TokenDataIncomplete {
			fmt.Println("\n> ⚠️ Token counts may be incomplete due to errors during token estimation")
		}
	}

	// Regressions
	if len(diff.Regressions) > 0 {
		fmt.Println()
		fmt.Printf("#### ❌ Regressions (%d)\n", len(diff.Regressions))
		for _, r := range diff.Regressions {
			fmt.Printf("- `%s`: PASSED → FAILED", r.TaskName)
			if r.FailureReason != "" {
				fmt.Printf(" - %s", r.FailureReason)
			}
			fmt.Println()
		}
	}

	// Improvements
	if len(diff.Improvements) > 0 {
		fmt.Println()
		fmt.Printf("#### ✅ Improvements (%d)\n", len(diff.Improvements))
		for _, r := range diff.Improvements {
			fmt.Printf("- `%s`: FAILED → PASSED\n", r.TaskName)
		}
	}

	// New tasks
	if len(diff.New) > 0 {
		fmt.Println()
		fmt.Printf("#### 🆕 New Tasks (%d)\n", len(diff.New))
		for _, r := range diff.New {
			status := "PASSED"
			if !r.HeadPassed {
				status = "FAILED"
			}
			fmt.Printf("- `%s`: %s\n", r.TaskName, status)
		}
	}

	// Removed tasks
	if len(diff.Removed) > 0 {
		fmt.Println()
		fmt.Printf("#### 🗑️ Removed Tasks (%d)\n", len(diff.Removed))
		for _, r := range diff.Removed {
			fmt.Printf("- `%s`\n", r.TaskName)
		}
	}
}

func formatChangeMarkdown(change float64) string {
	if change > 0 {
		return fmt.Sprintf("🟢 +%.1f%%", change*100)
	} else if change < 0 {
		return fmt.Sprintf("🔴 %.1f%%", change*100)
	}
	return "➖ 0.0%"
}

func formatTokenChangeMarkdown(base, head int64) string {
	diff := head - base
	if base == 0 {
		if head > 0 {
			return fmt.Sprintf("🔴 +%s", formatTokenCount(head))
		}
		return "➖"
	}

	pctChange := float64(diff) / float64(base) * 100
	if diff > 0 {
		return fmt.Sprintf("🔴 +%s (+%.1f%%)", formatTokenCount(diff), pctChange)
	} else if diff < 0 {
		return fmt.Sprintf("🟢 %s (%.1f%%)", formatTokenCount(diff), pctChange)
	}
	return "➖ 0"
}

// formatTokenChangeMarkdownWithCoverage handles token change display when one or both sides may lack token data.
func formatTokenChangeMarkdownWithCoverage(baseTokens int64, baseTasksWithTokens int, headTokens int64, headTasksWithTokens int) string {
	// If neither has token data, nothing to compare
	if baseTasksWithTokens == 0 && headTasksWithTokens == 0 {
		return "➖"
	}

	// If only head has data, can't compare meaningfully
	if baseTasksWithTokens == 0 {
		return "(no base data)"
	}

	// If only base has data, can't compare meaningfully
	if headTasksWithTokens == 0 {
		return "(no head data)"
	}

	// Both have data, show normal comparison
	return formatTokenChangeMarkdown(baseTokens, headTokens)
}
