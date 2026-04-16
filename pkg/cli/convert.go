package cli

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpchecker/mcpchecker/pkg/eval"
	"github.com/mcpchecker/mcpchecker/pkg/results"
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

// NewConvertCmd creates the convert command for converting eval results to other formats.
func NewConvertCmd() *cobra.Command {
	var (
		taskFilter     string
		outputFormat   string
		outputFile     string
		maxEvents      = defaultMaxEvents
		maxOutputLines = defaultMaxOutputLines
		maxLineLength  = defaultMaxLineLength
	)

	cmd := &cobra.Command{
		Use:   "convert <results-file>",
		Short: "Convert evaluation results to other formats (e.g. JUnit XML)",
		Long: `Convert the JSON output produced by "mcpchecker check" into other formats.

Currently supports JUnit XML, suitable for CI/CD tools like Jenkins and GitLab.

Examples:
  mcpchecker result convert mcpchecker-eval-out.json
  mcpchecker result convert --output junit results.json
  mcpchecker result convert --task my-task results.json
  mcpchecker result convert --output-file report.xml results.json`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
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

			var w io.Writer = os.Stdout
			if outputFile != "" {
				f, err := os.Create(outputFile)
				if err != nil {
					return fmt.Errorf("failed to create output file: %w", err)
				}
				defer f.Close()
				w = f
			}

			opts := viewOptions{
				showTimeline:   true,
				maxEvents:      maxEvents,
				maxOutputLines: maxOutputLines,
				maxLineLength:  maxLineLength,
			}

			switch outputFormat {
			case "junit":
				return outputJUnit(w, filtered, opts)
			default:
				return fmt.Errorf("unsupported output format: %s", outputFormat)
			}
		},
	}

	cmd.Flags().StringVar(&taskFilter, "task", "", "Only convert results for tasks whose name contains this value")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "junit", "Output format (junit)")
	cmd.Flags().StringVar(&outputFile, "output-file", "", "Write output to a file instead of stdout")
	cmd.Flags().IntVar(&maxEvents, "max-events", maxEvents, "Maximum number of timeline entries to include (0 = unlimited)")
	cmd.Flags().IntVar(&maxOutputLines, "max-output-lines", maxOutputLines, "Maximum lines per command output in the timeline")
	cmd.Flags().IntVar(&maxLineLength, "max-line-length", maxLineLength, "Maximum characters per line when formatting output")

	return cmd
}

// outputJUnit writes JUnit XML to w.
func outputJUnit(w io.Writer, evalResults []*eval.EvalResult, opts viewOptions) error {
	suite := buildJUnitSuite(evalResults, opts)
	suites := junitTestSuites{TestSuites: []junitTestSuite{suite}}

	fmt.Fprint(w, xml.Header)
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	if err := encoder.Encode(suites); err != nil {
		return fmt.Errorf("failed to encode JUnit XML: %w", err)
	}
	fmt.Fprintln(w)
	return nil
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
			Classname: extractClassname(result.TaskPath),
			SystemOut: renderEvalResultToString(result, opts),
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
				Message: truncateString(msg, 200),
				Type:    errType,
				Body:    msg,
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
				Message: truncateString(msg, 200),
				Type:    "AssertionFailure",
				Body:    strings.Join(failureDetails, "\n"),
			}
		}

		suite.Cases = append(suite.Cases, tc)
	}

	return suite
}

// renderEvalResultToString renders an eval result using the same format as "result view".
func renderEvalResultToString(result *eval.EvalResult, opts viewOptions) string {
	var buf strings.Builder
	printEvalResult(&buf, result, opts)
	return buf.String()
}

// extractClassname derives a classname from the task file path.
func extractClassname(taskPath string) string {
	if taskPath == "" {
		return ""
	}
	// Remove extension and use the path components
	base := strings.TrimSuffix(taskPath, filepath.Ext(taskPath))
	return filepath.Base(base)
}
