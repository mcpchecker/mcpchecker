package steps

import (
	"context"
	"encoding/json"
	"time"
)

const (
	DefaultTimeout = 5 * time.Minute
)

var (
	DefaultRegistry = &Registry{
		parsers:       make(map[string]Parser),
		prefixParsers: make(map[string]PrefixParser),
	}
)

type StepRunner interface {
	Execute(ctx context.Context, input *StepInput) (*StepOutput, error)
}

type StepInput struct {
	Env         map[string]string
	Workdir     string
	Agent       *AgentContext
	StepOutputs map[string]map[string]string // Maps step type to its outputs
}

type StepOutput struct {
	Type    string            `json:"type,omitempty"`
	Success bool              `json:"success"`
	Message string            `json:"message,omitempty"`
	Outputs map[string]string `json:"outputs,omitempty"`
	Error   string            `json:"error,omitempty"`
}

type AgentContext struct {
	Prompt string
	Output string
}

type StepConfig struct {
	ID     string                     `json:"id"`
	Config map[string]json.RawMessage `json:"_"`
}

func (cfg *StepConfig) UnmarshalJSON(data []byte) error {
	type Alias StepConfig
	tmp := &struct {
		*Alias
	}{
		Alias: (*Alias)(cfg),
	}

	if err := json.Unmarshal(data, tmp); err != nil {
		return err
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	delete(rawMap, "id")

	cfg.Config = rawMap

	return nil
}

func (cfg *StepConfig) MarshalJSON() ([]byte, error) {
	rawMap := make(map[string]json.RawMessage, len(cfg.Config)+1)
	for k, v := range cfg.Config {
		rawMap[k] = v
	}

	if cfg.ID != "" {
		idBytes, err := json.Marshal(cfg.ID)
		if err != nil {
			return nil, err
		}
		rawMap["id"] = idBytes
	}

	return json.Marshal(rawMap)
}

func init() {
	DefaultRegistry.Register("http", ParseHttpStep)
	DefaultRegistry.Register("script", ParseScriptStep)
	DefaultRegistry.Register("llmJudge", ParseLLMJudgeStep)
}
