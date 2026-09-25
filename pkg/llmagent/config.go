package llmagent

import (
	"fmt"
	"strings"
)

type Config struct {
	// Model specifies the provider and model in "provider:model-id" format
	// Supported providers: openai, anthropic, gemini
	// Example: "openai:gpt-5", "gemini:gemini-3-pro"
	Model string

	// SystemPrompt contains optional system instructions for the agent
	SystemPrompt string

	// UseResponsesAPI explicitly selects the OpenAI Responses API when set.
	// When unset, models known to require Responses are selected automatically.
	UseResponsesAPI *bool
}

func (cfg *Config) ParseModel() (provider, modelID string, err error) {
	parts := strings.SplitN(cfg.Model, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("model must be in 'provider:model-id' format (e.g. 'openai:gpt-5'), got %q", cfg.Model)
	}

	return parts[0], parts[1], nil
}
