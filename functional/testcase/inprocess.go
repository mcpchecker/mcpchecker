package testcase

import (
	"bytes"
	"io"
	"os"
	"sync"

	"github.com/mcpchecker/mcpchecker/pkg/cli"
)

// EnvInProcessMode is the environment variable to enable in-process execution mode.
// When set to "true", functional tests will execute the mcpchecker CLI in-process
// instead of spawning a subprocess. This enables code coverage collection and
// IDE debugging with breakpoints.
const EnvInProcessMode = "MCPCHECKER_TEST_INPROCESS"

// inProcessMutex ensures only one in-process test runs at a time.
// This is necessary because in-process mode modifies global state
// (os.Args, os.Stdout, os.Stderr, working directory, environment variables).
var inProcessMutex sync.Mutex

// InProcessResult holds the result of an in-process CLI execution.
type InProcessResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// InProcessExecutor executes the mcpchecker CLI in-process.
// It captures stdout/stderr, manages working directory, and handles
// environment variables while preserving and restoring the original state.
type InProcessExecutor struct {
	Args    []string          // Command line arguments (excluding program name)
	Dir     string            // Working directory for execution
	Env     map[string]string // Environment variables to set
	BaseEnv []string          // Base environment (os.Environ() if nil)
}

// Execute runs the mcpchecker CLI in-process and returns the result.
// Thread-safe: uses a mutex to ensure only one in-process execution at a time.
func (e *InProcessExecutor) Execute() *InProcessResult {
	inProcessMutex.Lock()
	defer inProcessMutex.Unlock()

	result := &InProcessResult{}

	// Save original state
	origArgs := os.Args
	origDir, origDirErr := os.Getwd()
	origStdout := os.Stdout
	origStderr := os.Stderr
	origEnv := captureEnv()

	if origDirErr != nil {
		result.Err = origDirErr
		result.ExitCode = 1
		return result
	}

	// Restore original state when done
	defer func() {
		os.Args = origArgs
		_ = os.Chdir(origDir)
		os.Stdout = origStdout
		os.Stderr = origStderr
		restoreEnv(origEnv)
	}()

	// Set up os.Args for Cobra
	os.Args = append([]string{"mcpchecker"}, e.Args...)

	// Change working directory if specified
	if e.Dir != "" {
		if err := os.Chdir(e.Dir); err != nil {
			result.Err = err
			result.ExitCode = 1
			return result
		}
	}

	// Set environment variables
	baseEnv := e.BaseEnv
	if baseEnv == nil {
		baseEnv = os.Environ()
	}
	setEnv(baseEnv, e.Env)

	// Capture stdout
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		result.Err = err
		result.ExitCode = 1
		return result
	}

	// Capture stderr
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		result.Err = err
		result.ExitCode = 1
		return result
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW

	// Channel to collect captured output
	stdoutCh := make(chan string)
	stderrCh := make(chan string)

	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, stdoutR)
		stdoutCh <- buf.String()
	}()

	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, stderrR)
		stderrCh <- buf.String()
	}()

	// Execute the CLI
	result.Err = cli.Execute()

	// Close write ends to signal EOF to readers
	_ = stdoutW.Close()
	_ = stderrW.Close()

	// Collect captured output
	result.Stdout = <-stdoutCh
	result.Stderr = <-stderrCh

	// Close read ends
	_ = stdoutR.Close()
	_ = stderrR.Close()

	// Determine exit code
	if result.Err != nil {
		result.ExitCode = 1
	} else {
		result.ExitCode = 0
	}

	return result
}

// captureEnv returns a map of all current environment variables.
func captureEnv() map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		for i := 0; i < len(e); i++ {
			if e[i] == '=' {
				env[e[:i]] = e[i+1:]
				break
			}
		}
	}
	return env
}

// setEnv clears the environment and sets variables from base and additional.
func setEnv(base []string, additional map[string]string) {
	os.Clearenv()

	// Set base environment
	for _, e := range base {
		for i := 0; i < len(e); i++ {
			if e[i] == '=' {
				_ = os.Setenv(e[:i], e[i+1:])
				break
			}
		}
	}

	// Set additional environment variables
	for k, v := range additional {
		_ = os.Setenv(k, v)
	}
}

// restoreEnv restores the environment to the given state.
func restoreEnv(env map[string]string) {
	os.Clearenv()
	for k, v := range env {
		_ = os.Setenv(k, v)
	}
}
