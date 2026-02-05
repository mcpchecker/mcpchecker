package acpclient

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"sync"

	"github.com/coder/acp-go-sdk"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
)

// RunResult contains the results of running a prompt, including session updates
// and any usage data reported by the agent.
type RunResult struct {
	Updates []acp.SessionUpdate
	Usage   *Usage // Actual token usage from agent (nil if not reported)
}

type Client interface {
	// Start starts the agent process and initializes the ACP connection
	Start(ctx context.Context) error
	// Run starts a new ACP session and runs the prompt to completion. Must be called after Start
	Run(ctx context.Context, prompt string, servers mcpproxy.ServerManager) ([]acp.SessionUpdate, error)
	// RunWithUsage is like Run but also returns usage data if the agent reports it.
	RunWithUsage(ctx context.Context, prompt string, servers mcpproxy.ServerManager) (*RunResult, error)
	// Close closes the client
	Close(ctx context.Context) error
}

func NewClient(ctx context.Context, cfg *AcpConfig) Client {
	return &client{
		cfg:      cfg,
		sessions: make(map[acp.SessionId]*session),
	}
}

type client struct {
	cfg      *AcpConfig
	mu       sync.RWMutex
	cmd      *exec.Cmd
	conn     *acp.ClientSideConnection
	sessions map[acp.SessionId]*session
}

func (c *client) Start(ctx context.Context) error {
	stdin, stdout, err := c.startPipes(ctx)
	if err != nil {
		return err
	}

	c.conn = acp.NewClientSideConnection(c, stdin, stdout)

	initResp, err := c.conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs:       acp.FileSystemCapability{ReadTextFile: false, WriteTextFile: false},
			Terminal: false,
		},
	})
	if err != nil {
		_ = c.Close(ctx)
		return fmt.Errorf("failed to initialize connection to acp agent: %w", err)
	}

	if !initResp.AgentCapabilities.McpCapabilities.Http {
		_ = c.Close(ctx)
		return fmt.Errorf("invalid acp agent: mcpchecker requires acp agents support http mcp transport")
	}

	return nil
}

func (c *client) Run(ctx context.Context, prompt string, servers mcpproxy.ServerManager) ([]acp.SessionUpdate, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("acpclient.Client.Run must be called after acpclient.Client.Start")
	}

	tmpDir, err := os.MkdirTemp("", "mcpchecker-agent-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory for agent execution: %w", err)
	}

	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	mcpServers := make([]acp.McpServer, 0, len(servers.GetMcpServers()))
	for _, srv := range servers.GetMcpServers() {
		cfg, err := srv.GetConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get config for mcp server %q: %w", srv.GetName(), err)
		}

		mcpServers = append(mcpServers, acp.McpServer{
			Http: &acp.McpServerHttp{
				Name:    srv.GetName(),
				Url:     cfg.URL,
				Type:    mcpclient.TransportTypeHttp,
				Headers: make([]acp.HttpHeader, 0),
			},
		})
	}

	session, err := c.conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        tmpDir,
		McpServers: mcpServers,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start new ACP session: %w", err)
	}

	// store the session
	c.mu.Lock()
	c.sessions[session.SessionId] = NewSession(servers)
	c.mu.Unlock()

	// this runs the current prompt to completion
	// if we were to support multi turn flows, we could run further prompts to the same session from here
	if _, err := c.conn.Prompt(ctx, acp.PromptRequest{
		SessionId: session.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
	}); err != nil {
		return nil, fmt.Errorf("failed to send prompt to acp session: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// return all the updates from this session, remove it from storage it
	res := slices.Clone(c.sessions[session.SessionId].updates)
	delete(c.sessions, session.SessionId)

	return res, nil
}

func (c *client) RunWithUsage(ctx context.Context, prompt string, servers mcpproxy.ServerManager) (*RunResult, error) {
	updates, err := c.Run(ctx, prompt, servers)
	if err != nil {
		return nil, err
	}

	result := &RunResult{
		Updates: updates,
	}

	// Extract usage data from Meta fields (safe - SDK already parsed the JSON)
	result.Usage = ExtractUsageFromMeta(updates)

	return result, nil
}

func (c *client) Close(ctx context.Context) error {
	if c.cfg.Transport != nil {
		return c.cfg.Transport.Close(ctx)
	}
	return c.closeSubprocess(ctx)
}

func (c *client) startPipes(ctx context.Context) (stdin io.Writer, stdout io.Reader, err error) {
	if c.cfg.Transport != nil {
		return c.startTransport(ctx)
	}
	return c.startSubprocess(ctx)
}

func (c *client) startTransport(ctx context.Context) (io.Writer, io.Reader, error) {
	stdin, stdout, err := c.cfg.Transport.Start(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start transport: %w", err)
	}
	return stdin, stdout, nil
}

func (c *client) startSubprocess(ctx context.Context) (io.Writer, io.Reader, error) {
	c.cmd = exec.CommandContext(ctx, c.cfg.Cmd, c.cfg.Args...)

	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open stdin pipe to acp client: %w", err)
	}

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open stdout pipe to acp client: %w", err)
	}

	if err := c.cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start acp client: %w", err)
	}

	return stdin, stdout, nil
}

func (c *client) closeSubprocess(ctx context.Context) error {
	if c.cmd == nil || (c.cmd.ProcessState != nil && c.cmd.ProcessState.Exited()) {
		return nil
	}

	if err := c.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to kill acp client process: %w", err)
	}

	// Wait for the process in a goroutine so we can respect context cancellation
	done := make(chan error, 1)
	go func() {
		done <- c.cmd.Wait()
	}()

	select {
	case <-done:
		// Process exited, pipes are closed, receive goroutine will exit
		return nil
	case <-ctx.Done():
		// Context cancelled while waiting - process is already killed,
		// so we just return the context error
		return ctx.Err()
	}
}
