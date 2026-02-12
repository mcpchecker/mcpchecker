package llmjudge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"

	"github.com/mcpchecker/mcpchecker/pkg/usage"
)

const (
	openaiSeed = 0 // allows for consistent eval results
)

var (
	submitJudgementFunction = openai.FunctionDefinitionParam{
		Name:        "submit_judgement",
		Description: openai.String(""),
		Parameters: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"passed": map[string]any{
					"type":        "boolean",
					"description": "Binary result: true for pass, false for fail",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "A detailed explanation for the score, referencing the evaluation criterion and the text",
				},
				"failureCategory": map[string]any{
					"type":        "string",
					"description": "If passed is false, specify the reason. Use 'n/a' if passing",
					"enum": []string{
						"semantic_mismatch",
						"missing_information",
						"contains_extra_info",
						"n/a",
					},
				},
			},
			"required": []string{"passed", "reason", "failureCategory"},
		},
	}
)

type LLMJudge interface {
	EvaluateText(ctx context.Context, judgeConfig *LLMJudgeStepConfig, prompt, output string) (*LLMJudgeResult, error)
	ModelName() string
}

type LLMJudgeResult struct {
	Passed          bool              `json:"passed"`
	Reason          string            `json:"reason"`
	FailureCategory string            `json:"failureCategory"`
	Usage           *usage.TokenUsage `json:"usage,omitempty"`
}

type llmJudge struct {
	client  openai.Client
	model   string
	baseUrl string
}

type noopLLMJudge struct{}

func (n *noopLLMJudge) EvaluateText(ctx context.Context, judgeConfig *LLMJudgeStepConfig, prompt, output string) (*LLMJudgeResult, error) {
	return &LLMJudgeResult{
		Passed:          true,
		Reason:          "noop judge always passes",
		FailureCategory: "n/a",
	}, nil
}

func (n *noopLLMJudge) ModelName() string {
	return "noop"
}

func NewLLMJudge(cfg *LLMJudgeEvalConfig) (LLMJudge, error) {
	if cfg == nil {
		return &noopLLMJudge{}, nil
	}
	if cfg.Env == nil {
		return nil, fmt.Errorf("llm judge env config is required to create an llm judge")
	}
	baseUrl := cfg.BaseUrl()
	apiKey := cfg.ApiKey()
	model := cfg.ModelName()

	var missingVars []string
	if baseUrl == "" {
		missingVars = append(missingVars, fmt.Sprintf("%s (base URL)", cfg.Env.BaseUrlKey))
	}
	if apiKey == "" {
		missingVars = append(missingVars, fmt.Sprintf("%s (API key)", cfg.Env.ApiKeyKey))
	}
	if model == "" {
		missingVars = append(missingVars, fmt.Sprintf("%s (model name)", cfg.Env.ModelNameKey))
	}

	if len(missingVars) > 0 {
		return nil, fmt.Errorf("missing required environment variables for LLM judge: %v", missingVars)
	}

	client := openai.NewClient(
		option.WithBaseURL(baseUrl),
		option.WithAPIKey(apiKey),
	)

	return &llmJudge{
		client:  client,
		model:   model,
		baseUrl: baseUrl,
	}, nil
}

// supportsSeed returns true if the LLM provider supports the seed parameter for deterministic outputs
func (j *llmJudge) supportsSeed() bool {
	// Gemini's OpenAI-compatible endpoint doesn't support the seed parameter as of v1beta.
	// Sending seed results in: "Invalid JSON payload received. Unknown name \"seed\": Cannot find field."
	// Reference: https://discuss.ai.google.dev/t/openai-compatibility-not-fully-implemented/95991
	// The OpenAI compatibility layer is still in beta with limited feature support.
	// Check if the base URL contains "generativelanguage.googleapis.com" (Gemini)
	return !strings.Contains(j.baseUrl, "generativelanguage.googleapis.com")
}

func (j *llmJudge) EvaluateText(ctx context.Context, judgeConfig *LLMJudgeStepConfig, prompt, output string) (*LLMJudgeResult, error) {
	systemPrompt, err := BuildSystemPrompt(SystemPromptData{
		EvaluationMode:  judgeConfig.EvaluationMode(),
		ReferenceAnswer: judgeConfig.ReferenceAnswer(),
	})
	if err != nil {
		return nil, err
	}

	userPrompt, err := BuildUserPrompt(UserPromptData{
		UserPrompt:    prompt,
		ModelResponse: output,
	})
	if err != nil {
		return nil, err
	}

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Tools: []openai.ChatCompletionToolUnionParam{
			{
				OfFunction: &openai.ChatCompletionFunctionToolParam{
					Function: submitJudgementFunction,
				},
			},
		},
		ToolChoice: openai.ToolChoiceOptionFunctionToolChoice(openai.ChatCompletionNamedToolChoiceFunctionParam{Name: submitJudgementFunction.Name}),
		Model:      j.model,
	}

	// Only include seed parameter for providers that support it (e.g., OpenAI, Mistral).
	// Gemini's v1beta OpenAI-compatible endpoint rejects the seed parameter with a 400 error.
	// See: https://discuss.ai.google.dev/t/openai-compatibility-not-fully-implemented/95991
	if j.supportsSeed() {
		params.Seed = openai.Int(openaiSeed)
	}

	completion, err := j.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to call llm judge: %w", err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no completion choices returned from LLM")
	}

	toolCalls := completion.Choices[0].Message.ToolCalls

	if len(toolCalls) != 1 {
		return nil, fmt.Errorf("failed to call the correct number of tools, expected 1 call, got %d", len(toolCalls))
	}

	toolCall := toolCalls[0]

	if toolCall.Function.Name != submitJudgementFunction.Name {
		return nil, fmt.Errorf("llm judge failed to call '%s' tool, called '%s' instead", submitJudgementFunction.Name, toolCall.Function.Name)
	}

	result := &LLMJudgeResult{}

	err = json.Unmarshal([]byte(toolCall.Function.Arguments), result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall '%s' tool call arguments: %w", submitJudgementFunction.Name, err)
	}

	result.Usage = usage.FromOpenAIUsage(completion.Usage)

	return result, nil
}

func (j *llmJudge) ModelName() string {
	return j.model
}
