package mcpclient

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	basePath = "testdata"
)

func TestParseConfigFile(t *testing.T) {
	type serverTypes struct {
		isStdio bool
		isHttp  bool
	}

	tt := map[string]struct {
		file          string
		expected      *MCPConfig
		expectedTypes map[string]serverTypes
		expectErr     bool
	}{
		"basic": {
			file: "basic.json",
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"filesystem": {
						Command: "npx",
						Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
					},
				},
			},
			expectedTypes: map[string]serverTypes{
				"filesystem": {isStdio: true},
			},
		},
		"with-env": {
			file: "with-env.json",
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"postgres": {
						Command: "uvx",
						Args:    []string{"mcp-server-postgres"},
						Env: map[string]string{
							"POSTGRES_PASSWORD": "secret",
						},
					},
				},
			},
			expectedTypes: map[string]serverTypes{
				"postgres": {isStdio: true},
			},
		},
		"http-server": {
			file: "http-server.json",
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"api-server": {
						Type: "http",
						URL:  "https://api.example.com/mcp",
						Headers: map[string]string{
							"Authorization": "Bearer ",
						},
					},
				},
			},
			expectedTypes: map[string]serverTypes{
				"api-server": {isHttp: true},
			},
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			if tn == "http-server" {
				t.Setenv("API_BASE_URL", "")
				t.Setenv("API_KEY", "")
			}
			got, err := ParseConfigFile(fmt.Sprintf("%s/%s", basePath, tc.file))
			if tc.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestConfigFromEnv(t *testing.T) {
	// Helper to clear all MCP env vars
	clearEnv := func() {
		envVars := []string{
			EnvMcpURL, EnvMcpHost, EnvMcpPort, EnvMcpPath,
			EnvMcpCommand, EnvMcpArgs, EnvMcpEnv, EnvMcpServerName,
			EnvMcpHeaders, EnvMcpEnableAllTools,
		}
		for _, v := range envVars {
			os.Unsetenv(v)
		}
	}

	tests := map[string]struct {
		envVars     map[string]string
		expected    *MCPConfig
		expectErr   bool
		errContains string
	}{
		"no env vars returns nil without error": {
			envVars:  map[string]string{},
			expected: nil,
		},
		"MCP_URL creates HTTP server": {
			envVars: map[string]string{
				EnvMcpURL: "http://localhost:8080/mcp",
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"default": {
						Type:           TransportTypeHttp,
						URL:            "http://localhost:8080/mcp",
						EnableAllTools: true,
					},
				},
			},
		},
		"MCP_HOST + MCP_PORT creates HTTP server": {
			envVars: map[string]string{
				EnvMcpHost: "example.com",
				EnvMcpPort: "9090",
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"default": {
						Type:           TransportTypeHttp,
						URL:            "http://example.com:9090/mcp",
						EnableAllTools: true,
					},
				},
			},
		},
		"MCP_PORT only uses localhost default": {
			envVars: map[string]string{
				EnvMcpPort: "8080",
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"default": {
						Type:           TransportTypeHttp,
						URL:            "http://localhost:8080/mcp",
						EnableAllTools: true,
					},
				},
			},
		},
		"MCP_HOST only uses default path": {
			envVars: map[string]string{
				EnvMcpHost: "myserver.local",
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"default": {
						Type:           TransportTypeHttp,
						URL:            "http://myserver.local/mcp",
						EnableAllTools: true,
					},
				},
			},
		},
		"MCP_PATH customizes path": {
			envVars: map[string]string{
				EnvMcpHost: "localhost",
				EnvMcpPort: "8080",
				EnvMcpPath: "/api/v1/mcp",
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"default": {
						Type:           TransportTypeHttp,
						URL:            "http://localhost:8080/api/v1/mcp",
						EnableAllTools: true,
					},
				},
			},
		},
		"MCP_PATH without leading slash": {
			envVars: map[string]string{
				EnvMcpHost: "localhost",
				EnvMcpPort: "8080",
				EnvMcpPath: "custom/path",
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"default": {
						Type:           TransportTypeHttp,
						URL:            "http://localhost:8080/custom/path",
						EnableAllTools: true,
					},
				},
			},
		},
		"MCP_COMMAND creates stdio server": {
			envVars: map[string]string{
				EnvMcpCommand: "npx",
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"default": {
						Type:           TransportTypeStdio,
						Command:        "npx",
						EnableAllTools: true,
					},
				},
			},
		},
		"MCP_ARGS as comma-separated": {
			envVars: map[string]string{
				EnvMcpCommand: "npx",
				EnvMcpArgs:    "-y,@modelcontextprotocol/server-filesystem,/tmp",
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"default": {
						Type:           TransportTypeStdio,
						Command:        "npx",
						Args:           []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
						EnableAllTools: true,
					},
				},
			},
		},
		"MCP_ARGS as JSON array": {
			envVars: map[string]string{
				EnvMcpCommand: "npx",
				EnvMcpArgs:    `["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]`,
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"default": {
						Type:           TransportTypeStdio,
						Command:        "npx",
						Args:           []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
						EnableAllTools: true,
					},
				},
			},
		},
		"MCP_ENV parsed as JSON": {
			envVars: map[string]string{
				EnvMcpCommand: "npx",
				EnvMcpEnv:     `{"KUBECONFIG":"/path/to/config","DEBUG":"true"}`,
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"default": {
						Type:           TransportTypeStdio,
						Command:        "npx",
						Env:            map[string]string{"KUBECONFIG": "/path/to/config", "DEBUG": "true"},
						EnableAllTools: true,
					},
				},
			},
		},
		"invalid MCP_ENV returns error": {
			envVars: map[string]string{
				EnvMcpCommand: "npx",
				EnvMcpEnv:     "not-json",
			},
			expectErr:   true,
			errContains: "invalid MCP_ENV",
		},
		"MCP_HEADERS parsed as JSON": {
			envVars: map[string]string{
				EnvMcpURL:     "http://localhost:8080/mcp",
				EnvMcpHeaders: `{"Authorization":"Bearer token123","X-Custom":"value"}`,
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"default": {
						Type:           TransportTypeHttp,
						URL:            "http://localhost:8080/mcp",
						Headers:        map[string]string{"Authorization": "Bearer token123", "X-Custom": "value"},
						EnableAllTools: true,
					},
				},
			},
		},
		"invalid MCP_HEADERS returns error": {
			envVars: map[string]string{
				EnvMcpURL:     "http://localhost:8080/mcp",
				EnvMcpHeaders: "not-json",
			},
			expectErr:   true,
			errContains: "invalid MCP_HEADERS",
		},
		"MCP_SERVER_NAME customizes server name": {
			envVars: map[string]string{
				EnvMcpURL:        "http://localhost:8080/mcp",
				EnvMcpServerName: "kubernetes",
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"kubernetes": {
						Type:           TransportTypeHttp,
						URL:            "http://localhost:8080/mcp",
						EnableAllTools: true,
					},
				},
			},
		},
		"MCP_ENABLE_ALL_TOOLS false": {
			envVars: map[string]string{
				EnvMcpURL:            "http://localhost:8080/mcp",
				EnvMcpEnableAllTools: "false",
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"default": {
						Type:           TransportTypeHttp,
						URL:            "http://localhost:8080/mcp",
						EnableAllTools: false,
					},
				},
			},
		},
		"MCP_ENABLE_ALL_TOOLS 0": {
			envVars: map[string]string{
				EnvMcpURL:            "http://localhost:8080/mcp",
				EnvMcpEnableAllTools: "0",
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"default": {
						Type:           TransportTypeHttp,
						URL:            "http://localhost:8080/mcp",
						EnableAllTools: false,
					},
				},
			},
		},
		"MCP_ENABLE_ALL_TOOLS true explicitly": {
			envVars: map[string]string{
				EnvMcpURL:            "http://localhost:8080/mcp",
				EnvMcpEnableAllTools: "true",
			},
			expected: &MCPConfig{
				MCPServers: map[string]*ServerConfig{
					"default": {
						Type:           TransportTypeHttp,
						URL:            "http://localhost:8080/mcp",
						EnableAllTools: true,
					},
				},
			},
		},
		"invalid MCP_ARGS JSON returns error": {
			envVars: map[string]string{
				EnvMcpCommand: "npx",
				EnvMcpArgs:    `["invalid`,
			},
			expectErr:   true,
			errContains: "invalid MCP_ARGS",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			clearEnv()
			defer clearEnv()

			for k, v := range tc.envVars {
				os.Setenv(k, v)
			}

			config, err := ConfigFromEnv()

			if tc.expectErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expected, config)
		})
	}
}

func TestParseArgs(t *testing.T) {
	tests := map[string]struct {
		input     string
		expected  []string
		expectErr bool
	}{
		"empty string": {
			input:    "",
			expected: []string{},
		},
		"single arg": {
			input:    "arg1",
			expected: []string{"arg1"},
		},
		"comma-separated": {
			input:    "arg1,arg2,arg3",
			expected: []string{"arg1", "arg2", "arg3"},
		},
		"comma-separated with spaces": {
			input:    " arg1 , arg2 , arg3 ",
			expected: []string{"arg1", "arg2", "arg3"},
		},
		"JSON array": {
			input:    `["arg1", "arg2", "arg3"]`,
			expected: []string{"arg1", "arg2", "arg3"},
		},
		"JSON array with special chars": {
			input:    `["-y", "@scope/pkg", "/path/to/dir"]`,
			expected: []string{"-y", "@scope/pkg", "/path/to/dir"},
		},
		"invalid JSON array": {
			input:     `["invalid`,
			expectErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := parseArgs(tc.input)

			if tc.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
