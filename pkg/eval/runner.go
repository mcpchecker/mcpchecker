package eval

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mcpchecker/mcpchecker/pkg/agent"
	"github.com/mcpchecker/mcpchecker/pkg/extension/client"
	"github.com/mcpchecker/mcpchecker/pkg/extension/resolver"
	"github.com/mcpchecker/mcpchecker/pkg/llmjudge"
	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/mcpchecker/mcpchecker/pkg/task"
	"github.com/mcpchecker/mcpchecker/pkg/tokens"
	"github.com/mcpchecker/mcpchecker/pkg/util"
)

type EvalResult struct {
	TaskName            string                    `json:"taskName"`
	TaskPath            string                    `json:"taskPath"`
	TaskPassed          bool                      `json:"taskPassed"`
	TaskOutput          string                    `json:"taskOutput"`
	TaskError           string                    `json:"taskError,omitempty"`
	TimedOut            bool                      `json:"timedOut,omitempty"`
	TaskJudgeReason     string                    `json:"taskJudgeReason,omitempty"`
	TaskJudgeError      string                    `json:"taskJudgeError,omitempty"`
	AgentExecutionError bool                      `json:"agentExecutionError,omitempty"` // True if agent failed to execute
	Difficulty          string                    `json:"difficulty"`
	Parallel            bool                      `json:"parallel,omitempty"`
	RunIndex            int                       `json:"runIndex,omitempty"`  // 0-indexed run number (for multi-run)
	TotalRuns           int                       `json:"totalRuns,omitempty"` // Total runs for this task (for multi-run)
	AssertionResults    *CompositeAssertionResult `json:"assertionResults"`
	AllAssertionsPassed bool                      `json:"allAssertionsPassed"`
	CallHistory         *mcpproxy.CallHistory     `json:"callHistory"`

	// TokenEstimate contains token count estimates from agent execution.
	// Uses tiktoken (cl100k_base encoding). Excludes system prompt and cache tokens.
	TokenEstimate *tokens.Estimate `json:"tokenEstimate,omitempty"`

	// JudgeTokenUsage contains token usage from LLM judge.
	JudgeTokenUsage *tokens.Usage `json:"judgeTokenUsage,omitempty"`

	// Phase outputs from task execution
	SetupOutput   *task.PhaseOutput `json:"setupOutput,omitempty"`
	AgentOutput   *task.PhaseOutput `json:"agentOutput,omitempty"`
	VerifyOutput  *task.PhaseOutput `json:"verifyOutput,omitempty"`
	CleanupOutput *task.PhaseOutput `json:"cleanupOutput,omitempty"`
}

type EvalRunner interface {
	Run(ctx context.Context, taskPattern string) (*EvalOutput, error)
	RunWithProgress(ctx context.Context, taskPattern string, callback ProgressCallback) (*EvalOutput, error)
}

// RunnerOptions configures the eval runner behavior
type RunnerOptions struct {
	ParallelWorkers   int
	Runs              int  // Number of times to run each task (default: 1)
	RunsExplicitlySet bool // True if Runs was explicitly set via CLI (overrides task-level runs)

	// Timeout overrides (CLI flags)
	DefaultTaskTimeout    string // Overrides eval config defaultTaskLimits.timeout for tasks without their own
	TaskTimeout           string // Hard override for ALL task timeouts
	DefaultCleanupTimeout string // Overrides eval config defaultTaskLimits.cleanupTimeout for tasks without their own
	CleanupTimeout        string // Hard override for ALL cleanup timeouts
}

type evalRunner struct {
	spec              *EvalSpec
	progressCallback  ProgressCallback
	parallelWorkers   int
	runs              int
	runsExplicitlySet bool

	// Timeout overrides from CLI
	defaultTaskTimeout    string
	taskTimeout           string
	defaultCleanupTimeout string
	cleanupTimeout        string
}

var _ EvalRunner = &evalRunner{}

type taskConfig struct {
	path       string
	spec       *task.TaskConfig
	assertions []*TaskAssertions // multiple assertion sets from matching TaskSets, evaluated independently
}

// NewRunner creates a new EvalRunner from an EvalSpec
func NewRunner(spec *EvalSpec, opts ...RunnerOptions) (EvalRunner, error) {
	if spec == nil {
		return nil, fmt.Errorf("eval spec cannot be nil")
	}

	workers := 1
	runs := 1
	runsExplicitlySet := false
	if len(opts) > 0 {
		if opts[0].ParallelWorkers > 0 {
			workers = opts[0].ParallelWorkers
		}
		if opts[0].Runs > 0 {
			runs = opts[0].Runs
		}
		runsExplicitlySet = opts[0].RunsExplicitlySet
	}

	r := &evalRunner{
		spec:              spec,
		progressCallback:  NoopProgressCallback,
		parallelWorkers:   workers,
		runs:              runs,
		runsExplicitlySet: runsExplicitlySet,
	}

	if len(opts) > 0 {
		r.defaultTaskTimeout = opts[0].DefaultTaskTimeout
		r.taskTimeout = opts[0].TaskTimeout
		r.defaultCleanupTimeout = opts[0].DefaultCleanupTimeout
		r.cleanupTimeout = opts[0].CleanupTimeout
	}

	return r, nil
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

	return agent.ResolveAgentRef(r.spec.Config.Agent)
}

func (r *evalRunner) Run(ctx context.Context, taskPattern string) (*EvalOutput, error) {
	return r.RunWithProgress(ctx, taskPattern, NoopProgressCallback)
}

func (r *evalRunner) RunWithProgress(ctx context.Context, taskPattern string, callback ProgressCallback) (*EvalOutput, error) {
	r.progressCallback = callback

	if taskPattern == "" {
		taskPattern = "." // match everything (any character matches all task names)
	}

	taskMatcher, err := regexp.Compile(taskPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to compile regexp for task name match: %w", err)
	}

	mcpConfig, err := r.loadMcpConfig()
	if err != nil {
		return nil, err
	}

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
	defer judge.Close()

	resolver := resolver.GetResolver(resolver.Options{
		BasePath: r.spec.BasePath(),
	})

	ctx = llmjudge.WithJudge(ctx, judge)

	taskConfigs, err := r.collectTaskConfigs(taskMatcher)
	if err != nil {
		return nil, err
	}

	// Build summary from resolved configuration
	summary := r.buildSummary(agentSpec, mcpConfig, judge, taskConfigs)

	r.progressCallback(ProgressEvent{
		Type:    EventEvalStart,
		Message: "Starting evaluation",
		Summary: summary,
	})

	// Group tasks by parallel support
	groups := groupTasksByParallelSupport(taskConfigs)

	results := make([]*EvalResult, 0, len(taskConfigs))

	for _, group := range groups {
		// Determine worker limit: use configured workers for parallel tasks, 1 for sequential
		workerLimit := 1
		if group.parallel && r.parallelWorkers > 1 {
			workerLimit = r.parallelWorkers
		}

		groupResults := r.runTaskGroup(ctx, runner, mcpConfig, resolver, group.tasks, workerLimit)
		results = append(results, groupResults...)
	}

	r.progressCallback(ProgressEvent{
		Type:    EventEvalComplete,
		Message: "Evaluation complete",
	})

	return &EvalOutput{
		Summary: summary,
		Results: results,
	}, nil
}

func (r *evalRunner) buildSummary(agentSpec *agent.AgentSpec, mcpConfig *mcpclient.MCPConfig, judge llmjudge.LLMJudge, taskConfigs []taskConfig) *EvalSummary {
	summary := &EvalSummary{
		ParallelWorkers: r.parallelWorkers,
		Runs:            r.runs,
	}

	// Agent — include ref-level info plus resolved spec details
	if r.spec.Config.Agent != nil {
		agentSummary := &AgentSummary{
			Type:  r.spec.Config.Agent.Type,
			Model: r.spec.Config.Agent.Model,
			Path:  r.spec.Config.Agent.Path,
		}
		if agentSpec != nil {
			agentSummary.Name = agentSpec.Metadata.Name
			if agentSummary.Model == "" && agentSpec.Builtin != nil {
				agentSummary.Model = agentSpec.Builtin.Model
			}
			if agentSpec.AcpConfig != nil {
				agentSummary.Command = agentSpec.AcpConfig.Cmd
			}
		}
		summary.Agent = agentSummary
	}

	// Judge
	if modelName := judge.ModelName(); modelName != "" && modelName != "noop" {
		judgeSummary := &JudgeSummary{Model: modelName}
		if r.spec.Config.LLMJudge != nil && r.spec.Config.LLMJudge.AgentRef != nil {
			ref := r.spec.Config.LLMJudge.AgentRef
			judgeSummary.Type = ref.Type
			judgeSummary.Path = ref.Path
			// Resolve spec for additional details (name, ACP command)
			if judgeSpec, err := agent.ResolveAgentRef(ref); err == nil && judgeSpec != nil {
				judgeSummary.Name = judgeSpec.Metadata.Name
				if judgeSummary.Model == "" && judgeSpec.Builtin != nil {
					judgeSummary.Model = judgeSpec.Builtin.Model
				}
				if judgeSpec.AcpConfig != nil {
					judgeSummary.Command = judgeSpec.AcpConfig.Cmd
				}
			}
		}
		summary.Judge = judgeSummary
	}

	// MCP servers (sorted by name for deterministic output)
	if mcpConfig != nil {
		servers := mcpConfig.GetEnabledServers()
		names := make([]string, 0, len(servers))
		for name := range servers {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			server := servers[name]
			serverType := "stdio"
			if server.IsHttp() {
				serverType = "http"
			}
			summary.MCPServers = append(summary.MCPServers, MCPServerSummary{
				Name:    name,
				Type:    serverType,
				URL:     sanitizeURL(server.URL),
				Command: server.Command,
			})
		}
	}

	// Task sets from config
	var taskSetSummaries []TaskSetSummary
	for _, ts := range r.spec.Config.TaskSets {
		taskSetSummaries = append(taskSetSummaries, TaskSetSummary{
			Glob:          ts.Glob,
			Path:          ts.Path,
			LabelSelector: ts.LabelSelector,
		})
	}

	// Matched task names
	taskNames := make([]string, 0, len(taskConfigs))
	for _, tc := range taskConfigs {
		taskNames = append(taskNames, tc.spec.Metadata.Name)
	}
	summary.Evals = &EvalsSummary{
		Names:    taskNames,
		TaskSets: taskSetSummaries,
	}

	// Timeouts
	timeout := &TimeoutSummary{
		Task:           r.taskTimeout,
		DefaultTask:    r.defaultTaskTimeout,
		Cleanup:        r.cleanupTimeout,
		DefaultCleanup: r.defaultCleanupTimeout,
	}
	if r.spec.Config.DefaultTaskLimits != nil {
		if timeout.DefaultTask == "" && r.spec.Config.DefaultTaskLimits.Timeout != "" {
			timeout.DefaultTask = r.spec.Config.DefaultTaskLimits.Timeout
		}
		if timeout.DefaultCleanup == "" && r.spec.Config.DefaultTaskLimits.CleanupTimeout != "" {
			timeout.DefaultCleanup = r.spec.Config.DefaultTaskLimits.CleanupTimeout
		}
	}
	if timeout.Task != "" || timeout.DefaultTask != "" || timeout.Cleanup != "" || timeout.DefaultCleanup != "" {
		summary.Timeout = timeout
	}

	return summary
}

func (r *evalRunner) collectTaskConfigs(rx *regexp.Regexp) ([]taskConfig, error) {
	taskConfigs := make([]taskConfig, 0)
	seen := make(map[string]int) // maps canonical path to index in taskConfigs for merging assertions

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

			// Canonicalize path for deduplication (resolves ./foo vs foo, symlinks, etc.)
			canonicalPath, err := filepath.Abs(path)
			if err != nil {
				canonicalPath = path // fallback to raw path if Abs fails
			}
			if resolved, err := filepath.EvalSymlinks(canonicalPath); err == nil {
				canonicalPath = resolved
			}

			// Keep display path clean but relative (avoids leaking machine-specific paths in results)
			displayPath := filepath.Clean(path)

			// If task already exists, append assertions to evaluate independently
			if idx, exists := seen[canonicalPath]; exists {
				if ts.Assertions != nil {
					taskConfigs[idx].assertions = append(taskConfigs[idx].assertions, ts.Assertions)
				}
				continue
			}

			seen[canonicalPath] = len(taskConfigs)
			var assertions []*TaskAssertions
			if ts.Assertions != nil {
				assertions = []*TaskAssertions{ts.Assertions}
			}
			taskConfigs = append(taskConfigs, taskConfig{
				path:       displayPath,
				spec:       taskSpec,
				assertions: assertions,
			})
		}
	}

	return taskConfigs, nil
}

// taskGroup represents a batch of tasks to run together
type taskGroup struct {
	tasks    []taskConfig
	parallel bool
}

// groupTasksByParallelSupport separates tasks into sequential and parallel groups.
// Sequential tasks run first (in order), then all parallel tasks run together as one batch.
func groupTasksByParallelSupport(tasks []taskConfig) []taskGroup {
	if len(tasks) == 0 {
		return nil
	}

	var sequential []taskConfig
	var parallel []taskConfig

	for _, tc := range tasks {
		if tc.spec.Metadata.Parallel {
			parallel = append(parallel, tc)
		} else {
			sequential = append(sequential, tc)
		}
	}

	var groups []taskGroup

	// Add sequential tasks as individual groups (run in order)
	for _, tc := range sequential {
		groups = append(groups, taskGroup{
			tasks:    []taskConfig{tc},
			parallel: false,
		})
	}

	// Add all parallel tasks as one group
	if len(parallel) > 0 {
		groups = append(groups, taskGroup{
			tasks:    parallel,
			parallel: true,
		})
	}

	return groups
}

// runTaskGroup runs a group of tasks with the specified worker limit.
// Each task gets its own MCP and extension managers to ensure isolation.
func (r *evalRunner) runTaskGroup(
	ctx context.Context,
	agentRunner agent.Runner,
	mcpConfig *mcpclient.MCPConfig,
	extResolver resolver.Resolver,
	tasks []taskConfig,
	workerLimit int,
) []*EvalResult {
	var allResults []*EvalResult
	var mu sync.Mutex

	var wg sync.WaitGroup
	sem := make(chan struct{}, workerLimit)

	for _, tc := range tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			taskResults := r.executeTask(ctx, agentRunner, mcpConfig, extResolver, tc)

			mu.Lock()
			allResults = append(allResults, taskResults...)
			mu.Unlock()
		}()
	}

	wg.Wait()

	return allResults
}

// getRunsForTask determines the number of runs for a specific task.
// Priority: CLI --runs (if explicitly set) > task metadata runs > default (1)
func (r *evalRunner) getRunsForTask(tc taskConfig) int {
	if r.runsExplicitlySet {
		return r.runs
	}
	if tc.spec.Metadata.Runs > 0 {
		return tc.spec.Metadata.Runs
	}
	return 1
}

// resolveTaskTimeout determines the effective task timeout for a specific task.
// Precedence (highest to lowest):
//  1. CLI --task-timeout (hard override)
//  2. task spec.limits.timeout (per-task)
//  3. CLI --default-task-timeout
//  4. eval config.defaultTaskLimits.timeout
//  5. no timeout (returns 0, false, nil)
func (r *evalRunner) resolveTaskTimeout(tc taskConfig) (time.Duration, bool, error) {
	if r.taskTimeout != "" {
		d, err := time.ParseDuration(r.taskTimeout)
		if err != nil {
			return 0, false, fmt.Errorf("invalid --task-timeout %q: %w", r.taskTimeout, err)
		}
		return d, true, nil
	}

	if tc.spec.Spec != nil && tc.spec.Spec.Limits != nil {
		d, ok, err := tc.spec.Spec.Limits.GetTimeout()
		if err != nil {
			return 0, false, err
		}
		if ok {
			return d, true, nil
		}
	}

	if r.defaultTaskTimeout != "" {
		d, err := time.ParseDuration(r.defaultTaskTimeout)
		if err != nil {
			return 0, false, fmt.Errorf("invalid --default-task-timeout %q: %w", r.defaultTaskTimeout, err)
		}
		return d, true, nil
	}

	if r.spec.Config.DefaultTaskLimits != nil {
		d, ok, err := r.spec.Config.DefaultTaskLimits.GetTimeout()
		if err != nil {
			return 0, false, err
		}
		if ok {
			return d, true, nil
		}
	}

	return 0, false, nil
}

// resolveCleanupTimeout determines the effective cleanup timeout for a specific task.
// Follows the same precedence pattern as resolveTaskTimeout.
func (r *evalRunner) resolveCleanupTimeout(tc taskConfig) (time.Duration, bool, error) {
	if r.cleanupTimeout != "" {
		d, err := time.ParseDuration(r.cleanupTimeout)
		if err != nil {
			return 0, false, fmt.Errorf("invalid --cleanup-timeout %q: %w", r.cleanupTimeout, err)
		}
		return d, true, nil
	}

	if tc.spec.Spec != nil && tc.spec.Spec.Limits != nil {
		d, ok, err := tc.spec.Spec.Limits.GetCleanupTimeout()
		if err != nil {
			return 0, false, err
		}
		if ok {
			return d, true, nil
		}
	}

	if r.defaultCleanupTimeout != "" {
		d, err := time.ParseDuration(r.defaultCleanupTimeout)
		if err != nil {
			return 0, false, fmt.Errorf("invalid --default-cleanup-timeout %q: %w", r.defaultCleanupTimeout, err)
		}
		return d, true, nil
	}

	if r.spec.Config.DefaultTaskLimits != nil {
		d, ok, err := r.spec.Config.DefaultTaskLimits.GetCleanupTimeout()
		if err != nil {
			return 0, false, err
		}
		if ok {
			return d, true, nil
		}
	}

	return 0, false, nil
}

// executeTask runs a task for the configured number of runs.
// Returns a slice of results, one per run.
func (r *evalRunner) executeTask(
	ctx context.Context,
	agentRunner agent.Runner,
	mcpConfig *mcpclient.MCPConfig,
	extResolver resolver.Resolver,
	tc taskConfig,
) []*EvalResult {
	runs := r.getRunsForTask(tc)
	results := make([]*EvalResult, 0, runs)

	for runIdx := 0; runIdx < runs; runIdx++ {
		result := r.executeSingleRun(ctx, agentRunner, mcpConfig, extResolver, tc)
		result.RunIndex = runIdx
		result.TotalRuns = runs
		results = append(results, result)
	}

	return results
}

// executeSingleRun runs a single task execution with its own isolated MCP and extension managers.
// Always returns a result, even on error.
func (r *evalRunner) executeSingleRun(
	ctx context.Context,
	agentRunner agent.Runner,
	mcpConfig *mcpclient.MCPConfig,
	extResolver resolver.Resolver,
	tc taskConfig,
) *EvalResult {
	// Create a separate MCP manager for this task
	taskMcpManager, err := mcpclient.NewManager(ctx, mcpConfig)
	if err != nil {
		return &EvalResult{
			TaskName:   tc.spec.Metadata.Name,
			TaskPath:   tc.path,
			Difficulty: tc.spec.Metadata.Difficulty,
			Parallel:   tc.spec.Metadata.Parallel,
			TaskPassed: false,
			TaskError:  fmt.Sprintf("failed to create mcp manager: %v", err),
		}
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = taskMcpManager.Close(cleanupCtx)
	}()

	// Create a separate extension manager for this task
	taskExtManager := client.NewManager(extResolver, client.ExtensionOptions{})
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = taskExtManager.ShutdownAll(cleanupCtx)
	}()

	for alias, ext := range r.spec.Config.Extensions {
		if err := taskExtManager.Register(alias, ext); err != nil {
			return &EvalResult{
				TaskName:   tc.spec.Metadata.Name,
				TaskPath:   tc.path,
				Difficulty: tc.spec.Metadata.Difficulty,
				Parallel:   tc.spec.Metadata.Parallel,
				TaskPassed: false,
				TaskError:  fmt.Sprintf("failed to register extension %s: %v", alias, err),
			}
		}
	}

	// Attach task-specific managers to context
	taskCtx := mcpclient.ManagerToContext(ctx, taskMcpManager)
	taskCtx = client.ManagerToContext(taskCtx, taskExtManager)

	result, err := r.runTask(taskCtx, agentRunner, tc)
	if err != nil && result == nil {
		return &EvalResult{
			TaskName:   tc.spec.Metadata.Name,
			TaskPath:   tc.path,
			Difficulty: tc.spec.Metadata.Difficulty,
			Parallel:   tc.spec.Metadata.Parallel,
			TaskPassed: false,
			TaskError:  err.Error(),
		}
	}

	return result
}

func (r *evalRunner) runTask(
	ctx context.Context,
	agentRunner agent.Runner,
	tc taskConfig,
) (*EvalResult, error) {
	result := &EvalResult{
		TaskName:   tc.spec.Metadata.Name,
		TaskPath:   tc.path,
		Difficulty: tc.spec.Metadata.Difficulty,
		Parallel:   tc.spec.Metadata.Parallel,
	}

	// Resolve timeouts
	taskTimeout, hasTaskTimeout, err := r.resolveTaskTimeout(tc)
	if err != nil {
		result.TaskPassed = false
		result.TaskError = err.Error()
		return result, nil
	}

	cleanupTimeout, hasCleanupTimeout, err := r.resolveCleanupTimeout(tc)
	if err != nil {
		result.TaskPassed = false
		result.TaskError = err.Error()
		return result, nil
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

	// Create a task-scoped context with timeout if configured
	taskCtx := ctx
	var taskCancel context.CancelFunc
	if hasTaskTimeout {
		taskCtx, taskCancel = context.WithTimeout(ctx, taskTimeout)
		defer taskCancel()
	}

	taskRunner, manager, cleanup, err := r.setupTaskResources(taskCtx, tc, result)
	if err != nil {
		result.TaskPassed = false
		// Check if the error was caused by timeout
		if hasTaskTimeout && taskCtx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.TaskError = fmt.Sprintf("task exceeded timeout of %s during setup", taskTimeout)
			r.progressCallback(ProgressEvent{
				Type:    EventTaskTimeout,
				Message: fmt.Sprintf("Task %s timed out after %s", tc.spec.Metadata.Name, taskTimeout),
				Task:    result,
			})
		} else {
			result.TaskError = err.Error()
			r.progressCallback(ProgressEvent{
				Type:    EventTaskError,
				Message: fmt.Sprintf("Task setup failed: %s", tc.spec.Metadata.Name),
				Task:    result,
			})
		}
		return result, nil
	}

	// Defer cleanup with its own timeout context, independent of task timeout
	defer func() {
		var cleanupCtx context.Context
		var cleanupCancel context.CancelFunc
		if hasCleanupTimeout {
			cleanupCtx, cleanupCancel = context.WithTimeout(context.Background(), cleanupTimeout)
		} else {
			cleanupCtx = context.Background()
			cleanupCancel = func() {}
		}
		defer cleanupCancel()
		cleanup(cleanupCtx)
	}()

	r.executeTaskSteps(taskCtx, taskRunner, agentRunner, manager, result)

	// Check if executeTaskSteps was terminated by timeout
	if hasTaskTimeout && taskCtx.Err() == context.DeadlineExceeded && !result.TimedOut {
		result.TimedOut = true
		result.TaskPassed = false
		result.TaskError = fmt.Sprintf("task exceeded timeout of %s", taskTimeout)
		r.progressCallback(ProgressEvent{
			Type:    EventTaskTimeout,
			Message: fmt.Sprintf("Task %s timed out after %s", tc.spec.Metadata.Name, taskTimeout),
			Task:    result,
		})
	}

	// Assertions and token computation use the original ctx, not taskCtx
	r.progressCallback(ProgressEvent{
		Type:    EventTaskAssertions,
		Message: fmt.Sprintf("Evaluating assertions for task: %s", tc.spec.Metadata.Name),
		Task:    result,
	})

	r.evaluateTaskAssertions(tc, manager, result)

	result.CallHistory = manager.GetAllCallHistory()

	// Compute per-call token counts on CallHistory records
	callHistoryErr := mcpproxy.ComputeCallHistoryTokens(result.CallHistory)

	// Compute MCP schema overhead (tool definitions + server instructions)
	schemaTokens, schemaErr := mcpproxy.ComputeSchemaTokens(ctx, manager.GetMcpServers())

	// Ensure TokenEstimate exists so MCP token data is always reported,
	// even on agent failure or shell runner
	if result.TokenEstimate == nil {
		result.TokenEstimate = &tokens.Estimate{}
	}

	// Propagate token-counting errors
	var tokenErrors []string
	if result.TokenEstimate.Error != "" {
		tokenErrors = append(tokenErrors, result.TokenEstimate.Error)
	}
	if callHistoryErr != "" {
		tokenErrors = append(tokenErrors, callHistoryErr)
	}
	if schemaErr != nil {
		tokenErrors = append(tokenErrors, schemaErr.Error())
	}
	result.TokenEstimate.Error = strings.Join(tokenErrors, "; ")

	result.TokenEstimate.McpSchemaTokens = schemaTokens
	result.TokenEstimate.MergeCallHistory(result.CallHistory)
	result.TokenEstimate.RecalculateAggregates(result.CallHistory)

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
	result *EvalResult,
) (task.TaskRunner, mcpproxy.ServerManager, func(context.Context), error) {
	mcpManager, ok := mcpclient.ManagerFromContext(ctx)
	if !ok {
		return nil, nil, nil, fmt.Errorf("mcp manager not found in context")
	}

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

	cleanup := func(cleanupCtx context.Context) {
		cleanupOutput, _ := taskRunner.Cleanup(cleanupCtx)
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
		if agentOutput != nil && agentOutput.AgentDetails != nil {
			result.TaskOutput = agent.FinalMessageFromSteps(agentOutput.AgentDetails.OutputSteps)
		}
		return
	}

	if agentOutput != nil && agentOutput.AgentDetails != nil {
		result.TaskOutput = agent.FinalMessageFromSteps(agentOutput.AgentDetails.OutputSteps)
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
		judgeUsage := &tokens.Usage{}
		for _, step := range verifyOutput.Steps {
			if step != nil && step.Type == "llmJudge" && step.Usage != nil {
				judgeUsage.Add(step.Usage)
			}
		}

		if judgeUsage.InputTokens > 0 || judgeUsage.OutputTokens > 0 {
			result.JudgeTokenUsage = judgeUsage
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
	if len(tc.assertions) == 0 {
		// No assertions = all pass
		result.AllAssertionsPassed = true
		return
	}

	// Evaluate each assertion set independently and combine results
	callHistory := manager.GetAllCallHistory()
	var combinedResults *CompositeAssertionResult
	allPassed := true

	for _, assertions := range tc.assertions {
		if assertions == nil {
			continue
		}
		evaluator := NewCompositeAssertionEvaluator(assertions)
		assertionResults := evaluator.Evaluate(callHistory)

		if combinedResults == nil {
			combinedResults = assertionResults
		} else {
			combinedResults = combinedResults.Merge(assertionResults)
		}

		if !assertionResults.Succeeded() {
			allPassed = false
		}
	}

	result.AssertionResults = combinedResults
	result.AllAssertionsPassed = allPassed
}
