package eval

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerToolCoverage describes coverage against one server's discovery snapshot.
type ServerToolCoverage struct {
	ServerName     string         `json:"serverName"`
	DiscoveryError string         `json:"discoveryError,omitempty"`
	DiscoveryOK    bool           `json:"discoveryOk"`
	Tools          []ToolCoverage `json:"tools"`
}

// ToolCoverage counts completed recorded attempts, including failed calls.
type ToolCoverage struct {
	Name         string `json:"name"`
	Called       bool   `json:"called"`
	SuccessCount int    `json:"successCount"`
	FailureCount int    `json:"failureCount"`
}

const toolCoverageDiscoveryTimeout = 30 * time.Second

func discoverToolCoverage(ctx context.Context, config *mcpclient.MCPConfig) []ServerToolCoverage {
	return discoverToolCoverageWithTimeout(ctx, config, toolCoverageDiscoveryTimeout)
}

func discoverToolCoverageWithTimeout(ctx context.Context, config *mcpclient.MCPConfig, timeout time.Duration) []ServerToolCoverage {
	if config == nil {
		return nil
	}
	manager, _ := mcpclient.ManagerFromContext(ctx)
	servers := config.GetEnabledServers()
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	coverage := make([]ServerToolCoverage, 0, len(names))
	for _, name := range names {
		entry := ServerToolCoverage{ServerName: name, Tools: []ToolCoverage{}}
		if manager == nil {
			entry.DiscoveryError = "MCP client manager unavailable"
		} else if client, ok := manager.Get(name); !ok {
			entry.DiscoveryError = "MCP client unavailable"
		} else if client.InitializeResult().Capabilities.Tools == nil {
			entry.DiscoveryOK = true
		} else {
			var discoveryErr error
			discoveryCtx, cancel := context.WithTimeout(ctx, timeout)
			for tool, err := range client.Tools(discoveryCtx, &mcp.ListToolsParams{}) {
				if err != nil {
					discoveryErr = err
					break
				}
				entry.Tools = append(entry.Tools, ToolCoverage{Name: tool.Name})
			}
			cancel()
			if discoveryErr != nil {
				entry.DiscoveryError = fmt.Sprintf("failed to discover tools: %v", discoveryErr)
				entry.Tools = []ToolCoverage{}
			} else {
				entry.DiscoveryOK = true
				sort.Slice(entry.Tools, func(i, j int) bool { return entry.Tools[i].Name < entry.Tools[j].Name })
			}
		}
		coverage = append(coverage, entry)
	}
	return coverage
}

func aggregateToolCoverage(coverage []ServerToolCoverage, results []*EvalResult) []ServerToolCoverage {
	type key struct{ server, tool string }
	counts := make(map[key]ToolCoverage)
	for _, result := range results {
		if result == nil || result.CallHistory == nil {
			continue
		}
		for _, call := range result.CallHistory.ToolCalls {
			if call == nil {
				continue
			}
			k := key{call.ServerName, call.ToolName}
			count := counts[k]
			count.Name, count.Called = call.ToolName, true
			if !call.Success || (call.Result != nil && call.Result.IsError) {
				count.FailureCount++
			} else {
				count.SuccessCount++
			}
			counts[k] = count
		}
	}
	for i := range coverage {
		for j := range coverage[i].Tools {
			k := key{coverage[i].ServerName, coverage[i].Tools[j].Name}
			if count, ok := counts[k]; ok {
				coverage[i].Tools[j] = count
			}
		}
	}
	sort.Slice(coverage, func(i, j int) bool { return coverage[i].ServerName < coverage[j].ServerName })
	return coverage
}
