package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/mcpchecker/mcpchecker/pkg/extension/protocol"
	"golang.org/x/exp/jsonrpc2"
)

type Client interface {
	Start(ctx context.Context, params *protocol.InitializeParams) error
	Execute(ctx context.Context, params *protocol.ExecuteParams) (*protocol.ExecuteResult, error)
	Manifest() *protocol.InitializeResult
	Shutdown(ctx context.Context) error
}

type client struct {
	cmd      *exec.Cmd
	conn     *jsonrpc2.Connection
	manifest *protocol.InitializeResult
	opts     Options
	mux      sync.Mutex
}

var _ Client = &client{}

type Options struct {
	BinaryPath string
	Env        []string
	LogHandler func(level, message string, data map[string]any)
}

func New(opts Options) Client {
	return &client{opts: opts}
}

func (c *client) Start(ctx context.Context, params *protocol.InitializeParams) error {
	c.cmd = exec.CommandContext(ctx, c.opts.BinaryPath)
	c.cmd.Env = c.opts.Env

	var err error

	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err = c.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start extension: %w", err)
	}

	c.conn, err = jsonrpc2.Dial(ctx, &cmdDialer{stdin: stdin, stdout: stdout}, &jsonrpc2.ConnectionOptions{
		Handler: c,
		Framer:  protocol.NewlineFramer(),
	})
	if err != nil {
		_ = c.cmd.Process.Kill()
		return fmt.Errorf("failed to connect to extension: %w", err)
	}

	c.manifest, err = c.initialize(ctx, params)
	if err != nil {
		_ = c.cmd.Process.Kill()
		return fmt.Errorf("failed to initialize extension: %w", err)
	}

	return nil
}

func (c *client) Handle(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	if req.Method == protocol.MethodLog && c.opts.LogHandler != nil {
		var params protocol.LogParams
		if err := json.Unmarshal(req.Params, &params); err == nil {
			c.opts.LogHandler(params.Level, params.Message, params.Data)
		}
	}

	return nil, nil
}

func (c *client) Execute(ctx context.Context, params *protocol.ExecuteParams) (*protocol.ExecuteResult, error) {
	result := &protocol.ExecuteResult{}
	if err := c.call(ctx, protocol.MethodExecute, params, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *client) Shutdown(ctx context.Context) error {
	// Use a timeout for the shutdown RPC call to avoid hanging if the extension is unresponsive
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := c.call(shutdownCtx, protocol.MethodShutdown, struct{}{}, nil); err != nil {
		c.closeConn()
		killErr := c.cmd.Process.Kill()
		_ = c.cmd.Wait() // reap the process to avoid zombies
		return errors.Join(err, killErr)
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- c.cmd.Wait()
	}()

	select {
	case err := <-waitDone:
		c.closeConn()
		return err
	case <-ctx.Done():
		c.closeConn()
		killErr := c.cmd.Process.Kill()
		waitErr := <-waitDone // drain the goroutine's Wait result
		return errors.Join(ctx.Err(), killErr, waitErr)
	}
}

// closeConn safely closes the JSON-RPC connection if it exists.
// Errors from Close are intentionally ignored to avoid masking primary errors.
func (c *client) closeConn() {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

func (c *client) Manifest() *protocol.InitializeResult {
	return c.manifest
}

func (c *client) initialize(ctx context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	params.ProtocolVersion = protocol.ProtocolVersion

	result := &protocol.InitializeResult{}
	if err := c.call(ctx, protocol.MethodInitialize, params, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *client) call(ctx context.Context, method string, params, result any) error {
	c.mux.Lock()
	defer c.mux.Unlock()
	call := c.conn.Call(ctx, method, params)
	return call.Await(ctx, result)
}
