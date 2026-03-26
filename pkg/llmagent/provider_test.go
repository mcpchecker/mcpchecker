package llmagent

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveProvider(t *testing.T) {
	tests := map[string]struct {
		provider    string
		expectErr   bool
		errContains string
	}{
		"unsupported provider returns error": {
			provider:    "invalid-provider",
			expectErr:   true,
			errContains: `unsupported provider "invalid-provider"`,
		},
		"error lists supported providers": {
			provider:    "unknown",
			expectErr:   true,
			errContains: "anthropic",
		},
		"error lists google as supported provider": {
			provider:    "unknown",
			expectErr:   true,
			errContains: "google",
		},
		"empty provider name": {
			provider:    "",
			expectErr:   true,
			errContains: "unsupported provider",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ResolveProvider(tc.provider)

			if tc.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
				return
			}

			require.NoError(t, err)
		})
	}
}

// clearProviderEnv unsets all provider-related environment variables.
func clearProviderEnv() {
	envVars := []string{
		anthropicApiKeyEnvVar,
		anthropicUseVertexEnvVar,
		geminiApiKeyEnvVar,
		geminiUseVertexEnvVar,
		googleApiKeyEnvVar,
		googleUseVertexEnvVar,
		googleCloudProjectEnvVar,
		googleCloudLocationEnvVar,
		openaiApiKeyEnvVar,
		openaiBaseUrlEnvVar,
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}
}

func TestAnthropicProviderBuilder(t *testing.T) {
	tests := map[string]struct {
		setupEnv    func()
		expectErr   bool
		errContains string
	}{
		"missing API key without vertex": {
			setupEnv:    func() {},
			expectErr:   true,
			errContains: anthropicApiKeyEnvVar,
		},
		"vertex enabled missing project": {
			setupEnv: func() {
				os.Setenv(anthropicUseVertexEnvVar, "1")
				os.Setenv(googleCloudLocationEnvVar, "us-central1")
			},
			expectErr:   true,
			errContains: googleCloudProjectEnvVar,
		},
		"vertex enabled missing location": {
			setupEnv: func() {
				os.Setenv(anthropicUseVertexEnvVar, "1")
				os.Setenv(googleCloudProjectEnvVar, "my-project")
			},
			expectErr:   true,
			errContains: googleCloudLocationEnvVar,
		},
		"vertex enabled with all env vars succeeds": {
			setupEnv: func() {
				os.Setenv(anthropicUseVertexEnvVar, "1")
				os.Setenv(googleCloudProjectEnvVar, "my-project")
				os.Setenv(googleCloudLocationEnvVar, "us-central1")
			},
		},
		"API key set succeeds": {
			setupEnv: func() {
				os.Setenv(anthropicApiKeyEnvVar, "sk-ant-test-key")
			},
		},
		"vertex not enabled treats as API key mode": {
			setupEnv: func() {
				os.Setenv(anthropicUseVertexEnvVar, "0")
			},
			expectErr:   true,
			errContains: anthropicApiKeyEnvVar,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			clearProviderEnv()
			defer clearProviderEnv()
			tc.setupEnv()

			builder := &anthropicProviderBuilder{}
			provider, err := builder.Build()

			if tc.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, provider)
		})
	}
}

func TestGoogleProviderBuilder(t *testing.T) {
	tests := map[string]struct {
		setupEnv    func()
		expectErr   bool
		errContains string
	}{
		"missing API key without vertex": {
			setupEnv:    func() {},
			expectErr:   true,
			errContains: geminiApiKeyEnvVar,
		},
		"vertex enabled missing project": {
			setupEnv: func() {
				os.Setenv(geminiUseVertexEnvVar, "1")
				os.Setenv(googleCloudLocationEnvVar, "us-central1")
			},
			expectErr:   true,
			errContains: googleCloudProjectEnvVar,
		},
		"vertex enabled missing location": {
			setupEnv: func() {
				os.Setenv(geminiUseVertexEnvVar, "1")
				os.Setenv(googleCloudProjectEnvVar, "my-project")
			},
			expectErr:   true,
			errContains: googleCloudLocationEnvVar,
		},
		"vertex enabled with all env vars succeeds": {
			setupEnv: func() {
				os.Setenv(geminiUseVertexEnvVar, "1")
				os.Setenv(googleCloudProjectEnvVar, "my-project")
				os.Setenv(googleCloudLocationEnvVar, "us-central1")
			},
		},
		"API key set succeeds": {
			setupEnv: func() {
				os.Setenv(geminiApiKeyEnvVar, "test-gemini-key")
			},
		},
		"GOOGLE_API_KEY used as fallback": {
			setupEnv: func() {
				os.Setenv(googleApiKeyEnvVar, "test-google-key")
			},
		},
		"GEMINI_API_KEY takes precedence over GOOGLE_API_KEY": {
			setupEnv: func() {
				os.Setenv(geminiApiKeyEnvVar, "test-gemini-key")
				os.Setenv(googleApiKeyEnvVar, "test-google-key")
			},
		},
		"GOOGLE_USE_VERTEX enables vertex": {
			setupEnv: func() {
				os.Setenv(googleUseVertexEnvVar, "1")
				os.Setenv(googleCloudProjectEnvVar, "my-project")
				os.Setenv(googleCloudLocationEnvVar, "us-central1")
			},
		},
		"GOOGLE_USE_VERTEX missing project": {
			setupEnv: func() {
				os.Setenv(googleUseVertexEnvVar, "1")
				os.Setenv(googleCloudLocationEnvVar, "us-central1")
			},
			expectErr:   true,
			errContains: googleCloudProjectEnvVar,
		},
		"GOOGLE_USE_VERTEX missing location": {
			setupEnv: func() {
				os.Setenv(googleUseVertexEnvVar, "1")
				os.Setenv(googleCloudProjectEnvVar, "my-project")
			},
			expectErr:   true,
			errContains: googleCloudLocationEnvVar,
		},
		"GOOGLE_USE_VERTEX disabled treats as API key mode": {
			setupEnv: func() {
				os.Setenv(googleUseVertexEnvVar, "0")
			},
			expectErr:   true,
			errContains: googleApiKeyEnvVar,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			clearProviderEnv()
			defer clearProviderEnv()
			tc.setupEnv()

			builder := &googleProviderBuilder{providerName: googleProviderKey}
			provider, err := builder.Build()

			if tc.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, provider)
		})
	}
}

func TestOpenAIProviderBuilder(t *testing.T) {
	tests := map[string]struct {
		setupEnv func()
	}{
		"no env vars still succeeds": {
			setupEnv: func() {},
		},
		"with API key": {
			setupEnv: func() {
				os.Setenv(openaiApiKeyEnvVar, "sk-test-key")
			},
		},
		"with base URL": {
			setupEnv: func() {
				os.Setenv(openaiBaseUrlEnvVar, "https://custom-endpoint/v1")
			},
		},
		"with API key and base URL": {
			setupEnv: func() {
				os.Setenv(openaiApiKeyEnvVar, "sk-test-key")
				os.Setenv(openaiBaseUrlEnvVar, "https://custom-endpoint/v1")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			clearProviderEnv()
			defer clearProviderEnv()
			tc.setupEnv()

			builder := &openaiProviderBuilder{}
			provider, err := builder.Build()

			require.NoError(t, err)
			assert.NotNil(t, provider)
		})
	}
}
