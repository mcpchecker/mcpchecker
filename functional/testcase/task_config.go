package testcase

import (
	"encoding/json"
	"strings"

	"github.com/mcpchecker/mcpchecker/pkg/llmjudge"
	"github.com/mcpchecker/mcpchecker/pkg/steps"
	"github.com/mcpchecker/mcpchecker/pkg/task"
	"github.com/mcpchecker/mcpchecker/pkg/util"
)

// shellEscapeSingleQuote escapes a string for use within single quotes in a shell command.
// It replaces each ' with '\” (end single quote, escaped single quote, start single quote).
func shellEscapeSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

// TaskConfig provides a fluent API for building task configurations using the
// legacy v1alpha1 format. This ensures backwards compatibility testing.
// For the new step-based format, use TaskConfigV2.
type TaskConfig struct {
	metadata task.TaskMetadata
	steps    *task.TaskStepsV1Alpha1
}

// NewTaskConfig creates a new task config builder using the legacy format
func NewTaskConfig() *TaskConfig {
	return &TaskConfig{
		steps: &task.TaskStepsV1Alpha1{
			VerifyScript: &task.VerifyStep{},
		},
	}
}

// Name sets the task name
func (tc *TaskConfig) Name(name string) *TaskConfig {
	tc.metadata.Name = name
	return tc
}

// Difficulty sets the task difficulty level
func (tc *TaskConfig) Difficulty(difficulty string) *TaskConfig {
	tc.metadata.Difficulty = difficulty
	return tc
}

// Easy sets the difficulty to "easy"
func (tc *TaskConfig) Easy() *TaskConfig {
	return tc.Difficulty(task.DifficultyEasy)
}

// Medium sets the difficulty to "medium"
func (tc *TaskConfig) Medium() *TaskConfig {
	return tc.Difficulty(task.DifficultyMedium)
}

// Hard sets the difficulty to "hard"
func (tc *TaskConfig) Hard() *TaskConfig {
	return tc.Difficulty(task.DifficultyHard)
}

// Labels sets the task labels
func (tc *TaskConfig) Labels(labels map[string]string) *TaskConfig {
	tc.metadata.Labels = labels
	return tc
}

// AddLabel adds a single label to the task
func (tc *TaskConfig) AddLabel(key, value string) *TaskConfig {
	if tc.metadata.Labels == nil {
		tc.metadata.Labels = make(map[string]string)
	}
	tc.metadata.Labels[key] = value
	return tc
}

// Parallel marks the task as safe to run in parallel with other parallel tasks
func (tc *TaskConfig) Parallel() *TaskConfig {
	tc.metadata.Parallel = true
	return tc
}

// Prompt sets the prompt text for the agent.
// The prompt is shell-escaped for single quotes since the agent spec template
// uses single quotes around the prompt argument.
func (tc *TaskConfig) Prompt(prompt string) *TaskConfig {
	tc.steps.Prompt = &util.Step{
		Inline: shellEscapeSingleQuote(prompt),
	}
	return tc
}

// PromptFile sets the prompt to be read from a file
func (tc *TaskConfig) PromptFile(path string) *TaskConfig {
	tc.steps.Prompt = &util.Step{
		File: path,
	}
	return tc
}

// SetupScript sets an inline setup script to run before the task
func (tc *TaskConfig) SetupScript(script string) *TaskConfig {
	tc.steps.SetupScript = &util.Step{
		Inline: script,
	}
	return tc
}

// SetupScriptFile sets the setup script to be read from a file
func (tc *TaskConfig) SetupScriptFile(path string) *TaskConfig {
	tc.steps.SetupScript = &util.Step{
		File: path,
	}
	return tc
}

// CleanupScript sets an inline cleanup script to run after the task
func (tc *TaskConfig) CleanupScript(script string) *TaskConfig {
	tc.steps.CleanupScript = &util.Step{
		Inline: script,
	}
	return tc
}

// CleanupScriptFile sets the cleanup script to be read from a file
func (tc *TaskConfig) CleanupScriptFile(path string) *TaskConfig {
	tc.steps.CleanupScript = &util.Step{
		File: path,
	}
	return tc
}

// VerifyScript sets an inline verification script
func (tc *TaskConfig) VerifyScript(script string) *TaskConfig {
	tc.steps.VerifyScript = &task.VerifyStep{
		Step: &util.Step{
			Inline: script,
		},
	}
	return tc
}

// VerifyScriptFile sets the verification script to be read from a file
func (tc *TaskConfig) VerifyScriptFile(path string) *TaskConfig {
	tc.steps.VerifyScript = &task.VerifyStep{
		Step: &util.Step{
			File: path,
		},
	}
	return tc
}

// VerifyContains sets LLM judge verification with CONTAINS mode.
// The judge will check if the agent output contains the expected content.
func (tc *TaskConfig) VerifyContains(expected string) *TaskConfig {
	tc.steps.VerifyScript = &task.VerifyStep{
		LLMJudgeStepConfig: &llmjudge.LLMJudgeStepConfig{
			Contains: expected,
		},
	}
	return tc
}

// VerifyExact sets LLM judge verification with EXACT mode.
// The judge will check if the agent output exactly matches the expected content.
func (tc *TaskConfig) VerifyExact(expected string) *TaskConfig {
	tc.steps.VerifyScript = &task.VerifyStep{
		LLMJudgeStepConfig: &llmjudge.LLMJudgeStepConfig{
			Exact: expected,
		},
	}
	return tc
}

// Metadata returns the task metadata
func (tc *TaskConfig) Metadata() task.TaskMetadata {
	return tc.metadata
}

// Steps returns the legacy steps
func (tc *TaskConfig) Steps() *task.TaskStepsV1Alpha1 {
	return tc.steps
}

// TaskConfigV2 provides a fluent API for building task configurations using
// the new step-based format with typed step arrays.
type TaskConfigV2 struct {
	metadata task.TaskMetadata
	setup    []*steps.StepConfig
	cleanup  []*steps.StepConfig
	verify   []*steps.StepConfig
	prompt   *util.Step
}

// NewTaskConfigV2 creates a new task config builder using the new step-based format
func NewTaskConfigV2() *TaskConfigV2 {
	return &TaskConfigV2{
		setup:   []*steps.StepConfig{},
		cleanup: []*steps.StepConfig{},
		verify:  []*steps.StepConfig{},
	}
}

// Name sets the task name
func (tc *TaskConfigV2) Name(name string) *TaskConfigV2 {
	tc.metadata.Name = name
	return tc
}

// Difficulty sets the task difficulty level
func (tc *TaskConfigV2) Difficulty(difficulty string) *TaskConfigV2 {
	tc.metadata.Difficulty = difficulty
	return tc
}

// Easy sets the difficulty to "easy"
func (tc *TaskConfigV2) Easy() *TaskConfigV2 {
	return tc.Difficulty(task.DifficultyEasy)
}

// Medium sets the difficulty to "medium"
func (tc *TaskConfigV2) Medium() *TaskConfigV2 {
	return tc.Difficulty(task.DifficultyMedium)
}

// Hard sets the difficulty to "hard"
func (tc *TaskConfigV2) Hard() *TaskConfigV2 {
	return tc.Difficulty(task.DifficultyHard)
}

// Labels sets the task labels
func (tc *TaskConfigV2) Labels(labels map[string]string) *TaskConfigV2 {
	tc.metadata.Labels = labels
	return tc
}

// AddLabel adds a single label to the task
func (tc *TaskConfigV2) AddLabel(key, value string) *TaskConfigV2 {
	if tc.metadata.Labels == nil {
		tc.metadata.Labels = make(map[string]string)
	}
	tc.metadata.Labels[key] = value
	return tc
}

// Parallel marks the task as safe to run in parallel with other parallel tasks
func (tc *TaskConfigV2) Parallel() *TaskConfigV2 {
	tc.metadata.Parallel = true
	return tc
}

// Prompt sets the prompt text for the agent.
func (tc *TaskConfigV2) Prompt(prompt string) *TaskConfigV2 {
	tc.prompt = &util.Step{
		Inline: shellEscapeSingleQuote(prompt),
	}
	return tc
}

// PromptFile sets the prompt to be read from a file
func (tc *TaskConfigV2) PromptFile(path string) *TaskConfigV2 {
	tc.prompt = &util.Step{
		File: path,
	}
	return tc
}

// AddSetupScript adds an inline script step to the setup phase
func (tc *TaskConfigV2) AddSetupScript(script string) *TaskConfigV2 {
	tc.setup = append(tc.setup, makeScriptStep(script, ""))
	return tc
}

// AddSetupScriptFile adds a file-based script step to the setup phase
func (tc *TaskConfigV2) AddSetupScriptFile(path string) *TaskConfigV2 {
	tc.setup = append(tc.setup, makeScriptStep("", path))
	return tc
}

// AddSetupHTTP adds an HTTP step to the setup phase
func (tc *TaskConfigV2) AddSetupHTTP(method, url string) *TaskConfigV2 {
	tc.setup = append(tc.setup, makeHTTPStep(method, url))
	return tc
}

// AddCleanupScript adds an inline script step to the cleanup phase
func (tc *TaskConfigV2) AddCleanupScript(script string) *TaskConfigV2 {
	tc.cleanup = append(tc.cleanup, makeScriptStep(script, ""))
	return tc
}

// AddCleanupScriptFile adds a file-based script step to the cleanup phase
func (tc *TaskConfigV2) AddCleanupScriptFile(path string) *TaskConfigV2 {
	tc.cleanup = append(tc.cleanup, makeScriptStep("", path))
	return tc
}

// AddCleanupHTTP adds an HTTP step to the cleanup phase
func (tc *TaskConfigV2) AddCleanupHTTP(method, url string) *TaskConfigV2 {
	tc.cleanup = append(tc.cleanup, makeHTTPStep(method, url))
	return tc
}

// AddVerifyScript adds an inline script step to the verify phase
func (tc *TaskConfigV2) AddVerifyScript(script string) *TaskConfigV2 {
	tc.verify = append(tc.verify, makeScriptStep(script, ""))
	return tc
}

// AddVerifyScriptFile adds a file-based script step to the verify phase
func (tc *TaskConfigV2) AddVerifyScriptFile(path string) *TaskConfigV2 {
	tc.verify = append(tc.verify, makeScriptStep("", path))
	return tc
}

// AddVerifyContains adds an LLM judge step with CONTAINS mode to the verify phase
func (tc *TaskConfigV2) AddVerifyContains(expected string) *TaskConfigV2 {
	tc.verify = append(tc.verify, makeLLMJudgeStep(expected, ""))
	return tc
}

// AddVerifyExact adds an LLM judge step with EXACT mode to the verify phase
func (tc *TaskConfigV2) AddVerifyExact(expected string) *TaskConfigV2 {
	tc.verify = append(tc.verify, makeLLMJudgeStep("", expected))
	return tc
}

// AddVerifyHTTP adds an HTTP step to the verify phase
func (tc *TaskConfigV2) AddVerifyHTTP(method, url string) *TaskConfigV2 {
	tc.verify = append(tc.verify, makeHTTPStep(method, url))
	return tc
}

// Metadata returns the task metadata
func (tc *TaskConfigV2) Metadata() task.TaskMetadata {
	return tc.metadata
}

// Setup returns the setup steps
func (tc *TaskConfigV2) Setup() []*steps.StepConfig {
	return tc.setup
}

// Cleanup returns the cleanup steps
func (tc *TaskConfigV2) Cleanup() []*steps.StepConfig {
	return tc.cleanup
}

// Verify returns the verify steps
func (tc *TaskConfigV2) Verify() []*steps.StepConfig {
	return tc.verify
}

// PromptStep returns the prompt step
func (tc *TaskConfigV2) PromptStep() *util.Step {
	return tc.prompt
}

// Build returns the task spec
func (tc *TaskConfigV2) Build() *task.TaskSpec {
	return &task.TaskSpec{
		Setup:   tc.setup,
		Cleanup: tc.cleanup,
		Verify:  tc.verify,
		Prompt:  tc.prompt,
	}
}

// --- Helper functions to create step configs ---

func makeScriptStep(inline, file string) *steps.StepConfig {
	cfg := map[string]any{}
	if inline != "" {
		cfg["inline"] = inline
	}
	if file != "" {
		cfg["file"] = file
	}
	raw, _ := json.Marshal(cfg)
	return &steps.StepConfig{Config: map[string]json.RawMessage{"script": raw}}
}

func makeLLMJudgeStep(contains, exact string) *steps.StepConfig {
	cfg := map[string]any{}
	if contains != "" {
		cfg["contains"] = contains
	}
	if exact != "" {
		cfg["exact"] = exact
	}
	raw, _ := json.Marshal(cfg)
	return &steps.StepConfig{Config: map[string]json.RawMessage{"llmJudge": raw}}
}

func makeHTTPStep(method, url string) *steps.StepConfig {
	cfg := map[string]any{
		"method": method,
		"url":    url,
	}
	raw, _ := json.Marshal(cfg)
	return &steps.StepConfig{Config: map[string]json.RawMessage{"http": raw}}
}

// Re-export types for convenience
type (
	TaskMetadata       = task.TaskMetadata
	TaskStepsV1Alpha1  = task.TaskStepsV1Alpha1
	VerifyStep         = task.VerifyStep
	LLMJudgeStepConfig = llmjudge.LLMJudgeStepConfig
)

// Re-export difficulty constants
const (
	DifficultyEasy   = task.DifficultyEasy
	DifficultyMedium = task.DifficultyMedium
	DifficultyHard   = task.DifficultyHard
)
