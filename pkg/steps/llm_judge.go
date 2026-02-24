package steps

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/genmcp/gen-mcp/pkg/template"
	"github.com/mcpchecker/mcpchecker/pkg/llmjudge"
	"github.com/mcpchecker/mcpchecker/pkg/util"
)

// LLMJudgeStep validates agent outputs using an LLM judge.
type LLMJudgeStep struct {
	cfg              *llmjudge.LLMJudgeStepConfig
	containsTemplate *template.TemplateBuilder
	exactTemplate    *template.TemplateBuilder
}

var _ StepRunner = &LLMJudgeStep{}

// ParseLLMJudgeStep parses an LLM judge step from JSON configuration.
func ParseLLMJudgeStep(raw json.RawMessage) (StepRunner, error) {
	cfg := &llmjudge.LLMJudgeStepConfig{}

	err := json.Unmarshal(raw, cfg)
	if err != nil {
		return nil, err
	}

	return NewLLMJudgeStep(cfg)
}

// NewLLMJudgeStep creates a new LLM judge step with template support.
func NewLLMJudgeStep(cfg *llmjudge.LLMJudgeStepConfig) (*LLMJudgeStep, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	step := &LLMJudgeStep{cfg: cfg}

	// Register source factories for template parsing
	sources := map[string]template.SourceFactory{
		"steps":  template.NewSourceFactory("steps"),
		"random": template.NewSourceFactory("random"),
	}

	// Parse Contains field as template if present
	if cfg.Contains != "" {
		containsTemplate, err := template.ParseTemplate(cfg.Contains, template.TemplateParserOptions{
			Sources: sources,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to parse contains template: %w", err)
		}

		step.containsTemplate, err = template.NewTemplateBuilder(containsTemplate, false)
		if err != nil {
			return nil, fmt.Errorf("failed to create template builder for contains: %w", err)
		}
	}

	// Parse Exact field as template if present
	if cfg.Exact != "" {
		exactTemplate, err := template.ParseTemplate(cfg.Exact, template.TemplateParserOptions{
			Sources: sources,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to parse exact template: %w", err)
		}

		step.exactTemplate, err = template.NewTemplateBuilder(exactTemplate, false)
		if err != nil {
			return nil, fmt.Errorf("failed to create template builder for exact: %w", err)
		}
	}

	return step, nil
}

// Execute runs the LLM judge step with template expansion for step outputs.
func (s *LLMJudgeStep) Execute(ctx context.Context, input *StepInput) (*StepOutput, error) {
	judge, ok := llmjudge.FromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("no llm judge configured for llmJudge step")
	}

	if input.Agent == nil || input.Agent.Prompt == "" || input.Agent.Output == "" {
		return nil, fmt.Errorf("cannot run llmJudge step before agent (must be in verification)")
	}

	// Register step outputs as a template source
	// Always register if templates exist to ensure consistent error handling
	// and prevent resolver state leakage between executions
	stepOutputs := input.StepOutputs
	if stepOutputs == nil {
		stepOutputs = make(map[string]map[string]string)
	}

	resolver := NewStepOutputResolver(stepOutputs)
	if s.containsTemplate != nil {
		s.containsTemplate.SetSourceResolver("steps", resolver)
		if input.Random != nil {
			s.containsTemplate.SetSourceResolver("random", input.Random)
		}
	}
	if s.exactTemplate != nil {
		s.exactTemplate.SetSourceResolver("steps", resolver)
		if input.Random != nil {
			s.exactTemplate.SetSourceResolver("random", input.Random)
		}
	}

	// Resolve templates to get final values
	// Clone the config to preserve all fields (model, temperature, rubric, etc.)
	expandedCfg := *s.cfg

	if s.containsTemplate != nil {
		result, err := s.containsTemplate.GetResult()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve contains template: %w", err)
		}
		str, ok := result.(string)
		if !ok {
			return nil, fmt.Errorf("contains template resolved to non-string type: %T", result)
		}
		expandedCfg.Contains = str
	}

	if s.exactTemplate != nil {
		result, err := s.exactTemplate.GetResult()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve exact template: %w", err)
		}
		str, ok := result.(string)
		if !ok {
			return nil, fmt.Errorf("exact template resolved to non-string type: %T", result)
		}
		expandedCfg.Exact = str
	}

	if util.IsVerbose(ctx) {
		fmt.Printf("  → LLM judge '%s' is evaluating…\n", judge.ModelName())
		if expandedCfg.Contains != s.cfg.Contains || expandedCfg.Exact != s.cfg.Exact {
			fmt.Printf("  → Template expansion: %s -> %s\n", s.cfg.ReferenceAnswer(), expandedCfg.ReferenceAnswer())
		}
	}

	res, err := judge.EvaluateText(ctx, &expandedCfg, input.Agent.Prompt, input.Agent.Output)
	if err != nil {
		return nil, fmt.Errorf("failed to call llm judge: %w", err)
	}

	out := &StepOutput{
		Type:    "llmJudge",
		Success: res.Passed,
		Message: res.Reason,
	}

	if !res.Passed {
		out.Error = fmt.Sprintf("llm judge failed for reason '%s': %s", res.FailureCategory, res.Reason)
	}

	return out, nil
}

// StepOutputResolver resolves template variables from step outputs.
// It implements the template.SourceResolver interface.
type StepOutputResolver struct {
	outputs map[string]map[string]string
}

// NewStepOutputResolver creates a new resolver for step outputs.
func NewStepOutputResolver(outputs map[string]map[string]string) *StepOutputResolver {
	return &StepOutputResolver{outputs: outputs}
}

// Resolve looks up a field in the step outputs.
// Field names use the format "stepType.outputKey" where stepType can contain dots.
// Example: "kubernetes.listContexts.current" looks up outputs["kubernetes.listContexts"]["current"]
func (r *StepOutputResolver) Resolve(fieldName string) (string, error) {
	// Split on the last dot to separate stepType from outputKey
	lastDot := -1
	for i := len(fieldName) - 1; i >= 0; i-- {
		if fieldName[i] == '.' {
			lastDot = i
			break
		}
	}

	if lastDot == -1 {
		return "", fmt.Errorf("invalid field name %q: must be in format stepType.outputKey", fieldName)
	}

	stepType := fieldName[:lastDot]  // e.g., "kubernetes.listContexts"
	outputKey := fieldName[lastDot+1:] // e.g., "current"

	stepOutputs, ok := r.outputs[stepType]
	if !ok {
		return "", fmt.Errorf("step type %q not found in outputs", stepType)
	}

	value, ok := stepOutputs[outputKey]
	if !ok {
		return "", fmt.Errorf("output key %q not found for step type %q", outputKey, stepType)
	}

	return value, nil
}
