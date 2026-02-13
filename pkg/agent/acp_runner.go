package agent

import (
	"context"
	"fmt"

	"github.com/mcpchecker/mcpchecker/pkg/acpclient"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
)

type acpRunner struct {
	name       string
	cfg        *acpclient.AcpConfig
	mcpServers mcpproxy.ServerManager
}

var _ Runner = &acpRunner{}

func NewAcpRunner(cfg *acpclient.AcpConfig, name string) Runner {
	return &acpRunner{
		name: name,
		cfg:  cfg,
	}
}

func (r *acpRunner) RunTask(ctx context.Context, prompt string) (AgentResult, error) {
	client := acpclient.NewClient(ctx, r.cfg)
	defer client.Close(ctx)

	err := client.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start acp client: %w", err)
	}

	result, err := client.RunWithUsage(ctx, prompt, r.mcpServers)
	if err != nil {
		return nil, fmt.Errorf("failed to run acp agent: %w", err)
	}

	return &acpResult{
		updates:     result.Updates,
		prompt:      prompt,
		actualUsage: result.Usage,
	}, nil
}

func (r *acpRunner) WithMcpServerInfo(mcpServers mcpproxy.ServerManager) Runner {
	return &acpRunner{
		name:       r.name,
		cfg:        r.cfg,
		mcpServers: mcpServers,
	}
}

func (r *acpRunner) AgentName() string {
	return r.name
}
