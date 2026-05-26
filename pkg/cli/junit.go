package cli

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/mcpchecker/mcpchecker/pkg/eval"
	"github.com/mcpchecker/mcpchecker/pkg/results"
	"github.com/mcpchecker/mcpchecker/pkg/task"
	"github.com/spf13/cobra"
)

// JUnit XML types

type junitTestSuites struct {
	XMLName    xml.Name         `xml:"testsuites"`
	TestSuites []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Errors   int             `xml:"errors,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Error     *junitError   `xml:"error,omitempty"`
	SystemOut string        `xml:"system-out,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

type junitError struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

// NewJUnitCmd creates the junit command for converting eval results to JUnit XML.
func NewJUnitCmd() *cobra.Command {
	var (
		taskFilter string
		outputFile string
	)

	cmd := &cobra.Command{
		Use:   "junit <results-file>",
		Short: "Convert evaluation results to JUnit XML",
		Long: `Convert the JSON output produced by "mcpchecker check" into JUnit XML,
suitable for CI/CD tools like Jenkins and GitLab.

Example:
  mcpchecker result junit results.json
  mcpchecker result junit --output-file junit-report.xml results.json`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			evalResults, err := results.Load(args[0])
			if err != nil {
				return err
			}

			filtered := results.Filter(evalResults, taskFilter)
			if len(filtered) == 0 {
				if taskFilter == "" {
					return errors.New("no tasks found in results")
				}
				return fmt.Errorf("no tasks matched filter %q", taskFilter)
			}

			opts := viewOptions{
				showTimeline:   true,
				maxEvents:      defaultMaxEvents,
				maxOutputLines: defaultMaxOutputLines,
				maxLineLength:  defaultMaxLineLength,
			}
			data, err := convertJUnit(filtered, opts)
			if err != nil {
				return err
			}

			if outputFile != "" {
				if err := os.WriteFile(outputFile, data, 0o644); err != nil {
					return fmt.Errorf("failed to write output file %q: %w", outputFile, err)
				}
			} else {
				_, err = cmd.OutOrStdout().Write(data)
				return err
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&taskFilter, "task", "", "Only convert results for tasks whose name contains this value")
	cmd.Flags().StringVar(&outputFile, "output-file", "", "Write output to a file instead of stdout")
	return cmd
}

// convertJUnit converts eval results to JUnit XML format.
func convertJUnit(evalResults []*eval.EvalResult, opts viewOptions) ([]byte, error) {
	suite := buildJUnitSuite(evalResults, opts)
	suites := junitTestSuites{TestSuites: []junitTestSuite{suite}}

	output, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JUnit XML: %w", err)
	}
	return append([]byte(xml.Header), output...), nil
}

// buildJUnitSuite converts eval results into a single JUnit test suite.
func buildJUnitSuite(evalResults []*eval.EvalResult, opts viewOptions) junitTestSuite {
	suite := junitTestSuite{
		Name:  "mcpchecker",
		Tests: len(evalResults),
		Cases: make([]junitTestCase, 0, len(evalResults)),
	}

	for _, result := range evalResults {
		tc := junitTestCase{
			Name:      result.TaskName,
			Classname: extractJUnitClassname(result.TaskPath),
			SystemOut: renderEvalResult(result, opts),
		}

		switch {
		case !result.TaskPassed:
			// Execution error (agent error, timeout, verification failure)
			suite.Errors++
			msg := result.TaskError
			errType := "ExecutionError"
			if result.AgentExecutionError {
				errType = "AgentExecutionError"
			} else if result.TimedOut {
				errType = "Timeout"
			}
			if msg == "" {
				msg = errType
			}
			tc.Error = &junitError{
				Message: sanitizeXMLString(truncateString(msg, 200)),
				Type:    errType,
				Body:    sanitizeXMLString(msg),
			}

		case result.TaskPassed && !result.AllAssertionsPassed:
			// Assertion failure
			suite.Failures++
			var failureDetails []string
			if result.AssertionResults != nil {
				failureDetails = results.CollectFailedAssertions(result.AssertionResults)
			}
			msg := strings.Join(failureDetails, "; ")
			if msg == "" {
				msg = "Assertions failed"
			}
			tc.Failure = &junitFailure{
				Message: sanitizeXMLString(truncateString(msg, 200)),
				Type:    "AssertionFailure",
				Body:    sanitizeXMLString(strings.Join(failureDetails, "\n")),
			}
		}

		suite.Cases = append(suite.Cases, tc)
	}

	return suite
}

// renderEvalResult renders an eval result using the same format as "result view".
func renderEvalResult(result *eval.EvalResult, opts viewOptions) string {
	var buf strings.Builder
	prev := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = prev }()
	printEvalResult(&buf, result, opts)

	// Also include output from scripts such as setup, verify and cleanup which may contain useful information about failures.
	// This is not included in the default "result view" output but can be helpful in CI contexts.
	containsOutput := func(phase *task.PhaseOutput) bool { return phase != nil && len(phase.Steps) > 0 }
	if containsOutput(result.SetupOutput) || containsOutput(result.VerifyOutput) || containsOutput(result.CleanupOutput) {
		fmt.Fprintf(&buf, "Steps output:\n")
		printPhaseOutput(&buf, "Setup", result.SetupOutput)
		printPhaseOutput(&buf, "Verify", result.VerifyOutput)
		printPhaseOutput(&buf, "Cleanup", result.CleanupOutput)
	}

	return sanitizeXMLString(buf.String())
}

// printPhaseOutput writes step outputs from a single task phase to w.
func printPhaseOutput(w io.Writer, label string, phase *task.PhaseOutput) {
	if phase == nil || len(phase.Steps) == 0 {
		return
	}
	for _, step := range phase.Steps {
		if step == nil {
			continue
		}
		if step.Message != "" {
			fmt.Fprintf(w, "--- %s (stdout) ---\n", label)
			fmt.Fprintf(w, "%s\n", indentBlock(step.Message, "  "))
		}
		if step.Error != "" {
			fmt.Fprintf(w, "--- %s (stderr) ---\n", label)
			fmt.Fprintf(w, "%s\n", indentBlock(step.Error, "  "))
		}
	}
}

// sanitizeXMLString removes characters that are invalid in XML 1.0 from the input string.
func sanitizeXMLString(s string) string {
	isXmlChar := func(r rune) bool {
		return r == 0x09 || r == 0x0A || r == 0x0D ||
			(r >= 0x20 && r <= 0xD7FF) ||
			(r >= 0xE000 && r <= 0xFFFD) ||
			(r >= 0x10000 && r <= 0x10FFFF)
	}
	return strings.Map(func(r rune) rune {
		if isXmlChar(r) {
			return r
		}
		return -1 // Drop the character
	}, s)
}

// extractJUnitClassname derives a classname from the task file path.
func extractJUnitClassname(taskPath string) string {
	if taskPath == "" {
		return ""
	}
	// Remove extension and use the path components
	base := strings.TrimSuffix(taskPath, filepath.Ext(taskPath))
	return filepath.Base(base)
}
