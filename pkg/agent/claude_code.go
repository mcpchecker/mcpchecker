package agent

import (
	"fmt"
	"os/exec"

	"github.com/mcpchecker/mcpchecker/pkg/acpclient"
)

type ClaudeCodeAgent struct{}

func (a *ClaudeCodeAgent) Name() string {
	return "claude-code"
}

func (a *ClaudeCodeAgent) Description() string {
	return "Anthropic's Claude Code CLI"
}

func (a *ClaudeCodeAgent) RequiresModel() bool {
	return false // Claude Code manages its own model selection
}

func (a *ClaudeCodeAgent) ValidateEnvironment() error {
	if _, err := exec.LookPath("claude-agent-acp"); err != nil {
		return fmt.Errorf("'claude-agent-acp' binary not found in PATH (install with: npm install -g @agentclientprotocol/claude-agent-acp): %w", err)
	}
	return nil
}

func (a *ClaudeCodeAgent) GetDefaults(model string) (*AgentSpec, error) {
	return &AgentSpec{
		Metadata: AgentMetadata{
			Name: "claude-code",
		},
		AcpConfig: &acpclient.AcpConfig{
			Cmd: "claude-agent-acp",
		},
	}, nil
}
