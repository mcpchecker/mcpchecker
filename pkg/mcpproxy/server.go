package mcpproxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server interface {
	// Run starts the proxy server
	Run(ctx context.Context) error
	// GetConfig gets the mcp config to connect to the running MCP proxy server
	GetConfig() (*mcpclient.ServerConfig, error)
	// GetName returns the name of the MCP server
	GetName() string
	// GetAllowedTools returns all the tools the user allowed
	GetAllowedTools(ctx context.Context) ([]*mcp.Tool, error)
	// GetInstructions returns the server instructions from InitializeResult
	GetInstructions() string
	// Close closes the MCP proxy server, but not the underlying client connection
	Close() error
	// GetCallHistory returns all the MCP calls made while the proxy server was running
	GetCallHistory() CallHistory
	// WaitReady blocks until the server has initialized and is ready to serve
	WaitReady(ctx context.Context) error
}

type server struct {
	name         string
	proxyServer  *mcp.Server
	proxyClient  *mcpclient.Client
	url          string
	instructions string

	// Call tracking
	recorder Recorder

	// Ready signaling
	ready    chan struct{}
	startErr error // Stores any error that occurred during startup

	// Shutdown signalling
	cancel context.CancelFunc
	done   chan error
}

var _ Server = &server{}

func NewProxyServerForClient(ctx context.Context, name string, client *mcpclient.Client) (Server, error) {
	r := NewRecorder(name)

	s, err := createProxyServer(ctx, client.ClientSession, r)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy server for %q: %w", name, err)
	}

	var instructions string
	if initResult := client.ClientSession.InitializeResult(); initResult != nil {
		instructions = initResult.Instructions
	}

	return &server{
		name:         name,
		proxyServer:  s,
		proxyClient:  client,
		instructions: instructions,
		recorder:     r,
		ready:        make(chan struct{}),
		done:         make(chan error, 1),
	}, nil
}

func createProxyServer(ctx context.Context, cs *mcp.ClientSession, r Recorder) (*mcp.Server, error) {
	serverCaps := cs.InitializeResult().Capabilities
	opts := &mcp.ServerOptions{
		Instructions: cs.InitializeResult().Instructions,
		Capabilities: &mcp.ServerCapabilities{
			Prompts:   serverCaps.Prompts,
			Resources: serverCaps.Resources,
			Tools:     serverCaps.Tools,
		},
	}
	s := mcp.NewServer(
		cs.InitializeResult().ServerInfo,
		opts,
	)

	if opts.Capabilities.Prompts != nil {
		for p, err := range cs.Prompts(ctx, &mcp.ListPromptsParams{}) {
			if err != nil {
				return nil, fmt.Errorf("failed to list prompts: %w", err)
			}
			s.AddPrompt(p, func(ctx context.Context, gpr *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				start := time.Now()
				res, err := cs.GetPrompt(ctx, gpr.Params)
				r.RecordPromptGet(gpr, res, err, start)
				return res, err
			})
		}
	}

	if opts.Capabilities.Resources != nil {
		for rr, err := range cs.Resources(ctx, &mcp.ListResourcesParams{}) {
			if err != nil {
				return nil, fmt.Errorf("failed to list resources: %w", err)
			}
			s.AddResource(rr, func(ctx context.Context, rrr *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				start := time.Now()
				res, err := cs.ReadResource(ctx, rrr.Params)
				r.RecordResourceRead(rrr, res, err, start)
				return res, err
			})
		}

		for rt, err := range cs.ResourceTemplates(ctx, &mcp.ListResourceTemplatesParams{}) {
			if err != nil {
				return nil, fmt.Errorf("failed to list resource templates: %w", err)
			}
			s.AddResourceTemplate(rt, func(ctx context.Context, rrr *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				start := time.Now()
				res, err := cs.ReadResource(ctx, rrr.Params)
				r.RecordResourceRead(rrr, res, err, start)
				return res, err
			})
		}
	}

	if opts.Capabilities.Tools != nil {
		for t, err := range cs.Tools(ctx, &mcp.ListToolsParams{}) {
			if err != nil {
				return nil, fmt.Errorf("failed to list tools: %w", err)
			}
			s.AddTool(t, func(ctx context.Context, ctr *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				start := time.Now()
				res, err := cs.CallTool(ctx, &mcp.CallToolParams{
					Meta:      ctr.Params.Meta,
					Name:      ctr.Params.Name,
					Arguments: ctr.Params.Arguments,
				})
				r.RecordToolCall(ctr, res, err, start)
				return res, err
			})
		}
	}

	return s, nil
}

// Run is a blocking call until ctx is cancelled or Close is called
// Run will start the server in streamablehttp transport
// TODO(Cali0707): update this to support other transports
func (s *server) Run(ctx context.Context) error {
	ctx, s.cancel = context.WithCancel(ctx)

	mux := http.NewServeMux()

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.proxyServer
	}, &mcp.StreamableHTTPOptions{})

	mux.Handle("/mcp", handler)

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		s.startErr = fmt.Errorf("failed to start listen: %w", err)
		close(s.ready)
		s.done <- s.startErr
		return s.startErr
	}

	s.url = fmt.Sprintf("http://%s/mcp", listener.Addr().String())

	// Signal that the server is ready (URL is set and listener is ready)
	close(s.ready)

	httpServer := &http.Server{
		Handler: mux,
	}

	// Run server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Wait for context cancellation or server error
	var runErr error
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			runErr = fmt.Errorf("server shutdown failed: %w", err)
		}
	case runErr = <-serverErr:
	}

	s.done <- runErr
	return runErr
}

func (s *server) GetConfig() (*mcpclient.ServerConfig, error) {
	if s.url == "" {
		return nil, fmt.Errorf("url must be set for config to be valid, ensure Run() is called before GetConfig()")
	}

	// Intentionally no headers: this config describes how to reach the local
	// proxy, not the upstream MCP server. The proxy's mcpclient already holds
	// whatever headers/OAuth credentials are needed to talk to the upstream
	// server and attaches them itself. The local proxy listener is
	// unauthenticated and does not consume any headers from the agent, so
	// forwarding the upstream server's headers here would needlessly expose
	// its credentials (static Authorization/API-key headers, etc.) to the
	// agent.
	return &mcpclient.ServerConfig{
		Type: mcpclient.TransportTypeHttp,
		URL:  s.url,
	}, nil
}

func (s *server) GetName() string {
	return s.name
}

func (s *server) GetAllowedTools(ctx context.Context) ([]*mcp.Tool, error) {
	return s.proxyClient.GetAllowedTools(ctx)
}

func (s *server) GetInstructions() string {
	return s.instructions
}

func (s *server) Close() error {
	if s.cancel == nil {
		return nil
	}

	s.cancel()
	return <-s.done
}

func (s *server) GetCallHistory() CallHistory {
	return s.recorder.GetHistory()
}

func (s *server) WaitReady(ctx context.Context) error {
	select {
	case <-s.ready:
		return s.startErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
