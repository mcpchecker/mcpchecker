package eval

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/mcpchecker/mcpchecker/pkg/agent"
	"github.com/mcpchecker/mcpchecker/pkg/extension/client"
	"github.com/mcpchecker/mcpchecker/pkg/extension/resolver"
	"github.com/mcpchecker/mcpchecker/pkg/llmjudge"
	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/mcpchecker/mcpchecker/pkg/task"
	"github.com/mcpchecker/mcpchecker/pkg/usage"
	"github.com/mcpchecker/mcpchecker/pkg/util"
)

type EvalResult struct {
	TaskName            string                    `json:"taskName"`
	TaskPath            string                    `json:"taskPath"`
	TaskPassed          bool                      `json:"taskPassed"`
	TaskOutput          string                    `json:"taskOutput"`
	TaskError           string                    `json:"taskError,omitempty"`
	TaskJudgeReason     string                    `json:"taskJudgeReason,omitempty"`
	TaskJudgeError      string                    `json:"taskJudgeError,omitempty"`
	AgentExecutionError bool                      `json:"agentExecutionError,omitempty"` // True if agent failed to execute
	Difficulty          string                    `json:"difficulty"`
	AssertionResults    *CompositeAssertionResult `json:"assertionResults"`
	AllAssertionsPassed bool                      `json:"allAssertionsPassed"`
	CallHistory         *mcpproxy.CallHistory     `json:"callHistory"`

	// TokenEstimate contains token count estimates from agent execution.
	// Uses tiktoken (cl100k_base encoding). Excludes system prompt and cache tokens.
	TokenEstimate *agent.TokenEstimate `json:"tokenEstimate,omitempty"`

	// JudgeTokenUsage contains token usage from LLM judge.
	JudgeTokenUsage *agent.ActualUsage `json:"judgeTokenUsage,omitempty"`

	// Phase outputs from task execution
	SetupOutput   *task.PhaseOutput `json:"setupOutput,omitempty"`
	AgentOutput   *task.PhaseOutput `json:"agentOutput,omitempty"`
	VerifyOutput  *task.PhaseOutput `json:"verifyOutput,omitempty"`
	CleanupOutput *task.PhaseOutput `json:"cleanupOutput,omitempty"`
}

type EvalRunner interface {
	Run(ctx context.Context, taskPattern string) ([]*EvalResult, error)
	RunWithProgress(ctx context.Context, taskPattern string, callback ProgressCallback) ([]*EvalResult, error)
}

type evalRunner struct {
	spec             *EvalSpec
	progressCallback ProgressCallback
}

var _ EvalRunner = &evalRunner{}

type taskConfig struct {
	path       string
	spec       *task.TaskConfig
	assertions *TaskAssertions
}

// NewRunner creates a new EvalRunner from an EvalSpec
func NewRunner(spec *EvalSpec) (EvalRunner, error) {
	if spec == nil {
		return nil, fmt.Errorf("eval spec cannot be nil")
	}

	return &evalRunner{
		spec:             spec,
		progressCallback: NoopProgressCallback,
	}, nil
}

func (r *evalRunner) loadMcpConfig() (*mcpclient.MCPConfig, error) {
	// Priority 1: Config file
	if r.spec.Config.McpConfigFile != "" {
		config, err := mcpclient.ParseConfigFile(r.spec.Config.McpConfigFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load MCP config from file: %w", err)
		}
		return config, nil
	}

	// Priority 2: Environment variables
	config, err := mcpclient.ConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to load MCP config from environment: %w", err)
	}
	if config != nil {
		return config, nil
	}

	// Neither available
	return nil, fmt.Errorf("no MCP configuration found: specify mcpConfigFile in eval config or set MCP_URL/MCP_COMMAND environment variables")
}

func (r *evalRunner) loadAgentSpec() (*agent.AgentSpec, error) {
	if r.spec.Config.Agent == nil {
		return nil, fmt.Errorf("agent must be specified in eval config")
	}

	agentRef := r.spec.Config.Agent

	// Handle file-based agent configuration
	if agentRef.Type == "file" {
		if agentRef.Path == "" {
			return nil, fmt.Errorf("path must be specified when agent type is 'file'")
		}
		return agent.LoadWithBuiltins(agentRef.Path)
	}

	// Handle builtin agent configuration
	// Type should be in format "builtin.X" where X is the builtin type
	const builtinPrefix = "builtin."
	if len(agentRef.Type) <= len(builtinPrefix) || agentRef.Type[:len(builtinPrefix)] != builtinPrefix {
		return nil, fmt.Errorf("agent type must be either 'file' or 'builtin.X' format, got: %s", agentRef.Type)
	}

	builtinType := agentRef.Type[len(builtinPrefix):]
	builtinAgent, ok := agent.GetBuiltinType(builtinType)
	if !ok {
		return nil, fmt.Errorf("unknown builtin agent type: %s", builtinType)
	}

	// Enforce model requirement for this builtin type
	if builtinAgent.RequiresModel() && agentRef.Model == "" {
		return nil, fmt.Errorf("builtin type '%s' requires a model to be specified", builtinType)
	}

	// Validate environment (binaries, env vars, etc.) before using the agent
	if err := builtinAgent.ValidateEnvironment(); err != nil {
		return nil, fmt.Errorf("builtin type '%s' environment validation failed: %w", builtinType, err)
	}

	// Get the default spec for this builtin agent
	agentSpec, err := builtinAgent.GetDefaults(agentRef.Model)
	if err != nil {
		return nil, fmt.Errorf("failed to get defaults for builtin agent %s: %w", builtinType, err)
	}

	return agentSpec, nil
}

func (r *evalRunner) Run(ctx context.Context, taskPattern string) ([]*EvalResult, error) {
	return r.RunWithProgress(ctx, taskPattern, NoopProgressCallback)
}

func (r *evalRunner) RunWithProgress(ctx context.Context, taskPattern string, callback ProgressCallback) ([]*EvalResult, error) {
	r.progressCallback = callback

	if taskPattern == "" {
		taskPattern = "." // match everything (any character matches all task names)
	}

	taskMatcher, err := regexp.Compile(taskPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to compile regexp for task name match: %w", err)
	}

	r.progressCallback(ProgressEvent{
		Type:    EventEvalStart,
		Message: "Starting evaluation",
	})

	mcpConfig, err := r.loadMcpConfig()
	if err != nil {
		return nil, err
	}

	mcpManager, err := mcpclient.NewManager(ctx, mcpConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create mcp manager: %w", err)
	}
	defer func() {
		_ = mcpManager.Close(ctx)
	}()

	ctx = mcpclient.ManagerToContext(ctx, mcpManager)

	agentSpec, err := r.loadAgentSpec()
	if err != nil {
		return nil, fmt.Errorf("failed to load agent spec: %w", err)
	}

	runner, err := agent.NewRunnerForSpec(agentSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent runner from spec: %w", err)
	}

	judge, err := llmjudge.NewLLMJudge(r.spec.Config.LLMJudge)
	if err != nil {
		return nil, fmt.Errorf("failed to create llm judge from spec: %w", err)
	}

	resolver := resolver.GetResolver(resolver.Options{
		BasePath: r.spec.BasePath(),
	})

	manager := client.NewManager(resolver, client.ExtensionOptions{})
	defer manager.ShutdownAll(ctx)

	for alias, ext := range r.spec.Config.Extensions {
		if err := manager.Register(alias, ext); err != nil {
			return nil, fmt.Errorf("registering extension %q (%s): %w", alias, ext.Package, err)
		}
	}

	ctx = client.ManagerToContext(ctx, manager)

	ctx = llmjudge.WithJudge(ctx, judge)

	taskConfigs, err := r.collectTaskConfigs(taskMatcher)
	if err != nil {
		return nil, err
	}

	results := make([]*EvalResult, 0, len(taskConfigs))
	var runErr error
	for _, tc := range taskConfigs {
		result, err := r.runTask(ctx, runner, mcpManager, tc)
		if err != nil {
			runErr = errors.Join(runErr, err)
		} else {
			results = append(results, result)
		}
	}

	r.progressCallback(ProgressEvent{
		Type:    EventEvalComplete,
		Message: "Evaluation complete",
	})

	return results, runErr
}

func (r *evalRunner) collectTaskConfigs(rx *regexp.Regexp) ([]taskConfig, error) {
	taskConfigs := make([]taskConfig, 0)

	for _, ts := range r.spec.Config.TaskSets {
		var paths []string
		var err error

		if ts.Glob != "" {
			paths, err = filepath.Glob(ts.Glob)
			if err != nil {
				return nil, fmt.Errorf("failed to glob %s: %w", ts.Glob, err)
			}
		} else if ts.Path != "" {
			paths = []string{ts.Path}
		}

		for _, path := range paths {
			taskSpec, err := task.FromFile(path)
			if err != nil {
				// Skip files that are not tasks (e.g., eval.yaml files in the same directory)
				if errors.Is(err, util.ErrWrongKind) {
					continue
				}
				return nil, fmt.Errorf("failed to load task at path %s: %w", path, err)
			}

			if !rx.MatchString(taskSpec.Metadata.Name) {
				continue
			}

			// Filter by label selector if specified
			if !matchesLabelSelector(taskSpec.Metadata.Labels, ts.LabelSelector) {
				continue
			}

			taskConfigs = append(taskConfigs, taskConfig{
				path:       path,
				spec:       taskSpec,
				assertions: ts.Assertions,
			})
		}
	}

	return taskConfigs, nil
}

func (r *evalRunner) runTask(
	ctx context.Context,
	agentRunner agent.Runner,
	mcpManager mcpclient.Manager,
	tc taskConfig,
) (*EvalResult, error) {
	result := &EvalResult{
		TaskName:   tc.spec.Metadata.Name,
		TaskPath:   tc.path,
		Difficulty: tc.spec.Metadata.Difficulty,
	}

	r.progressCallback(ProgressEvent{
		Type:    EventTaskStart,
		Message: fmt.Sprintf("Starting task: %s", tc.spec.Metadata.Name),
		Task:    result,
	})

	r.progressCallback(ProgressEvent{
		Type:    EventTaskSetup,
		Message: fmt.Sprintf("Setting up task: %s", tc.spec.Metadata.Name),
		Task:    result,
	})

	taskRunner, manager, cleanup, err := r.setupTaskResources(ctx, tc, mcpManager, result)
	if err != nil {
		result.TaskPassed = false
		result.TaskError = err.Error()
		r.progressCallback(ProgressEvent{
			Type:    EventTaskError,
			Message: fmt.Sprintf("Task setup failed: %s", tc.spec.Metadata.Name),
			Task:    result,
		})
		return result, nil
	}
	defer cleanup()

	r.executeTaskSteps(ctx, taskRunner, agentRunner, manager, result)

	r.progressCallback(ProgressEvent{
		Type:    EventTaskAssertions,
		Message: fmt.Sprintf("Evaluating assertions for task: %s", tc.spec.Metadata.Name),
		Task:    result,
	})

	r.evaluateTaskAssertions(tc, manager, result)

	result.CallHistory = manager.GetAllCallHistory()

	r.progressCallback(ProgressEvent{
		Type:    EventTaskComplete,
		Message: fmt.Sprintf("Completed task: %s (passed: %v)", tc.spec.Metadata.Name, result.TaskPassed),
		Task:    result,
	})

	return result, nil
}

func (r *evalRunner) setupTaskResources(
	ctx context.Context,
	tc taskConfig,
	mcpManager mcpclient.Manager,
	result *EvalResult,
) (task.TaskRunner, mcpproxy.ServerManager, func(), error) {
	taskRunner, err := task.NewTaskRunner(ctx, tc.spec)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create task runner for task '%s': %w", tc.spec.Metadata.Name, err)
	}

	manager, err := mcpproxy.NewServerManager(ctx, mcpManager)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create mcp proxy server manager: %w", err)
	}

	if err := manager.Start(ctx); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to start mcp proxy servers: %w", err)
	}

	setupOutput, err := taskRunner.Setup(ctx)
	result.SetupOutput = setupOutput
	if err != nil {
		manager.Close()
		return nil, nil, nil, fmt.Errorf("failed to setup task: %w", err)
	}

	cleanup := func() {
		cleanupOutput, _ := taskRunner.Cleanup(ctx)
		result.CleanupOutput = cleanupOutput
		manager.Close()
	}

	return taskRunner, manager, cleanup, nil
}

func (r *evalRunner) executeTaskSteps(
	ctx context.Context,
	taskRunner task.TaskRunner,
	agentRunner agent.Runner,
	manager mcpproxy.ServerManager,
	result *EvalResult,
) {
	r.progressCallback(ProgressEvent{
		Type:    EventTaskRunning,
		Message: fmt.Sprintf("Running agent for task: %s", result.TaskName),
		Task:    result,
	})

	agentRunner = agentRunner.WithMcpServerInfo(manager)

	if util.IsVerbose(ctx) {
		fmt.Printf("  → Agent '%s' is working…\n", agentRunner.AgentName())
	}
	agentOutput, err := taskRunner.RunAgent(ctx, agentRunner)
	result.AgentOutput = agentOutput
	if err != nil {
		result.TaskPassed = false
		result.TaskError = err.Error()
		result.AgentExecutionError = true
		// Extract agent output from phase output for backwards compatibility
		if agentOutput != nil && len(agentOutput.Steps) > 0 {
			if out, ok := agentOutput.Steps[0].Outputs["output"]; ok {
				result.TaskOutput = out
			}
		}
		return
	}

	// Extract agent output from phase output for backwards compatibility
	if agentOutput != nil && len(agentOutput.Steps) > 0 {
		if out, ok := agentOutput.Steps[0].Outputs["output"]; ok {
			result.TaskOutput = out
		}
	}

	// Extract token estimate from agent details
	if agentOutput != nil && agentOutput.AgentDetails != nil {
		result.TokenEstimate = agentOutput.AgentDetails.TokenEstimate
	}

	r.progressCallback(ProgressEvent{
		Type:    EventTaskVerifying,
		Message: fmt.Sprintf("Verifying task: %s", result.TaskName),
		Task:    result,
	})

	verifyOutput, err := taskRunner.Verify(ctx)
	result.VerifyOutput = verifyOutput

	// Aggregate judge usage from verify phase steps
	if verifyOutput != nil {
		judgeUsage := &usage.TokenUsage{}
		for _, step := range verifyOutput.Steps {
			if step != nil && step.Type == "llmJudge" && step.Usage != nil {
				judgeUsage.Add(step.Usage)
			}
		}

		if judgeUsage.GetInput() > 0 || judgeUsage.GetOutput() > 0 {
			result.JudgeTokenUsage = agent.GetActualUsageFromTokenUsage(judgeUsage)
		}
	}

	if err != nil {
		result.TaskPassed = false
		result.TaskError = fmt.Sprintf("verification failed: %s", err.Error())
	} else if verifyOutput != nil && !verifyOutput.Success {
		result.TaskPassed = false
		result.TaskError = "one or more verification steps failed"
	} else {
		result.TaskPassed = true
	}

	// Extract judge results from verify phase output if LLM judge was used
	r.extractJudgeResults(verifyOutput, result)
}

func (r *evalRunner) extractJudgeResults(verifyOutput *task.PhaseOutput, result *EvalResult) {
	if verifyOutput == nil {
		return
	}

	// Look for llmJudge step outputs and extract their results
	for _, step := range verifyOutput.Steps {
		if step == nil || step.Type != "llmJudge" {
			continue
		}
		// The judge's reason is in Message for both pass and fail
		result.TaskJudgeReason = step.Message
		// If there was a judge error (API failure), it would have caused an error return
		// so we don't need to check for TaskJudgeError here - the verify phase would have failed
		break // Only capture first llmJudge result
	}
}

func (r *evalRunner) evaluateTaskAssertions(
	tc taskConfig,
	manager mcpproxy.ServerManager,
	result *EvalResult,
) {
	if tc.assertions != nil {
		evaluator := NewCompositeAssertionEvaluator(tc.assertions)
		assertionResults := evaluator.Evaluate(manager.GetAllCallHistory())

		result.AssertionResults = assertionResults
		result.AllAssertionsPassed = assertionResults.Succeeded()
	} else {
		// No assertions = all pass
		result.AllAssertionsPassed = true
	}
}
