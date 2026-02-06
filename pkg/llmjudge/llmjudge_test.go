package llmjudge

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLLMJudge(t *testing.T) {
	tests := map[string]struct {
		config      *LLMJudgeEvalConfig
		envVars     map[string]string
		expectedErr string
		skipReason  string
	}{
		"nil config returns noop judge": {
			config:     nil,
			envVars:    map[string]string{},
			skipReason: "",
		},
		"nil env config returns error": {
			config: &LLMJudgeEvalConfig{
				Env: nil,
			},
			envVars:     map[string]string{},
			expectedErr: "llm judge env config is required",
		},
		"openai judge - missing API key returns error": {
			config: &LLMJudgeEvalConfig{
				Env: &LLMJudgeEnvConfig{
					TypeKey:      "JUDGE_TYPE",
					BaseUrlKey:   "JUDGE_BASE_URL",
					ApiKeyKey:    "JUDGE_API_KEY",
					ModelNameKey: "JUDGE_MODEL_NAME",
				},
			},
			envVars: map[string]string{
				"JUDGE_TYPE":       "openai",
				"JUDGE_BASE_URL":   "https://api.openai.com/v1",
				"JUDGE_MODEL_NAME": "gpt-4o",
			},
			expectedErr: "missing required environment variables for OpenAI judge",
		},
		"openai judge - missing model name returns error": {
			config: &LLMJudgeEvalConfig{
				Env: &LLMJudgeEnvConfig{
					TypeKey:      "JUDGE_TYPE",
					BaseUrlKey:   "JUDGE_BASE_URL",
					ApiKeyKey:    "JUDGE_API_KEY",
					ModelNameKey: "JUDGE_MODEL_NAME",
				},
			},
			envVars: map[string]string{
				"JUDGE_TYPE":     "openai",
				"JUDGE_BASE_URL": "https://api.openai.com/v1",
				"JUDGE_API_KEY":  "sk-test",
			},
			expectedErr: "missing required environment variables for OpenAI judge",
		},
		"openai judge requires base URL": {
			config: &LLMJudgeEvalConfig{
				Env: &LLMJudgeEnvConfig{
					TypeKey:      "JUDGE_TYPE",
					BaseUrlKey:   "JUDGE_BASE_URL",
					ApiKeyKey:    "JUDGE_API_KEY",
					ModelNameKey: "JUDGE_MODEL_NAME",
				},
			},
			envVars: map[string]string{
				"JUDGE_TYPE":       "openai",
				"JUDGE_API_KEY":    "sk-test",
				"JUDGE_MODEL_NAME": "gpt-4o",
			},
			expectedErr: "missing required environment variable",
		},
		"unsupported judge type returns error": {
			config: &LLMJudgeEvalConfig{
				Env: &LLMJudgeEnvConfig{
					TypeKey:      "JUDGE_TYPE",
					ApiKeyKey:    "JUDGE_API_KEY",
					ModelNameKey: "JUDGE_MODEL_NAME",
				},
			},
			envVars: map[string]string{
				"JUDGE_TYPE":       "unsupported",
				"JUDGE_API_KEY":    "test",
				"JUDGE_MODEL_NAME": "test",
			},
			expectedErr: "unsupported judge type",
		},
		"claude judge - succeeds without API key or model": {
			config: &LLMJudgeEvalConfig{
				Env: &LLMJudgeEnvConfig{
					TypeKey: "JUDGE_TYPE",
				},
			},
			envVars: map[string]string{
				"JUDGE_TYPE": "claude",
			},
			skipReason: "requires claude binary in PATH",
		},
		"claude judge - succeeds with API key and model (ignored)": {
			config: &LLMJudgeEvalConfig{
				Env: &LLMJudgeEnvConfig{
					TypeKey:      "JUDGE_TYPE",
					ApiKeyKey:    "JUDGE_API_KEY",
					ModelNameKey: "JUDGE_MODEL_NAME",
				},
			},
			envVars: map[string]string{
				"JUDGE_TYPE":       "claude",
				"JUDGE_API_KEY":    "unused",
				"JUDGE_MODEL_NAME": "unused",
			},
			skipReason: "requires claude binary in PATH",
		},
		"defaults to openai when type not specified": {
			config: &LLMJudgeEvalConfig{
				Env: &LLMJudgeEnvConfig{
					BaseUrlKey:   "JUDGE_BASE_URL",
					ApiKeyKey:    "JUDGE_API_KEY",
					ModelNameKey: "JUDGE_MODEL_NAME",
				},
			},
			envVars: map[string]string{
				"JUDGE_BASE_URL":   "https://api.openai.com/v1",
				"JUDGE_API_KEY":    "sk-test",
				"JUDGE_MODEL_NAME": "gpt-4o",
			},
			expectedErr: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.skipReason != "" {
				// Check if claude binary exists
				if _, err := exec.LookPath("claude"); err != nil {
					t.Skip(tc.skipReason)
				}
			}

			// Set environment variables
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}

			judge, err := NewLLMJudge(tc.config)

			if tc.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, judge)
		})
	}
}

func TestLLMJudgeEvalConfig_Type(t *testing.T) {
	tests := map[string]struct {
		config   *LLMJudgeEvalConfig
		envVars  map[string]string
		expected string
	}{
		"returns openai when typeKey not set": {
			config: &LLMJudgeEvalConfig{
				Env: &LLMJudgeEnvConfig{},
			},
			envVars:  map[string]string{},
			expected: JudgeTypeOpenAI,
		},
		"returns openai when env var not set": {
			config: &LLMJudgeEvalConfig{
				Env: &LLMJudgeEnvConfig{
					TypeKey: "JUDGE_TYPE",
				},
			},
			envVars:  map[string]string{},
			expected: JudgeTypeOpenAI,
		},
		"returns claude when env var set to claude": {
			config: &LLMJudgeEvalConfig{
				Env: &LLMJudgeEnvConfig{
					TypeKey: "JUDGE_TYPE",
				},
			},
			envVars: map[string]string{
				"JUDGE_TYPE": "claude",
			},
			expected: JudgeTypeClaude,
		},
		"returns openai when env var set to openai": {
			config: &LLMJudgeEvalConfig{
				Env: &LLMJudgeEnvConfig{
					TypeKey: "JUDGE_TYPE",
				},
			},
			envVars: map[string]string{
				"JUDGE_TYPE": "openai",
			},
			expected: JudgeTypeOpenAI,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Set environment variables
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}

			got := tc.config.Type()
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestClaudeJudge_ModelName(t *testing.T) {
	judge := &claudeJudge{}
	assert.Equal(t, "claude-code-cli", judge.ModelName())
}

func TestNoopLLMJudge(t *testing.T) {
	judge := &noopLLMJudge{}

	t.Run("always passes", func(t *testing.T) {
		result, err := judge.EvaluateText(context.Background(), &LLMJudgeStepConfig{}, "prompt", "output")
		require.NoError(t, err)
		assert.True(t, result.Passed)
		assert.Equal(t, "noop judge always passes", result.Reason)
		assert.Equal(t, "n/a", result.FailureCategory)
	})

	t.Run("model name is noop", func(t *testing.T) {
		assert.Equal(t, "noop", judge.ModelName())
	})
}

// mockClaudeCommand creates a temporary mock script that simulates claude CLI output
func mockClaudeCommand(t *testing.T, output string, exitCode int) string {
	t.Helper()

	tmpDir := t.TempDir()
	mockScript := tmpDir + "/claude"

	scriptContent := "#!/bin/bash\n"
	if exitCode != 0 {
		scriptContent += "exit " + string(rune(exitCode)) + "\n"
	} else {
		scriptContent += "cat << 'EOF'\n" + output + "\nEOF\n"
	}

	err := os.WriteFile(mockScript, []byte(scriptContent), 0755)
	require.NoError(t, err)

	return tmpDir
}

func TestClaudeJudge_EvaluateText(t *testing.T) {
	// Skip if claude binary doesn't exist
	originalPath := os.Getenv("PATH")
	defer func() {
		os.Setenv("PATH", originalPath)
	}()

	tests := map[string]struct {
		claudeOutput string
		exitCode     int
		expectedErr  string
		validate     func(t *testing.T, result *LLMJudgeResult)
	}{
		"valid JSON response - passed": {
			claudeOutput: `{
  "passed": true,
  "reason": "The response contains all expected information",
  "failureCategory": "n/a"
}`,
			exitCode: 0,
			validate: func(t *testing.T, result *LLMJudgeResult) {
				assert.True(t, result.Passed)
				assert.Equal(t, "The response contains all expected information", result.Reason)
				assert.Equal(t, "n/a", result.FailureCategory)
			},
		},
		"valid JSON response - failed": {
			claudeOutput: `{
  "passed": false,
  "reason": "Missing critical information",
  "failureCategory": "missing_information"
}`,
			exitCode: 0,
			validate: func(t *testing.T, result *LLMJudgeResult) {
				assert.False(t, result.Passed)
				assert.Equal(t, "Missing critical information", result.Reason)
				assert.Equal(t, "missing_information", result.FailureCategory)
			},
		},
		"JSON with surrounding text": {
			claudeOutput: `Here is my evaluation:
{
  "passed": true,
  "reason": "Content matches",
  "failureCategory": "n/a"
}
Thank you!`,
			exitCode: 0,
			validate: func(t *testing.T, result *LLMJudgeResult) {
				assert.True(t, result.Passed)
				assert.Equal(t, "Content matches", result.Reason)
			},
		},
		"invalid JSON": {
			claudeOutput: `This is not JSON at all`,
			exitCode:     0,
			expectedErr:  "no valid JSON found",
		},
		"command execution error": {
			claudeOutput: "",
			exitCode:     1,
			expectedErr:  "failed to execute claude CLI",
		},
		"malformed JSON - incomplete": {
			claudeOutput: `{"passed": true, "reason": "test"`,
			exitCode:     0,
			expectedErr:  "no valid JSON found",
		},
		"malformed JSON - invalid structure": {
			claudeOutput: `{"passed": "not a boolean", "reason": "test", "failureCategory": "n/a"}`,
			exitCode:     0,
			expectedErr:  "failed to unmarshal",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create mock claude command
			mockDir := mockClaudeCommand(t, tc.claudeOutput, tc.exitCode)
			os.Setenv("PATH", mockDir+":"+originalPath)

			judge := &claudeJudge{}
			config := &LLMJudgeStepConfig{
				Contains: "expected content",
			}

			result, err := judge.EvaluateText(context.Background(), config, "test prompt", "test output")

			if tc.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, result)
			if tc.validate != nil {
				tc.validate(t, result)
			}
		})
	}
}

func TestNewClaudeJudge(t *testing.T) {
	t.Run("fails when claude binary not found", func(t *testing.T) {
		// Temporarily set PATH to empty to ensure claude is not found
		originalPath := os.Getenv("PATH")
		defer os.Setenv("PATH", originalPath)
		os.Setenv("PATH", "")

		cfg := &LLMJudgeEvalConfig{
			Env: &LLMJudgeEnvConfig{},
		}

		judge, err := newClaudeJudge(cfg)
		require.Error(t, err)
		assert.Nil(t, judge)
		assert.Contains(t, err.Error(), "'claude' binary not found in PATH")
	})

	t.Run("succeeds when claude binary exists", func(t *testing.T) {
		// Check if claude binary exists
		if _, err := exec.LookPath("claude"); err != nil {
			t.Skip("claude binary not found in PATH")
		}

		cfg := &LLMJudgeEvalConfig{
			Env: &LLMJudgeEnvConfig{},
		}

		judge, err := newClaudeJudge(cfg)
		require.NoError(t, err)
		assert.NotNil(t, judge)
		assert.Equal(t, "claude-code-cli", judge.ModelName())
	})
}
