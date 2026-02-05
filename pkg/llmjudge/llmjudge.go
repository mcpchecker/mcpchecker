package llmjudge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/openai/openai-go/v2"
	openai_option "github.com/openai/openai-go/v2/option"
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
	Passed          bool   `json:"passed"`
	Reason          string `json:"reason"`
	FailureCategory string `json:"failureCategory"`
}

type llmJudge struct {
	client  openai.Client
	model   string
	baseUrl string
}

type claudeJudge struct {
	// Claude Code CLI doesn't need a client, it executes the 'claude' binary
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

	judgeType := cfg.Type()
	apiKey := cfg.ApiKey()
	model := cfg.ModelName()

	var missingVars []string
	if apiKey == "" {
		missingVars = append(missingVars, fmt.Sprintf("%s (API key)", cfg.Env.ApiKeyKey))
	}
	if model == "" {
		missingVars = append(missingVars, fmt.Sprintf("%s (model name)", cfg.Env.ModelNameKey))
	}

	if len(missingVars) > 0 {
		return nil, fmt.Errorf("missing required environment variables for LLM judge: %v", missingVars)
	}

	switch judgeType {
	case JudgeTypeClaude:
		return newClaudeJudge(cfg, apiKey, model)
	case JudgeTypeOpenAI:
		return newOpenAIJudge(cfg, apiKey, model)
	default:
		return nil, fmt.Errorf("unsupported judge type: %s (supported types: %s, %s)", judgeType, JudgeTypeOpenAI, JudgeTypeClaude)
	}
}

func newOpenAIJudge(cfg *LLMJudgeEvalConfig, apiKey, model string) (LLMJudge, error) {
	baseUrl := cfg.BaseUrl()
	if baseUrl == "" {
		return nil, fmt.Errorf("missing required environment variable: %s (base URL)", cfg.Env.BaseUrlKey)
	}

	client := openai.NewClient(
		openai_option.WithBaseURL(baseUrl),
		openai_option.WithAPIKey(apiKey),
	)

	return &llmJudge{
		client:  client,
		model:   model,
		baseUrl: baseUrl,
	}, nil
}

func newClaudeJudge(cfg *LLMJudgeEvalConfig, apiKey, model string) (LLMJudge, error) {
	// Verify that the 'claude' binary is available
	if _, err := exec.LookPath("claude"); err != nil {
		return nil, fmt.Errorf("'claude' binary not found in PATH. Please install Claude Code CLI: %w", err)
	}

	return &claudeJudge{}, nil
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

	return result, nil
}

func (j *llmJudge) ModelName() string {
	return j.model
}

func (j *claudeJudge) EvaluateText(ctx context.Context, judgeConfig *LLMJudgeStepConfig, prompt, output string) (*LLMJudgeResult, error) {
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

	// Construct the full prompt for Claude Code CLI
	fullPrompt := fmt.Sprintf(`%s

%s

Please respond with ONLY a JSON object in the following format (no other text):
{
  "passed": true or false,
  "reason": "detailed explanation",
  "failureCategory": "semantic_mismatch" or "missing_information" or "contains_extra_info" or "n/a"
}`, systemPrompt, userPrompt)

	// Execute Claude Code CLI
	cmd := exec.CommandContext(ctx, "claude", "--print", fullPrompt)
	cmd.Env = os.Environ()

	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to execute claude CLI: %w\nOutput: %s", err, string(outputBytes))
	}

	// Parse the JSON response
	responseText := strings.TrimSpace(string(outputBytes))

	// Try to extract JSON from the response (in case there's extra text)
	jsonStart := strings.Index(responseText, "{")
	jsonEnd := strings.LastIndex(responseText, "}")

	if jsonStart == -1 || jsonEnd == -1 || jsonEnd < jsonStart {
		return nil, fmt.Errorf("no valid JSON found in Claude response: %s", responseText)
	}

	jsonText := responseText[jsonStart : jsonEnd+1]

	result := &LLMJudgeResult{}
	err = json.Unmarshal([]byte(jsonText), result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal Claude response as JSON: %w\nResponse: %s", err, jsonText)
	}

	return result, nil
}

func (j *claudeJudge) ModelName() string {
	return "claude-code-cli"
}
