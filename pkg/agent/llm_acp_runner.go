package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/mcpchecker/mcpchecker/pkg/acpclient"
	"github.com/mcpchecker/mcpchecker/pkg/llmagent"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
)

type llmACPRunner struct {
	model      string
	mcpServers mcpproxy.ServerManager
	skills     *SkillInfo
}

var _ Runner = &llmACPRunner{}

// NewLLMACPRunner creates a runner that uses the llmagent package with ACP protocol.
// The model string is in "provider:model-id" format (e.g. "openai:gpt-4o").
func NewLLMACPRunner(model string) (Runner, error) {
	if model == "" {
		return nil, fmt.Errorf("model is required for llm-agent")
	}

	return &llmACPRunner{
		model: model,
	}, nil
}

func (r *llmACPRunner) AgentName() string {
	return fmt.Sprintf("llm-agent-%s", strings.ReplaceAll(r.model, ":", "-"))
}

func (r *llmACPRunner) WithMcpServerInfo(mcpServers mcpproxy.ServerManager) Runner {
	return &llmACPRunner{
		model:      r.model,
		mcpServers: mcpServers,
		skills:     r.skills,
	}
}

func (r *llmACPRunner) WithSkillInfo(skills *SkillInfo) Runner {
	return &llmACPRunner{
		model:      r.model,
		mcpServers: r.mcpServers,
		skills:     skills,
	}
}

func (r *llmACPRunner) RunTask(ctx context.Context, prompt string) (AgentResult, error) {
	agent, err := llmagent.New(ctx, llmagent.Config{Model: r.model})
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM agent: %w", err)
	}

	transport := newLLMACPTransport(ctx, agent)

	client := acpclient.NewClient(ctx, &acpclient.AcpConfig{
		Transport: transport,
	}, r.skills.ClientOptions()...)

	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start ACP client: %w", err)
	}
	defer client.Close(ctx)

	result, err := client.RunWithUsage(ctx, prompt, r.mcpServers)
	if err != nil {
		return nil, fmt.Errorf("failed to run LLM agent: %w", err)
	}

	return &acpResult{
		updates:     result.Updates,
		prompt:      prompt,
		actualUsage: result.Usage,
	}, nil
}

// llmACPTransport implements acpclient.Transport using in-memory pipes
// connected to an LLM agent running the ACP protocol.
type llmACPTransport struct {
	ctx    context.Context
	cancel context.CancelFunc
	agent  llmagent.AcpAgent

	clientToAgentReader *io.PipeReader
	clientToAgentWriter *io.PipeWriter
	agentToClientReader *io.PipeReader
	agentToClientWriter *io.PipeWriter

	closeOnce sync.Once
	done      chan error
}

var _ acpclient.Transport = &llmACPTransport{}

func newLLMACPTransport(ctx context.Context, agent llmagent.AcpAgent) *llmACPTransport {
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()

	ctx, cancel := context.WithCancel(ctx)

	return &llmACPTransport{
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

func (t *llmACPTransport) Start(_ context.Context) (stdin io.Writer, stdout io.Reader, err error) {
	go func() {
		err := t.agent.RunACP(t.ctx, t.clientToAgentReader, t.agentToClientWriter)
		t.done <- err
	}()

	return t.clientToAgentWriter, t.agentToClientReader, nil
}

func (t *llmACPTransport) Close(ctx context.Context) error {
	var err error
	t.closeOnce.Do(func() {
		t.cancel()

		t.clientToAgentWriter.Close()
		t.clientToAgentReader.Close()
		t.agentToClientWriter.Close()
		t.agentToClientReader.Close()

		select {
		case err = <-t.done:
		case <-ctx.Done():
			err = ctx.Err()
		}
	})

	return errors.Join(err)
}
