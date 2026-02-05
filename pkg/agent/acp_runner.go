package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/coder/acp-go-sdk"
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

	return &acpRunnerResult{
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

type acpRunnerResult struct {
	updates     []acp.SessionUpdate
	prompt      string            // Original prompt sent to agent
	actualUsage *acpclient.Usage  // Actual usage from agent (nil if not reported)
}

var _ AgentResult = &acpRunnerResult{}

func (res *acpRunnerResult) GetOutput() string {
	if len(res.updates) == 0 {
		return "got no output from acp agent"
	}

	out, err := json.Marshal(res.updates)
	if err != nil {
		text := res.updates[len(res.updates)-1].AgentMessageChunk.Content.Text
		if text != nil {
			return text.Text
		}

		return "unable to get agent output from last acp update"
	}

	return string(out)
}

func (res *acpRunnerResult) GetFinalMessage() string {
	return ExtractFinalMessage(res.updates)
}

func (res *acpRunnerResult) GetToolCalls() []ToolCallSummary {
	return ExtractToolCalls(res.updates)
}

func (res *acpRunnerResult) GetThinking() string {
	return ExtractThinking(res.updates)
}

func (res *acpRunnerResult) GetRawUpdates() any {
	return res.updates
}

func (res *acpRunnerResult) GetTokenEstimate() TokenEstimate {
	estimate := ComputeTokenEstimate(res.prompt, res.GetFinalMessage(), res.GetThinking(), res.GetToolCalls())
	estimate.Source = TokenSourceEstimated

	// Use actual usage from agent if available
	if res.actualUsage != nil {
		estimate.Source = TokenSourceActual
		estimate.Actual = &ActualUsage{
			InputTokens:  res.actualUsage.InputTokens,
			OutputTokens: res.actualUsage.OutputTokens,
			TotalTokens:  res.actualUsage.TotalTokens,
		}
		if res.actualUsage.ThoughtTokens != nil {
			estimate.Actual.ThoughtTokens = res.actualUsage.ThoughtTokens
		}
		if res.actualUsage.CachedReadTokens != nil {
			estimate.Actual.CachedReadTokens = res.actualUsage.CachedReadTokens
		}
		if res.actualUsage.CachedWriteTokens != nil {
			estimate.Actual.CachedWriteTokens = res.actualUsage.CachedWriteTokens
		}
		// Override the main counts with actual values
		estimate.InputTokens = res.actualUsage.InputTokens
		estimate.OutputTokens = res.actualUsage.OutputTokens
		estimate.TotalTokens = res.actualUsage.TotalTokens
	}

	return estimate
}
