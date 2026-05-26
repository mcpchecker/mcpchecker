package acpclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/coder/acp-go-sdk"
)

// this file is responsible for implementing the acp.Client interface on our client type
var _ acp.Client = &client{}

// Request for user permission to execute a tool call.
//
// Sent when the agent needs authorization before performing a sensitive operation.
//
// See protocol docs: [Requesting Permission](https://agentclientprotocol.com/protocol/tool-calls#requesting-permission)
func (c *client) RequestPermission(ctx context.Context, params acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	session, ok := c.sessions[params.SessionId]
	if !ok {
		return acp.RequestPermissionResponse{}, fmt.Errorf("no matching session on client")
	}

	if len(params.Options) < 1 {
		return acp.RequestPermissionResponse{}, fmt.Errorf("at least one option is required to request permission")
	}

	session.recordPermissionToolCall(params.ToolCall)
	if session.isAllowedToolCall(ctx, params.ToolCall) {
		// try to find an always allow or allow once option, else default to first opt
		bestOpt := params.Options[0]
		for _, opt := range params.Options {
			if opt.Kind == acp.PermissionOptionKindAllowAlways {
				bestOpt = opt
				break
			}
			if opt.Kind == acp.PermissionOptionKindAllowOnce {
				bestOpt = opt // keep going in case we find the always allow option
			}
		}

		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{
				Selected: &acp.RequestPermissionOutcomeSelected{
					Outcome:  "selected",
					OptionId: bestOpt.OptionId,
				},
			},
		}, nil
	}

	found := false
	var bestOpt acp.PermissionOption
	for _, opt := range params.Options {
		if opt.Kind == acp.PermissionOptionKindRejectAlways {
			bestOpt = opt
			found = true
			break
		}
		if opt.Kind == acp.PermissionOptionKindRejectOnce {
			bestOpt = opt
			found = true
		}
	}

	if !found {
		return acp.RequestPermissionResponse{}, fmt.Errorf("no reject option provided")
	}

	return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{
		Selected: &acp.RequestPermissionOutcomeSelected{
			Outcome:  "selected",
			OptionId: bestOpt.OptionId,
		},
	}}, nil
}

// Notification containing a session update from the agent.
//
// Used to stream real-time progress and results during prompt processing.
//
// See protocol docs: [Agent Reports Output](https://agentclientprotocol.com/protocol/prompt-turn#3-agent-reports-output)
func (c *client) SessionUpdate(ctx context.Context, params acp.SessionNotification) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	session, ok := c.sessions[params.SessionId]
	if !ok {
		return fmt.Errorf("no matching session on client")
	}

	session.update(params.Update)

	return nil
}

// Request to read content from a text file.
//
// Only available if the client supports the 'fs.readTextFile' capability.
func (c *client) ReadTextFile(ctx context.Context, params acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	c.mu.RLock()
	sess, ok := c.sessions[params.SessionId]
	c.mu.RUnlock()

	if !ok || sess.cwd == "" {
		return acp.ReadTextFileResponse{}, fmt.Errorf("no fs.readTextFile capability")
	}
	cwd := sess.cwd

	if !filepath.IsAbs(params.Path) {
		return acp.ReadTextFileResponse{}, fmt.Errorf("path must be absolute: %s", params.Path)
	}

	// Security: scope reads to the agent's working directory.
	// Resolve symlinks so a symlink inside cwd cannot escape the boundary.
	cleanPath, err := filepath.EvalSymlinks(filepath.Clean(params.Path))
	if err != nil {
		return acp.ReadTextFileResponse{}, fmt.Errorf("resolve path %q: %w", params.Path, err)
	}
	resolvedCwd, err := filepath.EvalSymlinks(filepath.Clean(cwd))
	if err != nil {
		return acp.ReadTextFileResponse{}, fmt.Errorf("resolve working directory: %w", err)
	}
	cwdPrefix := resolvedCwd + string(filepath.Separator)
	if !strings.HasPrefix(cleanPath, cwdPrefix) && cleanPath != resolvedCwd {
		return acp.ReadTextFileResponse{}, fmt.Errorf("path %q is outside the agent working directory", params.Path)
	}

	b, err := os.ReadFile(cleanPath)
	if err != nil {
		return acp.ReadTextFileResponse{}, fmt.Errorf("read %s: %w", params.Path, err)
	}

	content := string(b)

	// Apply optional line/limit (1-based line index)
	if params.Line != nil || params.Limit != nil {
		lines := strings.Split(content, "\n")
		start := 0
		if params.Line != nil && *params.Line > 0 {
			start = min(max(*params.Line-1, 0), len(lines))
		}
		end := len(lines)
		if params.Limit != nil && *params.Limit > 0 {
			if start+*params.Limit < end {
				end = start + *params.Limit
			}
		}
		content = strings.Join(lines[start:end], "\n")
	}

	return acp.ReadTextFileResponse{Content: content}, nil
}

// Request to write content to a text file.
//
// Only available if the client supports the 'fs.writeTextFile' capability.
func (c *client) WriteTextFile(ctx context.Context, params acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, fmt.Errorf("no fs.writeTextFile capability")
}

// Request to create a new terminal and execute a command.
//
// Only available if the client supports the 'terminal' capability
func (c *client) CreateTerminal(ctx context.Context, params acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, fmt.Errorf("no terminal capability")
}

// Request to kill a terminal command without releasing the terminal.
//
// Only available if the client supports the 'terminal' capability
func (c *client) KillTerminal(ctx context.Context, params acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, fmt.Errorf("no terminal capability")
}

// Request to get the current output and status of a terminal.
//
// Only available if the client supports the 'terminal' capability
func (c *client) TerminalOutput(ctx context.Context, params acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, fmt.Errorf("no terminal capability")
}

// Request to release a terminal and free its resources.
//
// Only available if the client supports the 'terminal' capability
func (c *client) ReleaseTerminal(ctx context.Context, params acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, fmt.Errorf("no terminal capability")
}

// Request to wait for a terminal command to exit.
//
// Only available if the client supports the 'terminal' capability
func (c *client) WaitForTerminalExit(ctx context.Context, params acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, fmt.Errorf("no terminal capability")
}
