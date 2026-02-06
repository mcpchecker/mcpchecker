package task

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mcpchecker/mcpchecker/pkg/llmjudge"
	"github.com/mcpchecker/mcpchecker/pkg/steps"
	"github.com/mcpchecker/mcpchecker/pkg/util"
	"sigs.k8s.io/yaml"
)

const (
	KindTask         = "Task"
	DifficultyEasy   = "easy"
	DifficultyMedium = "medium"
	DifficultyHard   = "hard"
)

type TaskConfig struct {
	util.TypeMeta `json:",inline"`
	Metadata      TaskMetadata `json:"metadata"`
	Spec          *TaskSpec    `json:"spec"`

	basePath string
}

type TaskMetadata struct {
	Name       string            `json:"name"`
	Difficulty string            `json:"difficulty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type TaskSpec struct {
	Requires []Requirements      `json:"requires,omitempty"`
	Setup    []*steps.StepConfig `json:"setup,omitempty"`
	Cleanup  []*steps.StepConfig `json:"cleanup,omitempty"`
	Verify   []*steps.StepConfig `json:"verify,omitempty"`
	Prompt   *util.Step          `json:"prompt,omitempty"`
}

type Requirements struct {
	Extension *string `json:"extension,omitempty"`
	McpServer *string `json:"mcpServer,omitempty"`
	As        *string `json:"as,omitempty"`
}

type TaskStepsV1Alpha1 struct {
	SetupScript   *util.Step  `json:"setup,omitempty"`
	CleanupScript *util.Step  `json:"cleanup,omitempty"`
	VerifyScript  *VerifyStep `json:"verify,omitempty"`
	Prompt        *util.Step  `json:"prompt,omitempty"`
}

type VerifyStep struct {
	*util.Step
	*llmjudge.LLMJudgeStepConfig
}

func (v *VerifyStep) IsEmpty() bool {
	if v == nil {
		return true
	}

	hasStep := v.Step != nil && !v.Step.IsEmpty()
	hasJudgeConfig := v.LLMJudgeStepConfig != nil

	return !hasStep && !hasJudgeConfig
}

func (v *VerifyStep) Validate() error {
	if v == nil {
		return fmt.Errorf("verify step is nil")
	}

	hasStep := v.Step != nil && !v.Step.IsEmpty()
	hasJudgeConfig := v.LLMJudgeStepConfig != nil

	// Must have exactly one verification method
	if !hasStep && !hasJudgeConfig {
		return fmt.Errorf("verify.inline, verify.file, verify.exact, or verify.contains must be set")
	}

	if hasStep && hasJudgeConfig {
		return fmt.Errorf("cannot specify both a verify script (inline/file) and llm judge config (exact/contains)")
	}

	// Validate LLM judge config if present
	if hasJudgeConfig {
		if err := v.LLMJudgeStepConfig.Validate(); err != nil {
			return fmt.Errorf("invalid llm judge config: %w", err)
		}
	}

	return nil
}

func Read(data []byte, basePath string) (*TaskConfig, error) {
	type Wrapper struct {
		*TaskConfig `json:",inline"`
		Steps       *TaskStepsV1Alpha1 `json:"steps,omitempty"`
	}

	spec := &TaskConfig{}
	wrapper := &Wrapper{TaskConfig: spec}

	err := yaml.Unmarshal(data, wrapper)
	if err != nil {
		return nil, err
	}

	if err := wrapper.TypeMeta.Validate(KindTask); err != nil {
		return nil, err
	}

	spec.basePath = basePath

	if wrapper.GetAPIVersion() == util.APIVersionV1Alpha1 {
		if wrapper.Steps == nil {
			return nil, fmt.Errorf("v1alpha1 requires steps field")
		}

		if err := resolveStepPath(wrapper.Steps.SetupScript, basePath); err != nil {
			return nil, fmt.Errorf("failed to resolve setup script path: %w", err)
		}
		if err := resolveStepPath(wrapper.Steps.CleanupScript, basePath); err != nil {
			return nil, fmt.Errorf("failed to resolve cleanup script path: %w", err)
		}
		if err := resolveStepPath(wrapper.Steps.VerifyScript.Step, basePath); err != nil {
			return nil, fmt.Errorf("failed to resolve verify script path: %w", err)
		}

		spec.Spec, err = translateV1Alpha1ToSteps(wrapper.Steps)
		if err != nil {
			return nil, fmt.Errorf("failed to convert v1alpha1 format to v1alpha1: %w", err)
		}
	}

	if err := resolveStepPath(spec.Spec.Prompt, basePath); err != nil {
		return nil, fmt.Errorf("failed to resolve prompt path: %w", err)
	}

	return spec, nil
}

func resolveStepPath(step *util.Step, basePath string) error {
	if step == nil || step.File == "" {
		return nil
	}

	// If the path is already absolute, leave it as-is
	if filepath.IsAbs(step.File) {
		return nil
	}

	// Convert relative path to absolute path based on the YAML file's directory
	absPath := filepath.Join(basePath, step.File)
	step.File = absPath

	return nil
}

func FromFile(path string) (*TaskConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s' for taskspec: %w", path, err)
	}

	// Convert to absolute path to ensure basePath is absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for '%s': %w", path, err)
	}

	basePath := filepath.Dir(absPath)

	return Read(data, basePath)
}
