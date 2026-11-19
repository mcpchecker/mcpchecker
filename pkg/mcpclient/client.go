package mcpclient

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"

	"github.com/mcpchecker/mcpchecker/pkg/mcpapps"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Client struct {
	*mcp.ClientSession
	cfg      *ServerConfig
	appsHost bool
}

// ConnectOption configures an MCP client connection.
type ConnectOption func(*mcp.ClientOptions)

// WithInputRequiredPassthrough returns input-required tool, prompt, and resource
// results to the caller instead of fulfilling them through SDK input handlers.
// Use it for upstream connections whose results are forwarded to another client.
func WithInputRequiredPassthrough() ConnectOption {
	return func(opts *mcp.ClientOptions) {
		opts.MultiRoundTrip = &mcp.MultiRoundTripOptions{Disabled: true}
	}
}

func Connect(ctx context.Context, cfg *ServerConfig, options ...ConnectOption) (*Client, error) {
	if err := cfg.Resolve(); err != nil {
		return nil, err
	}
	var transport mcp.Transport
	if cfg.IsHttp() {
		httpClient, err := NewHTTPClient(cfg)
		if err != nil {
			return nil, err
		}

		transport = &mcp.StreamableClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpClient,
		}
	} else {
		cmd := exec.Command(cfg.Command, cfg.Args...)
		cmd.Env = buildEnv(cfg.Env)
		transport = &mcp.CommandTransport{Command: cmd}
	}

	// TODO: revisit the client options, we probably want to leverage many
	opts := &mcp.ClientOptions{}
	if mcpapps.HostEnabled(ctx) {
		opts.Capabilities = mcpapps.Capabilities()
	}
	for _, option := range options {
		option(opts)
	}
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "mcpchecker-client",
		Version: "0.0.0",
	}, opts)

	cs, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}

	return &Client{
		ClientSession: cs,
		cfg:           cfg,
		appsHost:      mcpapps.HostEnabled(ctx),
	}, nil
}

func (c *Client) GetAllowedTools(ctx context.Context) ([]*mcp.Tool, error) {
	allowed := []*mcp.Tool{}
	for t, err := range c.Tools(ctx, &mcp.ListToolsParams{}) {
		if err != nil {
			return nil, err
		}

		if c.appsHost && !mcpapps.ModelVisible(t) {
			continue
		}
		if c.cfg.EnableAllTools {
			allowed = append(allowed, t)
		} else if slices.Contains(c.cfg.AlwaysAllow, t.Name) {
			allowed = append(allowed, t)
		}
	}

	return allowed, nil
}

func (c *Client) GetConfig() *ServerConfig {
	return c.cfg
}

func buildEnv(env map[string]string) []string {
	full := os.Environ()
	for k, v := range env {
		full = append(full, fmt.Sprintf("%s=%s", k, v))
	}

	return full
}
