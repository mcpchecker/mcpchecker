package mcpclient

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"sigs.k8s.io/yaml"
)

const (
	TransportTypeHttp  = "http"
	TransportTypeStdio = "stdio"
)

// MCPConfig represents the top-level MCP configuration file structure
// used by Claude Code, Cursor, and other MCP clients.
type MCPConfig struct {
	MCPServers map[string]*ServerConfig `json:"mcpServers" yaml:"mcpServers"`
}

// ServerConfig represents the configuration for a single MCP server.
// Supports both stdio (command-based) and HTTP-based servers.
type ServerConfig struct {
	// Type specifies the server type: "stdio" or "http"
	// If not specified, will be inferred from URL (http) or Command (stdio)
	Type string `json:"type,omitempty"`

	// Command is the executable to run (e.g., "node", "python", "npx")
	// Used for stdio servers
	Command string `json:"command,omitempty"`

	// Args are the command-line arguments to pass to the command
	// Used for stdio servers
	Args []string `json:"args,omitempty"`

	// Env contains environment variables to set for the server process
	// Used for stdio servers
	Env map[string]string `json:"env,omitempty"`

	// URL is the HTTP endpoint for the MCP server
	// Used for http servers. May contain environment variable references
	// like ${VAR} or ${VAR:-default}
	URL string `json:"url,omitempty"`

	// Headers are HTTP headers to send with requests
	// Used for http servers. Values may contain environment variable references
	Headers map[string]string `json:"headers,omitempty"`

	// Auth configures OAuth client credentials for HTTP servers.
	Auth *AuthConfig `json:"auth,omitempty" yaml:"auth,omitempty"`

	// Disabled indicates whether this server should be skipped
	Disabled bool `json:"disabled,omitempty"`

	// AlwaysAllow is a list of tools/resources that should always be allowed
	AlwaysAllow []string `json:"alwaysAllow,omitempty"`

	// EnableAllTools sets all tools to be allowed
	EnableAllTools bool `json:"enableAllTools"`
}

// AuthConfig holds OAuth client-credentials settings for HTTP MCP servers.
// Field values may use ${ENV} or ${ENV:-default} expansion.
type AuthConfig struct {
	Type         string `json:"type,omitempty" yaml:"type,omitempty"`
	TokenURL     string `json:"tokenUrl,omitempty" yaml:"tokenUrl,omitempty"`
	ClientID     string `json:"clientId,omitempty" yaml:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty" yaml:"clientSecret,omitempty"`
	Scope        string `json:"scope,omitempty" yaml:"scope,omitempty"`
	// Resource is the RFC 8707 resource indicator. Empty means the server URL.
	Resource string `json:"resource,omitempty" yaml:"resource,omitempty"`
}

// ParseConfigFile reads and parses an MCP config file from the given path.
// The file can be in JSON or YAML format.
func ParseConfigFile(path string) (*MCPConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return ParseConfig(data)
}

// ParseConfig parses MCP config data from bytes.
// The data can be in JSON or YAML format.
func ParseConfig(data []byte) (*MCPConfig, error) {
	var config MCPConfig

	// sigs.k8s.io/yaml can handle both JSON and YAML
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if err := config.Resolve(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

// validateConfig validates the parsed configuration.
func validateConfig(config *MCPConfig) error {
	if config.MCPServers == nil {
		return fmt.Errorf("mcpServers field is required")
	}

	for name, server := range config.MCPServers {
		if server.Auth != nil {
			if !server.IsHttp() {
				return fmt.Errorf("server %q: auth is only supported for http servers", name)
			}
			if server.Auth.Type != "client_credentials" {
				return fmt.Errorf("server %q: unsupported auth type %q", name, server.Auth.Type)
			}
		}
		if server.IsHttp() {
			if server.URL == "" {
				return fmt.Errorf("server %q: url is required for http servers", name)
			}
		} else if server.IsStdio() {
			if server.Command == "" {
				return fmt.Errorf("server %q: command is required for stdio servers", name)
			}
		} else {
			return fmt.Errorf("server %q: must specify either command or url", name)
		}
	}

	return nil
}

// GetEnabledServers returns a map of server names to their configurations,
// excluding any servers marked as disabled.
func (c *MCPConfig) GetEnabledServers() map[string]*ServerConfig {
	enabled := make(map[string]*ServerConfig)
	for name, server := range c.MCPServers {
		if !server.Disabled {
			enabled[name] = server
		}
	}
	return enabled
}

// GetServer returns the configuration for a specific server by name.
func (c *MCPConfig) GetServer(name string) (*ServerConfig, bool) {
	server, ok := c.MCPServers[name]
	return server, ok
}

// ToFile writes the configuration to the specified path
func (c *MCPConfig) ToFile(path string) error {
	bytes, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal MCPConfig to bytes: %w", err)
	}

	err = os.WriteFile(path, bytes, 0644)
	if err != nil {
		return fmt.Errorf("failed to write MCPConfig to file at path '%s': %w", path, err)
	}

	return nil
}

// IsStdio returns true if this is a stdio-based (command) server.
func (s *ServerConfig) IsStdio() bool {
	if s.Type == "stdio" {
		return true
	}
	if s.Type == "http" {
		return false
	}
	// Type not specified - infer from fields
	return s.Command != ""
}

// IsHttp returns true if this is an HTTP-based server.
func (s *ServerConfig) IsHttp() bool {
	if s.Type == "http" {
		return true
	}
	if s.Type == "stdio" {
		return false
	}
	// Type not specified - infer from fields
	return s.URL != ""
}

// Environment variable names for MCP configuration
const (
	EnvMcpURL            = "MCP_URL"
	EnvMcpHost           = "MCP_HOST"
	EnvMcpPort           = "MCP_PORT"
	EnvMcpPath           = "MCP_PATH"
	EnvMcpCommand        = "MCP_COMMAND"
	EnvMcpArgs           = "MCP_ARGS"
	EnvMcpEnv            = "MCP_ENV"
	EnvMcpServerName     = "MCP_SERVER_NAME"
	EnvMcpHeaders        = "MCP_HEADERS"
	EnvMcpEnableAllTools = "MCP_ENABLE_ALL_TOOLS"
)

// ConfigFromEnv builds MCPConfig from environment variables.
// Returns nil, nil if no MCP env vars are set (not an error).
// Returns config, nil if valid configuration was built from env vars.
// Returns nil, error if env vars are set but invalid.
func ConfigFromEnv() (*MCPConfig, error) {
	mcpURL := os.Getenv(EnvMcpURL)
	mcpHost := os.Getenv(EnvMcpHost)
	mcpPort := os.Getenv(EnvMcpPort)
	mcpCommand := os.Getenv(EnvMcpCommand)

	// Check if any MCP env vars are set
	if mcpURL == "" && mcpHost == "" && mcpPort == "" && mcpCommand == "" {
		return nil, nil
	}

	var server *ServerConfig
	var err error

	// Determine server type based on which env vars are set
	if mcpCommand != "" {
		server, err = buildStdioServerFromEnv()
		if err != nil {
			return nil, err
		}
	} else {
		server, err = buildHTTPServerFromEnv()
		if err != nil {
			return nil, err
		}
	}

	// Get server name (default to "default")
	serverName := os.Getenv(EnvMcpServerName)
	if serverName == "" {
		serverName = "default"
	}

	config := &MCPConfig{
		MCPServers: map[string]*ServerConfig{
			serverName: server,
		},
	}

	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config from environment: %w", err)
	}

	return config, nil
}

// buildHTTPServerFromEnv builds an HTTP ServerConfig from environment variables.
func buildHTTPServerFromEnv() (*ServerConfig, error) {
	mcpURL := os.Getenv(EnvMcpURL)
	mcpHost := os.Getenv(EnvMcpHost)
	mcpPort := os.Getenv(EnvMcpPort)
	mcpPath := os.Getenv(EnvMcpPath)

	var serverURL string

	if mcpURL != "" {
		// Use full URL if provided
		serverURL = mcpURL
	} else {
		// Build URL from components
		if mcpHost == "" {
			mcpHost = "localhost"
		}
		if mcpPath == "" {
			mcpPath = "/mcp"
		}
		// Ensure path starts with /
		if !strings.HasPrefix(mcpPath, "/") {
			mcpPath = "/" + mcpPath
		}

		if mcpPort != "" {
			serverURL = fmt.Sprintf("http://%s:%s%s", mcpHost, mcpPort, mcpPath)
		} else {
			serverURL = fmt.Sprintf("http://%s%s", mcpHost, mcpPath)
		}
	}

	// Validate URL
	if _, err := url.Parse(serverURL); err != nil {
		return nil, fmt.Errorf("invalid MCP URL: %w", err)
	}

	server := &ServerConfig{
		Type: TransportTypeHttp,
		URL:  serverURL,
	}

	// Parse headers if provided
	headersJSON := os.Getenv(EnvMcpHeaders)
	if headersJSON != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(headersJSON), &headers); err != nil {
			return nil, fmt.Errorf("invalid %s: must be valid JSON object: %w", EnvMcpHeaders, err)
		}
		server.Headers = headers
	}

	// Parse enableAllTools (default to true)
	server.EnableAllTools = parseEnableAllTools()

	return server, nil
}

// buildStdioServerFromEnv builds a stdio ServerConfig from environment variables.
func buildStdioServerFromEnv() (*ServerConfig, error) {
	mcpCommand := os.Getenv(EnvMcpCommand)
	mcpArgs := os.Getenv(EnvMcpArgs)

	server := &ServerConfig{
		Type:    TransportTypeStdio,
		Command: mcpCommand,
	}

	// Parse args - support both JSON array and comma-separated
	if mcpArgs != "" {
		args, err := parseArgs(mcpArgs)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", EnvMcpArgs, err)
		}
		server.Args = args
	}

	// Parse env vars if provided
	envJSON := os.Getenv(EnvMcpEnv)
	if envJSON != "" {
		var env map[string]string
		if err := json.Unmarshal([]byte(envJSON), &env); err != nil {
			return nil, fmt.Errorf("invalid %s: must be valid JSON object: %w", EnvMcpEnv, err)
		}
		server.Env = env
	}

	// Parse enableAllTools (default to true)
	server.EnableAllTools = parseEnableAllTools()

	return server, nil
}

// parseArgs parses MCP_ARGS which can be either a JSON array or comma-separated string.
func parseArgs(argsStr string) ([]string, error) {
	argsStr = strings.TrimSpace(argsStr)

	// Try JSON array first
	if strings.HasPrefix(argsStr, "[") {
		var args []string
		if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
			return nil, fmt.Errorf("invalid JSON array: %w", err)
		}
		return args, nil
	}

	// Fall back to comma-separated
	parts := strings.Split(argsStr, ",")
	args := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			args = append(args, trimmed)
		}
	}
	return args, nil
}

// parseEnableAllTools parses MCP_ENABLE_ALL_TOOLS, defaulting to true.
func parseEnableAllTools() bool {
	val := os.Getenv(EnvMcpEnableAllTools)
	if val == "" {
		return true // default to true
	}
	// Only false if explicitly set to "false" or "0"
	lower := strings.ToLower(val)
	return lower != "false" && lower != "0"
}
