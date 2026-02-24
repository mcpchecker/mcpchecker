package task

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/genmcp/gen-mcp/pkg/template"
	"github.com/mcpchecker/mcpchecker/pkg/agent"
	"github.com/mcpchecker/mcpchecker/pkg/extension/client"
	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	"github.com/mcpchecker/mcpchecker/pkg/steps"
)

// AgentDetails captures structured information from the agent execution.
type AgentDetails struct {
	TokenEstimate *agent.TokenEstimate    `json:"tokenEstimate,omitempty"`
	ToolCalls     []agent.ToolCallSummary `json:"toolCalls,omitempty"`
	FinalMessage  string                  `json:"finalMessage,omitempty"`
	Thinking      string                  `json:"thinking,omitempty"`
}

// PhaseOutput represents the output from a task phase (setup, agent, verify, or cleanup).
// It contains both the individual step outputs and the overall phase result.
type PhaseOutput struct {
	// Steps contains the ordered outputs from each step executed in this phase.
	// For the agent phase, this will contain a single synthetic step output.
	Steps []*steps.StepOutput

	// Success indicates whether the phase completed successfully.
	Success bool

	// Error contains the error message if the phase failed.
	Error string

	// AgentDetails contains structured information from agent execution.
	// Only populated for the agent phase.
	AgentDetails *AgentDetails `json:"agentDetails,omitempty"`
}

type TaskRunner interface {
	Setup(ctx context.Context) (*PhaseOutput, error)
	Cleanup(ctx context.Context) (*PhaseOutput, error)
	RunAgent(ctx context.Context, agent agent.Runner) (*PhaseOutput, error)
	Verify(ctx context.Context) (*PhaseOutput, error)
}

type taskRunner struct {
	setup   []steps.StepRunner
	verify  []steps.StepRunner
	cleanup []steps.StepRunner
	prompt  string
	output  string
	baseDir string

	setupOutputs map[string]map[string]string
	random       *steps.RandomResolver
}

func NewTaskRunner(ctx context.Context, cfg *TaskConfig) (TaskRunner, error) {
	if cfg.Spec.Prompt.IsEmpty() {
		return nil, fmt.Errorf("prompt.inline or prompt.file must be set on a task to run it")
	}

	var err error
	r := &taskRunner{
		setup:   make([]steps.StepRunner, len(cfg.Spec.Setup)),
		verify:  make([]steps.StepRunner, len(cfg.Spec.Verify)),
		cleanup: make([]steps.StepRunner, len(cfg.Spec.Cleanup)),
		baseDir: cfg.basePath,
		random:  steps.NewRandomResolver(),
	}

	extensionManager, ok := client.ManagerFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("failed to get extension manager from context")
	}

	mcpClientManager, ok := mcpclient.ManagerFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("failed to get mcpclient manager from context")
	}

	extensions := make(map[string]string)
	mcpServers := make(map[string]string)
	for i, req := range cfg.Spec.Requires {
		if req.McpServer != nil && req.Extension != nil {
			return nil, fmt.Errorf("task.spec.requires[%d] is invalid: must have only one of mcpserver or extension defined, has both", i)
		}
		var alias string
		if req.As != nil {
			alias = *req.As
		}

		if req.Extension != nil {
			if !extensionManager.Has(*req.Extension) {
				return nil, fmt.Errorf("required extension %q not registered", *req.Extension)
			}

			if alias == "" {
				alias = *req.Extension
			}

			if _, ok := extensions[alias]; ok {
				return nil, fmt.Errorf("duplicate alias %q in requirements", alias)
			}

			if _, ok := mcpServers[alias]; ok {
				return nil, fmt.Errorf("duplicate alias %q in requirements", alias)
			}

			if strings.Contains(alias, ".") {
				return nil, fmt.Errorf("alias %q cannot contain dots", alias)
			}

			extensions[alias] = *req.Extension
		}

		if req.McpServer != nil {
			if _, ok := mcpClientManager.Get(*req.McpServer); !ok {
				return nil, fmt.Errorf("required mcpServer %q not registered", *req.McpServer)
			}

			if alias == "" {
				alias = *req.McpServer
			}

			if _, ok := extensions[alias]; ok {
				return nil, fmt.Errorf("duplicate alias %q in requirements", alias)
			}

			if _, ok := mcpServers[alias]; ok {
				return nil, fmt.Errorf("duplicate alias %q in requirements", alias)
			}

			if strings.Contains(alias, ".") {
				return nil, fmt.Errorf("alias %q cannot contain dots", alias)
			}

			mcpServers[alias] = *req.McpServer
		}
	}

	parser := steps.DefaultRegistry.WithExtensions(ctx, extensions).WithMcpServers(ctx, mcpServers)

	for i, stepCfg := range cfg.Spec.Setup {
		if stepCfg.ID == "" {
			stepCfg.ID = fmt.Sprintf("setup_%d", i)
		}
		var stepErr error
		r.setup[i], stepErr = parser.Parse(stepCfg)
		if stepErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to parse setup[%d]: %w", i, stepErr))
		}
	}

	for i, stepCfg := range cfg.Spec.Verify {
		if stepCfg.ID == "" {
			stepCfg.ID = fmt.Sprintf("verify_%d", i)
		}
		var stepErr error
		r.verify[i], stepErr = parser.Parse(stepCfg)
		if stepErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to parse verify[%d]: %w", i, stepErr))
		}
	}

	for i, stepCfg := range cfg.Spec.Cleanup {
		if stepCfg.ID == "" {
			stepCfg.ID = fmt.Sprintf("cleanup_%d", i)
		}
		var stepErr error
		r.cleanup[i], stepErr = parser.Parse(stepCfg)
		if stepErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to parse cleanup[%d]: %w", i, stepErr))
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse task steps: %w", err)
	}

	r.prompt, err = cfg.Spec.Prompt.GetValue()
	if err != nil {
		return nil, fmt.Errorf("failed to get prompt for task: %w", err)
	}

	return r, nil
}

func (r *taskRunner) Setup(ctx context.Context) (*PhaseOutput, error) {
	out := &PhaseOutput{
		Steps:   make([]*steps.StepOutput, 0),
		Success: true,
	}

	stepOutputs := make(map[string]map[string]string)

	for i, s := range r.setup {
		res, err := s.Execute(ctx, &steps.StepInput{
			Workdir:     r.baseDir,
			StepOutputs: stepOutputs,
			Random:      r.random,
		})

		out.Steps = append(out.Steps, res)
		if err != nil {
			out.Success = false
			out.Error = err.Error()
			return out, fmt.Errorf("setup[%d] failed: %w", i, err)
		}
		if res != nil && !res.Success {
			out.Success = false
		}

		// Accumulate outputs from this step
		if res != nil && res.Success && len(res.Outputs) > 0 && res.Type != "" {
			stepOutputs[res.Type] = res.Outputs
		}
	}

	r.setupOutputs = stepOutputs

	return out, nil
}

func (r *taskRunner) Cleanup(ctx context.Context) (*PhaseOutput, error) {
	out := &PhaseOutput{
		Steps:   make([]*steps.StepOutput, 0),
		Success: true,
	}

	// Seed cleanup step outputs with setup outputs so cleanup steps
	// can reference values produced during setup (e.g. generated namespace names).
	stepOutputs := make(map[string]map[string]string)
	for k, v := range r.setupOutputs {
		stepOutputs[k] = v
	}

	for i, s := range r.cleanup {
		res, err := s.Execute(ctx, &steps.StepInput{
			Workdir:     r.baseDir,
			StepOutputs: stepOutputs,
			Random:      r.random,
		})

		out.Steps = append(out.Steps, res)
		if err != nil {
			out.Success = false
			out.Error = err.Error()
			return out, fmt.Errorf("cleanup[%d] failed: %w", i, err)
		}
		if res != nil && !res.Success {
			out.Success = false
		}

		// Accumulate outputs from this step
		if res != nil && res.Success && len(res.Outputs) > 0 && res.Type != "" {
			stepOutputs[res.Type] = res.Outputs
		}
	}

	return out, nil
}

// resolvePromptTemplates resolves {steps.*} template variables in the prompt
// using outputs collected during setup. Returns the original prompt if no
// templates are present or if resolution fails.
func (r *taskRunner) resolvePromptTemplates(prompt string) string {
	if len(r.setupOutputs) == 0 || !strings.Contains(prompt, "{steps.") {
		return prompt
	}

	sources := map[string]template.SourceFactory{
		"steps": template.NewSourceFactory("steps"),
	}

	parsed, err := template.ParseTemplate(prompt, template.TemplateParserOptions{
		Sources: sources,
	})
	if err != nil {
		return prompt
	}

	builder, err := template.NewTemplateBuilder(parsed, false)
	if err != nil {
		return prompt
	}

	resolver := steps.NewStepOutputResolver(r.setupOutputs)
	builder.SetSourceResolver("steps", resolver)

	result, err := builder.GetResult()
	if err != nil {
		return prompt
	}

	str, ok := result.(string)
	if !ok {
		return prompt
	}

	return str
}

func (r *taskRunner) RunAgent(ctx context.Context, agentRunner agent.Runner) (*PhaseOutput, error) {
	r.prompt = r.resolvePromptTemplates(r.prompt)
	result, err := agentRunner.RunTask(ctx, r.prompt)
	if err != nil {
		detailErr := fmt.Errorf("failed to run agent: %w", err)
		return &PhaseOutput{
			Success: false,
			Error:   detailErr.Error(),
			Steps: []*steps.StepOutput{{
				Type:    "agent",
				Success: false,
				Error:   detailErr.Error(),
				Outputs: map[string]string{
					"output": err.Error(),
				},
			}},
		}, detailErr
	}

	output := result.GetOutput()
	r.output = output

	// Capture structured agent details
	tokenEstimate := result.GetTokenEstimate()
	agentDetails := &AgentDetails{
		TokenEstimate: &tokenEstimate,
		ToolCalls:     result.GetToolCalls(),
		FinalMessage:  result.GetFinalMessage(),
		Thinking:      result.GetThinking(),
	}

	return &PhaseOutput{
		Success:      true,
		AgentDetails: agentDetails,
		Steps: []*steps.StepOutput{{
			Type:    "agent",
			Success: true,
			Message: output,
			Outputs: map[string]string{
				"output": output,
			},
		}},
	}, nil
}

func (r *taskRunner) Verify(ctx context.Context) (*PhaseOutput, error) {
	out := &PhaseOutput{
		Steps:   make([]*steps.StepOutput, 0),
		Success: true,
	}

	stepOutputs := make(map[string]map[string]string)

	for i, s := range r.verify {
		res, err := s.Execute(ctx, &steps.StepInput{
			Agent: &steps.AgentContext{
				Prompt: r.prompt,
				Output: r.output,
			},
			Workdir:     r.baseDir,
			StepOutputs: stepOutputs,
			Random:      r.random,
		})

		out.Steps = append(out.Steps, res)
		if err != nil {
			out.Success = false
			out.Error = err.Error()
			return out, fmt.Errorf("verify[%d] failed: %w", i, err)
		}
		if res != nil && !res.Success {
			out.Success = false
		}

		// Accumulate outputs from this step
		if res != nil && res.Success && len(res.Outputs) > 0 && res.Type != "" {
			stepOutputs[res.Type] = res.Outputs
		}
	}

	return out, nil
}
