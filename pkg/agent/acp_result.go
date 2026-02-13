package agent

import (
	"encoding/json"

	"github.com/coder/acp-go-sdk"
	"github.com/mcpchecker/mcpchecker/pkg/acpclient"
)

// acpResult is a shared AgentResult implementation for ACP-based runners.
type acpResult struct {
	updates     []acp.SessionUpdate
	prompt      string
	actualUsage *acpclient.Usage
}

var _ AgentResult = &acpResult{}

func (res *acpResult) GetOutput() string {
	if len(res.updates) == 0 {
		return "got no output from acp agent"
	}

	out, err := json.Marshal(res.updates)
	if err != nil {
		lastUpdate := res.updates[len(res.updates)-1]
		if lastUpdate.AgentMessageChunk != nil &&
			lastUpdate.AgentMessageChunk.Content.Text != nil {
			return lastUpdate.AgentMessageChunk.Content.Text.Text
		}
		return "unable to get agent output from last acp update"
	}

	return string(out)
}

func (res *acpResult) GetFinalMessage() string {
	return ExtractFinalMessage(res.updates)
}

func (res *acpResult) GetToolCalls() []ToolCallSummary {
	return ExtractToolCalls(res.updates)
}

func (res *acpResult) GetThinking() string {
	return ExtractThinking(res.updates)
}

func (res *acpResult) GetRawUpdates() any {
	return res.updates
}

func (res *acpResult) GetTokenEstimate() TokenEstimate {
	estimate := ComputeTokenEstimate(res.prompt, res.GetFinalMessage(), res.GetThinking(), res.GetToolCalls())
	estimate.Source = TokenSourceEstimated

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
		estimate.InputTokens = res.actualUsage.InputTokens
		estimate.OutputTokens = res.actualUsage.OutputTokens
		estimate.TotalTokens = res.actualUsage.TotalTokens
	}

	return estimate
}
