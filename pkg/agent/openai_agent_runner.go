package agent

import (
	"context"
	"fmt"

	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/mcpchecker/mcpchecker/pkg/openaiagent"
	"github.com/mcpchecker/mcpchecker/pkg/usage"
)

// openAIAgentRunner implements Runner for OpenAI agents using the openaiagent package
type openAIAgentRunner struct {
	model   string
	baseURL string
	apiKey  string
	mcpInfo McpServerInfo
	usage   *usage.TokenUsage
}

type openAIAgentResult struct {
	output string
	usage  *usage.TokenUsage
}

func (r *openAIAgentResult) GetOutput() string {
	return r.output
}

func (r *openAIAgentResult) GetFinalMessage() string {
	return r.output // OpenAI agent output is the final message
}

func (r *openAIAgentResult) GetToolCalls() []ToolCallSummary {
	return nil // OpenAI agent doesn't expose structured tool calls yet
}

func (r *openAIAgentResult) GetThinking() string {
	return "" // OpenAI agent doesn't expose thinking
}

func (r *openAIAgentResult) GetRawUpdates() any {
	return nil // OpenAI agent doesn't have session updates
}

// GetTokenEstimate returns token estimate with only actual usage provided.
func (r *openAIAgentResult) GetTokenEstimate() TokenEstimate {
	return TokenEstimate{
		Actual: GetActualUsageFromTokenUsage(r.usage),
		Source: TokenSourceActual,
	}
}

// NewOpenAIAgentRunner creates a runner that uses the openaiagent package directly
func NewOpenAIAgentRunner(model, baseURL, apiKey string) (Runner, error) {
	if model == "" || baseURL == "" || apiKey == "" {
		return nil, fmt.Errorf("model, baseURL, and apiKey are required for OpenAI agent")
	}

	return &openAIAgentRunner{
		model:   model,
		baseURL: baseURL,
		apiKey:  apiKey,
	}, nil
}

func (r *openAIAgentRunner) WithMcpServerInfo(mcpServers mcpproxy.ServerManager) Runner {
	return &openAIAgentRunner{
		model:   r.model,
		baseURL: r.baseURL,
		apiKey:  r.apiKey,
		mcpInfo: mcpServers,
		usage:   r.usage,
	}
}

func (r *openAIAgentRunner) AgentName() string {
	return fmt.Sprintf("openai-agent-%s", r.model)
}

func (r *openAIAgentRunner) RunTask(ctx context.Context, prompt string) (AgentResult, error) {
	// Create the OpenAI agent
	agent, err := openaiagent.NewAIAgent(r.baseURL, r.apiKey, r.model, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI agent: %w", err)
	}
	defer agent.Close()

	// Add MCP servers if available
	if r.mcpInfo != nil {
		servers := r.mcpInfo.GetMcpServers()
		for _, server := range servers {
			serverCfg, err := server.GetConfig()
			if err != nil {
				return nil, fmt.Errorf("failed to get config for server %s: %w", server.GetName(), err)
			}

			if err := agent.AddMCPServer(ctx, serverCfg.URL); err != nil {
				return nil, fmt.Errorf("failed to add MCP server %s: %w", server.GetName(), err)
			}
		}
	}

	// Run the agent with the prompt
	result, err := agent.Run(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to run agent: %w", err)
	}

	// Store usage from this run
	r.usage = agent.GetUsage()

	return &openAIAgentResult{
		output: result,
		usage:  r.usage,
	}, nil
}
