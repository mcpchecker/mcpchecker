package eval

import (
	"os"
	"regexp"
	"testing"

	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	"github.com/mcpchecker/mcpchecker/pkg/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAgentSpec(t *testing.T) {
	tests := map[string]struct {
		setupEnv    func()
		cleanupEnv  func()
		spec        *EvalSpec
		expectErr   bool
		errContains string
		validate    func(t *testing.T, runner *evalRunner)
	}{
		"inline agent - builtin.claude-code": {
			spec: &EvalSpec{
				Config: EvalConfig{
					Agent: &AgentRef{
						Type: "builtin.claude-code",
					},
				},
			},
			validate: func(t *testing.T, runner *evalRunner) {
				agentSpec, err := runner.loadAgentSpec()
				// Note: This may fail with environment validation error if claude binary is not in PATH
				// That's expected behavior - the test will skip validation if claude is not available
				if err != nil {
					if assert.Contains(t, err.Error(), "environment validation failed") {
						t.Skip("claude binary not in PATH, skipping test")
					}
					require.NoError(t, err) // Fail if it's a different error
				}
				require.NotNil(t, agentSpec)
				assert.Equal(t, "claude-code", agentSpec.Metadata.Name)
			},
		},
		"inline agent - builtin.llm-agent": {
			spec: &EvalSpec{
				Config: EvalConfig{
					Agent: &AgentRef{
						Type:  "builtin.llm-agent",
						Model: "openai:gpt-4",
					},
				},
			},
			validate: func(t *testing.T, runner *evalRunner) {
				agentSpec, err := runner.loadAgentSpec()
				require.NoError(t, err)
				require.NotNil(t, agentSpec)
				assert.Equal(t, "llm-agent-openai:gpt-4", agentSpec.Metadata.Name)
				require.NotNil(t, agentSpec.Builtin)
				assert.Equal(t, "llm-agent", agentSpec.Builtin.Type)
				assert.Equal(t, "openai:gpt-4", agentSpec.Builtin.Model)
			},
		},
		"inline agent - builtin.llm-agent without model": {
			spec: &EvalSpec{
				Config: EvalConfig{
					Agent: &AgentRef{
						Type: "builtin.llm-agent",
					},
				},
			},
			expectErr:   true,
			errContains: "requires a model to be specified",
		},
		"inline agent - unknown type": {
			spec: &EvalSpec{
				Config: EvalConfig{
					Agent: &AgentRef{
						Type: "builtin.unknown-agent",
					},
				},
			},
			expectErr:   true,
			errContains: "unknown builtin agent type",
		},
		"no agent configuration": {
			spec: &EvalSpec{
				Config: EvalConfig{},
			},
			expectErr:   true,
			errContains: "agent must be specified",
		},
		"file agent without path": {
			spec: &EvalSpec{
				Config: EvalConfig{
					Agent: &AgentRef{
						Type: "file",
					},
				},
			},
			expectErr:   true,
			errContains: "path must be specified when agent type is 'file'",
		},
		"invalid agent type format": {
			spec: &EvalSpec{
				Config: EvalConfig{
					Agent: &AgentRef{
						Type: "invalid-format",
					},
				},
			},
			expectErr:   true,
			errContains: "agent type must be either 'file' or 'builtin.X' format",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.setupEnv != nil {
				tc.setupEnv()
			}
			if tc.cleanupEnv != nil {
				defer tc.cleanupEnv()
			}

			runner := &evalRunner{
				spec: tc.spec,
			}

			if tc.expectErr {
				_, err := runner.loadAgentSpec()
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			if tc.validate != nil {
				tc.validate(t, runner)
			}
		})
	}
}

func TestMatchesLabelSelector(t *testing.T) {
	tests := map[string]struct {
		taskLabels map[string]string
		selector   map[string]string
		expected   bool
	}{
		"empty selector matches any labels": {
			taskLabels: map[string]string{"suite": "kubernetes"},
			selector:   map[string]string{},
			expected:   true,
		},
		"nil selector matches any labels": {
			taskLabels: map[string]string{"suite": "kubernetes"},
			selector:   nil,
			expected:   true,
		},
		"exact match": {
			taskLabels: map[string]string{"suite": "kubernetes"},
			selector:   map[string]string{"suite": "kubernetes"},
			expected:   true,
		},
		"multiple labels all match": {
			taskLabels: map[string]string{
				"suite":    "kiali",
				"requires": "istio",
			},
			selector: map[string]string{
				"suite":    "kiali",
				"requires": "istio",
			},
			expected: true,
		},
		"selector has subset of task labels": {
			taskLabels: map[string]string{
				"suite":    "kubernetes",
				"category": "basic",
				"requires": "cluster",
			},
			selector: map[string]string{
				"suite": "kubernetes",
			},
			expected: true,
		},
		"task has subset of selector labels - no match": {
			taskLabels: map[string]string{
				"suite": "kubernetes",
			},
			selector: map[string]string{
				"suite":    "kubernetes",
				"requires": "istio",
			},
			expected: false,
		},
		"value mismatch": {
			taskLabels: map[string]string{"suite": "kubernetes"},
			selector:   map[string]string{"suite": "kiali"},
			expected:   false,
		},
		"key not present in task": {
			taskLabels: map[string]string{"suite": "kubernetes"},
			selector:   map[string]string{"category": "basic"},
			expected:   false,
		},
		"empty task labels with non-empty selector": {
			taskLabels: map[string]string{},
			selector:   map[string]string{"suite": "kubernetes"},
			expected:   false,
		},
		"nil task labels with non-empty selector": {
			taskLabels: nil,
			selector:   map[string]string{"suite": "kubernetes"},
			expected:   false,
		},
		"both empty - should match": {
			taskLabels: map[string]string{},
			selector:   map[string]string{},
			expected:   true,
		},
		"both nil - should match": {
			taskLabels: nil,
			selector:   nil,
			expected:   true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := matchesLabelSelector(tc.taskLabels, tc.selector)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestLoadMcpConfig(t *testing.T) {
	// Helper to clear all MCP env vars
	clearEnv := func() {
		envVars := []string{
			mcpclient.EnvMcpURL, mcpclient.EnvMcpHost, mcpclient.EnvMcpPort, mcpclient.EnvMcpPath,
			mcpclient.EnvMcpCommand, mcpclient.EnvMcpArgs, mcpclient.EnvMcpEnv, mcpclient.EnvMcpServerName,
			mcpclient.EnvMcpHeaders, mcpclient.EnvMcpEnableAllTools,
		}
		for _, v := range envVars {
			os.Unsetenv(v)
		}
	}

	tests := map[string]struct {
		setupEnv     func()
		cleanupEnv   func()
		spec         *EvalSpec
		expectErr    bool
		errContains  string
		validateFunc func(t *testing.T, config *mcpclient.MCPConfig)
	}{
		"config file takes priority over env vars": {
			setupEnv: func() {
				// Set env vars that would normally create a config
				os.Setenv(mcpclient.EnvMcpURL, "http://env-server:8080/mcp")
			},
			cleanupEnv: clearEnv,
			spec: &EvalSpec{
				Config: EvalConfig{
					McpConfigFile: "../mcpclient/testdata/basic.json",
				},
			},
			validateFunc: func(t *testing.T, config *mcpclient.MCPConfig) {
				require.NotNil(t, config)
				// Should load from file (filesystem server), not from env (env-server)
				_, hasFilesystem := config.MCPServers["filesystem"]
				assert.True(t, hasFilesystem, "should have filesystem server from config file")
				_, hasDefault := config.MCPServers["default"]
				assert.False(t, hasDefault, "should not have default server from env vars")
			},
		},
		"env vars used when no config file": {
			setupEnv: func() {
				os.Setenv(mcpclient.EnvMcpURL, "http://localhost:9090/mcp")
				os.Setenv(mcpclient.EnvMcpServerName, "test-server")
			},
			cleanupEnv: clearEnv,
			spec: &EvalSpec{
				Config: EvalConfig{
					McpConfigFile: "", // No config file
				},
			},
			validateFunc: func(t *testing.T, config *mcpclient.MCPConfig) {
				require.NotNil(t, config)
				server, hasServer := config.MCPServers["test-server"]
				assert.True(t, hasServer, "should have test-server from env vars")
				assert.Equal(t, "http://localhost:9090/mcp", server.URL)
			},
		},
		"error when neither config file nor env vars available": {
			setupEnv:   clearEnv,
			cleanupEnv: clearEnv,
			spec: &EvalSpec{
				Config: EvalConfig{
					McpConfigFile: "",
				},
			},
			expectErr:   true,
			errContains: "no MCP configuration found",
		},
		"error when config file does not exist": {
			setupEnv:   clearEnv,
			cleanupEnv: clearEnv,
			spec: &EvalSpec{
				Config: EvalConfig{
					McpConfigFile: "/nonexistent/path/config.json",
				},
			},
			expectErr:   true,
			errContains: "failed to load MCP config from file",
		},
		"stdio server from env vars": {
			setupEnv: func() {
				os.Setenv(mcpclient.EnvMcpCommand, "npx")
				os.Setenv(mcpclient.EnvMcpArgs, "-y,@modelcontextprotocol/server-filesystem,/tmp")
			},
			cleanupEnv: clearEnv,
			spec: &EvalSpec{
				Config: EvalConfig{
					McpConfigFile: "",
				},
			},
			validateFunc: func(t *testing.T, config *mcpclient.MCPConfig) {
				require.NotNil(t, config)
				server, hasServer := config.MCPServers["default"]
				require.True(t, hasServer, "should have default server from env vars")
				assert.Equal(t, "npx", server.Command)
				assert.Equal(t, []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"}, server.Args)
				assert.True(t, server.IsStdio())
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.setupEnv != nil {
				tc.setupEnv()
			}
			if tc.cleanupEnv != nil {
				defer tc.cleanupEnv()
			}

			runner := &evalRunner{
				spec: tc.spec,
			}

			config, err := runner.loadMcpConfig()

			if tc.expectErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			require.NoError(t, err)
			if tc.validateFunc != nil {
				tc.validateFunc(t, config)
			}
		})
	}
}

func TestNewRunnerWithOptions(t *testing.T) {
	spec := &EvalSpec{
		Config: EvalConfig{},
	}

	tests := map[string]struct {
		opts                     []RunnerOptions
		expectedWorkers          int
		expectedRuns             int
		expectedRunsExplicitlySet bool
	}{
		"no options - defaults to 1": {
			opts:                     nil,
			expectedWorkers:          1,
			expectedRuns:             1,
			expectedRunsExplicitlySet: false,
		},
		"empty options - defaults to 1": {
			opts:                     []RunnerOptions{{}},
			expectedWorkers:          1,
			expectedRuns:             1,
			expectedRunsExplicitlySet: false,
		},
		"zero workers - defaults to 1": {
			opts:                     []RunnerOptions{{ParallelWorkers: 0}},
			expectedWorkers:          1,
			expectedRuns:             1,
			expectedRunsExplicitlySet: false,
		},
		"negative workers - defaults to 1": {
			opts:                     []RunnerOptions{{ParallelWorkers: -5}},
			expectedWorkers:          1,
			expectedRuns:             1,
			expectedRunsExplicitlySet: false,
		},
		"valid workers": {
			opts:                     []RunnerOptions{{ParallelWorkers: 4}},
			expectedWorkers:          4,
			expectedRuns:             1,
			expectedRunsExplicitlySet: false,
		},
		"valid runs": {
			opts:                     []RunnerOptions{{Runs: 5}},
			expectedWorkers:          1,
			expectedRuns:             5,
			expectedRunsExplicitlySet: false,
		},
		"runs explicitly set": {
			opts:                     []RunnerOptions{{Runs: 3, RunsExplicitlySet: true}},
			expectedWorkers:          1,
			expectedRuns:             3,
			expectedRunsExplicitlySet: true,
		},
		"all options set": {
			opts:                     []RunnerOptions{{ParallelWorkers: 4, Runs: 5, RunsExplicitlySet: true}},
			expectedWorkers:          4,
			expectedRuns:             5,
			expectedRunsExplicitlySet: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			runner, err := NewRunner(spec, tc.opts...)
			require.NoError(t, err)

			r := runner.(*evalRunner)
			assert.Equal(t, tc.expectedWorkers, r.parallelWorkers)
			assert.Equal(t, tc.expectedRuns, r.runs)
			assert.Equal(t, tc.expectedRunsExplicitlySet, r.runsExplicitlySet)
		})
	}
}

func TestGroupTasksByParallelSupport(t *testing.T) {
	makeTask := func(name string, parallel bool) taskConfig {
		return taskConfig{
			path: name + ".yaml",
			spec: &task.TaskConfig{
				Metadata: task.TaskMetadata{
					Name:     name,
					Parallel: parallel,
				},
			},
		}
	}

	tests := map[string]struct {
		tasks             []taskConfig
		expectedSeqCount  int
		expectedParCount  int
		expectedGroupSize int // size of parallel group (0 if no parallel group)
	}{
		"empty tasks": {
			tasks:             []taskConfig{},
			expectedSeqCount:  0,
			expectedParCount:  0,
			expectedGroupSize: 0,
		},
		"all sequential": {
			tasks: []taskConfig{
				makeTask("a", false),
				makeTask("b", false),
				makeTask("c", false),
			},
			expectedSeqCount:  3,
			expectedParCount:  0,
			expectedGroupSize: 0,
		},
		"all parallel": {
			tasks: []taskConfig{
				makeTask("a", true),
				makeTask("b", true),
				makeTask("c", true),
			},
			expectedSeqCount:  0,
			expectedParCount:  1,
			expectedGroupSize: 3,
		},
		"mixed - sequential first then parallel": {
			tasks: []taskConfig{
				makeTask("a", false),
				makeTask("b", true),
				makeTask("c", true),
			},
			expectedSeqCount:  1,
			expectedParCount:  1,
			expectedGroupSize: 2,
		},
		"mixed - interleaved": {
			tasks: []taskConfig{
				makeTask("a", false),
				makeTask("b", true),
				makeTask("c", false),
				makeTask("d", true),
				makeTask("e", true),
			},
			expectedSeqCount:  2,
			expectedParCount:  1,
			expectedGroupSize: 3,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			groups := groupTasksByParallelSupport(tc.tasks)

			seqCount := 0
			parCount := 0
			parGroupSize := 0

			for _, g := range groups {
				if g.parallel {
					parCount++
					parGroupSize = len(g.tasks)
				} else {
					seqCount++
					assert.Len(t, g.tasks, 1, "sequential groups should have exactly 1 task")
				}
			}

			assert.Equal(t, tc.expectedSeqCount, seqCount, "sequential group count")
			assert.Equal(t, tc.expectedParCount, parCount, "parallel group count")
			assert.Equal(t, tc.expectedGroupSize, parGroupSize, "parallel group size")

			// Verify sequential tasks come before parallel
			if parCount > 0 && seqCount > 0 {
				lastGroup := groups[len(groups)-1]
				require.True(t, lastGroup.parallel, "parallel group should be last")
			}
		})
	}
}

func TestGetRunsForTask(t *testing.T) {
	makeTask := func(runs int) taskConfig {
		return taskConfig{
			path: "test.yaml",
			spec: &task.TaskConfig{
				Metadata: task.TaskMetadata{
					Name: "test-task",
					Runs: runs,
				},
			},
		}
	}

	tests := map[string]struct {
		runnerRuns        int
		runsExplicitlySet bool
		taskRuns          int
		expected          int
	}{
		"default - no CLI, no task runs": {
			runnerRuns:        1,
			runsExplicitlySet: false,
			taskRuns:          0,
			expected:          1,
		},
		"task has runs, CLI not set": {
			runnerRuns:        1,
			runsExplicitlySet: false,
			taskRuns:          3,
			expected:          3,
		},
		"CLI explicitly set, task has no runs": {
			runnerRuns:        5,
			runsExplicitlySet: true,
			taskRuns:          0,
			expected:          5,
		},
		"CLI explicitly set overrides task runs": {
			runnerRuns:        5,
			runsExplicitlySet: true,
			taskRuns:          3,
			expected:          5,
		},
		"CLI explicitly set to 1 overrides task runs": {
			runnerRuns:        1,
			runsExplicitlySet: true,
			taskRuns:          3,
			expected:          1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			runner := &evalRunner{
				runs:              tc.runnerRuns,
				runsExplicitlySet: tc.runsExplicitlySet,
			}
			task := makeTask(tc.taskRuns)

			result := runner.getRunsForTask(task)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCollectTaskConfigsDeduplication(t *testing.T) {
	tests := map[string]struct {
		taskSets      []TaskSet
		expectedCount int
	}{
		"duplicate paths from multiple globs": {
			taskSets: []TaskSet{
				{Glob: "../task/testdata/*.yaml"},
				{Glob: "../task/testdata/*.yaml"}, // same glob twice
			},
			expectedCount: 2, // 2 unique files, duplicates removed
		},
		"overlapping glob and explicit path": {
			taskSets: []TaskSet{
				{Glob: "../task/testdata/*.yaml"},
				{Path: "../task/testdata/create-pod-inline.yaml"}, // explicit path that matches glob
			},
			expectedCount: 2, // should deduplicate the overlapping one
		},
		"single task set": {
			taskSets: []TaskSet{
				{Glob: "../task/testdata/*.yaml"},
			},
			expectedCount: 2, // 2 task files in testdata
		},
		"triple duplicate same path": {
			taskSets: []TaskSet{
				{Path: "../task/testdata/create-pod-inline.yaml"},
				{Path: "../task/testdata/create-pod-inline.yaml"},
				{Path: "../task/testdata/create-pod-inline.yaml"},
			},
			expectedCount: 1, // should deduplicate to 1
		},
		"equivalent paths with dot prefix": {
			taskSets: []TaskSet{
				{Path: "../task/testdata/create-pod-inline.yaml"},
				{Path: "./../task/testdata/create-pod-inline.yaml"}, // same file, different path format
			},
			expectedCount: 1, // should deduplicate via canonical path
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			runner := &evalRunner{
				spec: &EvalSpec{
					Config: EvalConfig{
						TaskSets: tc.taskSets,
					},
				},
			}

			rx := regexp.MustCompile(".*")
			configs, err := runner.collectTaskConfigs(rx)
			require.NoError(t, err)
			assert.Len(t, configs, tc.expectedCount)
		})
	}
}

func TestCollectTaskConfigsAssertionSets(t *testing.T) {
	minCalls := 2
	maxCalls := 10

	runner := &evalRunner{
		spec: &EvalSpec{
			Config: EvalConfig{
				TaskSets: []TaskSet{
					{
						Path: "../task/testdata/create-pod-inline.yaml",
						Assertions: &TaskAssertions{
							ToolsUsed:    []ToolAssertion{{Server: "s1", Tool: "t1"}},
							MinToolCalls: &minCalls,
						},
					},
					{
						Path: "../task/testdata/create-pod-inline.yaml",
						Assertions: &TaskAssertions{
							ToolsUsed:        []ToolAssertion{{Server: "s2", Tool: "t2"}},
							NoDuplicateCalls: true,
							MaxToolCalls:     &maxCalls,
						},
					},
				},
			},
		},
	}

	rx := regexp.MustCompile(".*")
	configs, err := runner.collectTaskConfigs(rx)
	require.NoError(t, err)
	require.Len(t, configs, 1, "should deduplicate to single task")

	// Assertions should be kept as separate sets, not merged
	assertions := configs[0].assertions
	require.Len(t, assertions, 2, "should have 2 separate assertion sets")

	// First assertion set
	assert.Len(t, assertions[0].ToolsUsed, 1)
	assert.Equal(t, "s1", assertions[0].ToolsUsed[0].Server)
	assert.Equal(t, minCalls, *assertions[0].MinToolCalls)

	// Second assertion set
	assert.Len(t, assertions[1].ToolsUsed, 1)
	assert.Equal(t, "s2", assertions[1].ToolsUsed[0].Server)
	assert.True(t, assertions[1].NoDuplicateCalls)
	assert.Equal(t, maxCalls, *assertions[1].MaxToolCalls)
}

func TestCollectTaskConfigsNilAssertions(t *testing.T) {
	runner := &evalRunner{
		spec: &EvalSpec{
			Config: EvalConfig{
				TaskSets: []TaskSet{
					{Path: "../task/testdata/create-pod-inline.yaml"},
					{Path: "../task/testdata/create-pod-inline.yaml", Assertions: nil},
				},
			},
		},
	}

	rx := regexp.MustCompile(".*")
	configs, err := runner.collectTaskConfigs(rx)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Len(t, configs[0].assertions, 0, "nil assertions should not be added to slice")
}
