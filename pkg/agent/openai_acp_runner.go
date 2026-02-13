package agent

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/mcpchecker/mcpchecker/pkg/acpclient"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/mcpchecker/mcpchecker/pkg/openaiagent"
)

type openAIACPRunner struct {
	model      string
	baseURL    string
	apiKey     string
	mcpServers mcpproxy.ServerManager
}

var _ Runner = &openAIACPRunner{}

func NewOpenAIACPRunner(model, baseURL, apiKey string) (Runner, error) {
	if model == "" || baseURL == "" || apiKey == "" {
		return nil, fmt.Errorf("model, baseURL, and apiKey are required for OpenAI ACP agent")
	}

	return &openAIACPRunner{
		model:   model,
		baseURL: baseURL,
		apiKey:  apiKey,
	}, nil
}

func (r *openAIACPRunner) AgentName() string {
	return fmt.Sprintf("openai-acp-%s", r.model)
}

func (r *openAIACPRunner) WithMcpServerInfo(mcpServers mcpproxy.ServerManager) Runner {
	return &openAIACPRunner{
		model:      r.model,
		baseURL:    r.baseURL,
		apiKey:     r.apiKey,
		mcpServers: mcpServers,
	}
}

func (r *openAIACPRunner) RunTask(ctx context.Context, prompt string) (AgentResult, error) {
	agent, err := openaiagent.NewAIAgent(r.baseURL, r.apiKey, r.model, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI agent: %w", err)
	}

	// Add MCP servers to the agent
	if r.mcpServers != nil {
		for _, srv := range r.mcpServers.GetMcpServers() {
			cfg, err := srv.GetConfig()
			if err != nil {
				agent.Close()
				return nil, fmt.Errorf("failed to get config for server %s: %w", srv.GetName(), err)
			}
			if err := agent.AddMCPServer(ctx, cfg.URL); err != nil {
				agent.Close()
				return nil, fmt.Errorf("failed to add MCP server %s: %w", srv.GetName(), err)
			}
		}
	}

	transport := newOpenAIACPTransport(ctx, agent)

	client := acpclient.NewClient(ctx, &acpclient.AcpConfig{
		Transport: transport,
	})

	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start ACP client: %w", err)
	}
	// Only defer Close after successful Start - Start handles its own cleanup on failure
	defer client.Close(ctx)

	result, err := client.RunWithUsage(ctx, prompt, r.mcpServers)
	if err != nil {
		return nil, fmt.Errorf("failed to run ACP agent: %w", err)
	}

	return &acpResult{
		updates:     result.Updates,
		prompt:      prompt,
		actualUsage: result.Usage,
	}, nil
}

// openAIACPTransport implements acpclient.Transport using in-memory pipes
// connected to an OpenAI agent running the ACP protocol.
type openAIACPTransport struct {
	ctx    context.Context
	cancel context.CancelFunc
	agent  *openaiagent.AIAgent

	// Pipes for communication
	clientToAgentReader *io.PipeReader
	clientToAgentWriter *io.PipeWriter
	agentToClientReader *io.PipeReader
	agentToClientWriter *io.PipeWriter

	done chan error
}

func newOpenAIACPTransport(ctx context.Context, agent *openaiagent.AIAgent) *openAIACPTransport {
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()

	ctx, cancel := context.WithCancel(ctx)

	return &openAIACPTransport{
		ctx:                 ctx,
		cancel:              cancel,
		agent:               agent,
		clientToAgentReader: clientToAgentReader,
		clientToAgentWriter: clientToAgentWriter,
		agentToClientReader: agentToClientReader,
		agentToClientWriter: agentToClientWriter,
		done:                make(chan error, 1),
	}
}

func (t *openAIACPTransport) Start(ctx context.Context) (stdin io.Writer, stdout io.Reader, err error) {
	// Start the agent's ACP handler in a goroutine
	go func() {
		// Agent reads from clientToAgentReader, writes to agentToClientWriter
		err := openaiagent.RunACP(t.ctx, t.agent, t.clientToAgentReader, t.agentToClientWriter)
		t.done <- err
	}()

	// Client writes to clientToAgentWriter, reads from agentToClientReader
	return t.clientToAgentWriter, t.agentToClientReader, nil
}

func (t *openAIACPTransport) Close(ctx context.Context) error {
	// Cancel the context to signal RunACP to exit
	t.cancel()

	// Close all pipe ends to signal EOF and unblock any pending I/O
	t.clientToAgentWriter.Close()
	t.clientToAgentReader.Close()
	t.agentToClientWriter.Close()
	t.agentToClientReader.Close()

	// Wait for agent goroutine to finish and capture its error
	var err error
	select {
	case err = <-t.done:
	case <-ctx.Done():
		err = ctx.Err()
	}

	return errors.Join(err, t.agent.Close())
}

var _ acpclient.Transport = &openAIACPTransport{}
