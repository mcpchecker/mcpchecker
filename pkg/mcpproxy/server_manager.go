package mcpproxy

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"

	"golang.org/x/sync/errgroup"

	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
)

const (
	McpServerFileName = "mcp-server.json"
)

type ServerManager interface {
	GetMcpServerFiles() ([]string, error)
	GetMcpServers() []Server
	// Start blocks until all servers are ready or an error occurs. Caller must ensure this is only called once, and called before Close
	Start(ctx context.Context) error
	// Close closes associated server resources. Caller must ensure this is only called once, and called after Start
	Close() error

	// aggregate call tracking
	GetAllCallHistory() *CallHistory
	GetCallHistoryForServer(serverName string) (CallHistory, bool)
}

type serverManager struct {
	servers map[string]Server
	tmpDir  string

	cancel context.CancelFunc
	eg     *errgroup.Group
}

// NewEmptyServerManager creates a ServerManager with no servers.
// Used when MCP is not configured (e.g., skill-only evaluations).
func NewEmptyServerManager() ServerManager {
	return &emptyServerManager{}
}

type emptyServerManager struct{}

func (m *emptyServerManager) GetMcpServerFiles() ([]string, error) { return nil, nil }
func (m *emptyServerManager) GetMcpServers() []Server              { return nil }
func (m *emptyServerManager) Start(_ context.Context) error        { return nil }
func (m *emptyServerManager) Close() error                         { return nil }
func (m *emptyServerManager) GetAllCallHistory() *CallHistory      { return &CallHistory{} }
func (m *emptyServerManager) GetCallHistoryForServer(_ string) (CallHistory, bool) {
	return CallHistory{}, false
}

func NewServerManager(ctx context.Context, manager mcpclient.Manager) (ServerManager, error) {
	clients := manager.GetAll()
	servers := make(map[string]Server, len(clients))
	for name, client := range clients {
		s, err := NewProxyServerForClient(ctx, name, client)
		if err != nil {
			return nil, err
		}

		servers[name] = s
	}

	return &serverManager{
		servers: servers,
	}, nil
}

func (m *serverManager) GetMcpServerFiles() ([]string, error) {
	if m.tmpDir != "" {
		return []string{fmt.Sprintf("%s/%s", m.tmpDir, McpServerFileName)}, nil
	}

	cfg, err := m.getMcpServers()
	if err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, err
	}

	err = cfg.ToFile(fmt.Sprintf("%s/%s", tmpDir, McpServerFileName))
	if err != nil {
		rmErr := os.Remove(tmpDir)
		if rmErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to remove temp dir '%s': %w", tmpDir, rmErr))
		}

		return nil, err
	}

	m.tmpDir = tmpDir

	return []string{fmt.Sprintf("%s/%s", tmpDir, McpServerFileName)}, nil

}

func (m *serverManager) GetMcpServers() []Server {
	return slices.Collect(maps.Values(m.servers))
}

func (m *serverManager) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	// Use errgroup to start all servers concurrently
	g, gctx := errgroup.WithContext(ctx)
	m.eg = g

	// Start all servers
	for name, srv := range m.servers {
		g.Go(func() error {
			if err := srv.Run(gctx); err != nil {
				return fmt.Errorf("server %s failed: %w", name, err)
			}
			return nil
		})
	}

	// Wait for all servers to be ready before returning
	for name, srv := range m.servers {
		if err := srv.WaitReady(ctx); err != nil {
			cancel() // Cancel all servers if one fails to become ready
			return fmt.Errorf("server %s failed to become ready: %w", name, err)
		}
	}

	return nil
}

func (m *serverManager) Close() error {
	// Signal all servers to stop
	m.cancel()

	// Wait for all servers to finish
	var errs []error
	if err := m.eg.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		errs = append(errs, err)
	}

	// Close all servers (cleanup connections, etc.)
	for name, srv := range m.servers {
		if err := srv.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close server %s: %w", name, err))
		}
	}

	// Clean up temp directory
	if m.tmpDir != "" {
		if err := os.RemoveAll(m.tmpDir); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove temp dir: %w", err))
		}
	}

	return errors.Join(errs...)
}

func (m *serverManager) GetAllCallHistory() *CallHistory {
	combined := CallHistory{}

	for _, srv := range m.servers {
		history := srv.GetCallHistory()
		combined.PromptGets = append(combined.PromptGets, history.PromptGets...)
		combined.ResourceReads = append(combined.ResourceReads, history.ResourceReads...)
		combined.ToolCalls = append(combined.ToolCalls, history.ToolCalls...)
	}

	// sort all by timestamp for chronological order
	sort.Slice(combined.ToolCalls, func(i, j int) bool {
		return combined.ToolCalls[i].Timestamp.Before(combined.ToolCalls[j].Timestamp)
	})
	sort.Slice(combined.ResourceReads, func(i, j int) bool {
		return combined.ResourceReads[i].Timestamp.Before(combined.ResourceReads[j].Timestamp)
	})
	sort.Slice(combined.PromptGets, func(i, j int) bool {
		return combined.PromptGets[i].Timestamp.Before(combined.PromptGets[j].Timestamp)
	})

	return &combined
}

func (m *serverManager) GetCallHistoryForServer(serverName string) (CallHistory, bool) {
	srv, ok := m.servers[serverName]
	if !ok {
		return CallHistory{}, false
	}

	return srv.GetCallHistory(), true
}

func (m *serverManager) getMcpServers() (*mcpclient.MCPConfig, error) {
	cfg := &mcpclient.MCPConfig{
		MCPServers: make(map[string]*mcpclient.ServerConfig),
	}
	for n, s := range m.servers {
		serverCfg, err := s.GetConfig()
		if err != nil {
			return nil, err
		}

		cfg.MCPServers[n] = serverCfg
	}

	return cfg, nil
}
