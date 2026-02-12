package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mcpchecker/mcpchecker/pkg/openaiagent"
	"github.com/spf13/cobra"
)

var (
	mcpURL       string
	prompt       string
	modelBaseURL string
	modelKey     string
	modelName    string
	systemPrompt string
	runAcp       bool
)

var rootCmd = &cobra.Command{
	Use:   "agent-cli",
	Short: "A CLI tool that connects to an MCP server and runs an OpenAI compliant agent",
	Long: `agent-cli is a command-line interface that connects to a Model Context Protocol (MCP)
server and uses OpenAI compliant API to run an intelligent agent. The agent can interact with
tools provided by the MCP server to accomplish tasks.`,
	Example: `  agent-cli --mcp-url http://localhost:3000 --prompt "What files are in the current directory?"
  agent-cli --mcp-url http://localhost:3000 --prompt "Read the README file" --model-name gpt-4o`,
	RunE: runAgent,
}

func init() {
	// Required flags
	rootCmd.Flags().StringVar(&mcpURL, "mcp-url", "", "MCP server URL (required)")
	rootCmd.Flags().StringVar(&prompt, "prompt", "", "Prompt to send to the agent (required)")

	// Optional flags with environment variable defaults
	rootCmd.Flags().StringVar(&modelBaseURL, "model-base-url", getEnvOrDefault("MODEL_BASE_URL", ""), "OpenAI API compliant base URL, like https://api.openai.com/v1")
	rootCmd.Flags().StringVar(&modelKey, "model-key", getEnvOrDefault("MODEL_KEY", ""), "Model API key")
	rootCmd.Flags().StringVar(&modelName, "model-name", getEnvOrDefault("MODEL_NAME", ""), "Model name to use")
	rootCmd.Flags().StringVar(&systemPrompt, "system", getEnvOrDefault("SYSTEM_PROMPT", ""), "System prompt for the agent")
	rootCmd.Flags().BoolVar(&runAcp, "acp", false, "Run as an ACP agent")

	// Mark required flags
	rootCmd.MarkFlagRequired("mcp-url")
	rootCmd.MarkFlagRequired("prompt")
}

func runAgent(cmd *cobra.Command, args []string) error {
	// Validate Model API key
	if modelBaseURL == "" {
		return fmt.Errorf("OpenAI API compliant base URL must be provided via --model-base-url flag or MODEL_BASE_URL environment variable")
	}

	// Validate Model API key
	if modelKey == "" {
		return fmt.Errorf("Model API key must be provided via --model-key flag or MODEL_KEY environment variable")
	}

	// Create context
	ctx := context.Background()

	// Create the AI agent
	agentInstance, err := openaiagent.NewAIAgent(modelBaseURL, modelKey, modelName, systemPrompt)
	if err != nil {
		return fmt.Errorf("failed to create AI agent: %w", err)
	}

	// Ensure cleanup
	defer func() {
		if err := agentInstance.Close(); err != nil {
			log.Printf("Warning: Failed to close agent cleanly: %v", err)
		}
	}()

	if runAcp {
		return openaiagent.RunACP(ctx, agentInstance, os.Stdin, os.Stdout)
	}

	// Add the MCP server for legacy mode only
	if err := agentInstance.AddMCPServer(ctx, mcpURL); err != nil {
		return fmt.Errorf("failed to add MCP server: %w", err)
	}

	return runLegacy(ctx, agentInstance, prompt)
}

func runLegacy(ctx context.Context, agent *openaiagent.AIAgent, prompt string) error {
	result, err := agent.Run(ctx, prompt)
	if err != nil {
		return fmt.Errorf("agent execution failed: %w", err)
	}

	// Output the result
	fmt.Println("Agent Response:")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println(result)

	return nil
}

// getEnvOrDefault returns the value of an environment variable or a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
