package usage

import (
	"sync"

	"github.com/openai/openai-go/v2"
)

// TokenUsage tracks token consumption across one or more API calls.
type TokenUsage struct {
	mu sync.Mutex

	input  int
	output int
}

func (u *TokenUsage) GetInput() int {
	u.mu.Lock()
	defer u.mu.Unlock()

	return u.input
}

func (u *TokenUsage) GetOutput() int {
	u.mu.Lock()
	defer u.mu.Unlock()

	return u.output
}

// Add combines another TokenUsage into this one.
func (u *TokenUsage) Add(other *TokenUsage) {
	if other == nil {
		return
	}

	other.mu.Lock()
	otherInput := other.input
	otherOutput := other.output
	other.mu.Unlock()

	u.mu.Lock()
	defer u.mu.Unlock()

	u.input += otherInput
	u.output += otherOutput
}

// FromOpenAIUsage converts OpenAI SDK usage to TokenUsage.
func FromOpenAIUsage(openaiUsage openai.CompletionUsage) *TokenUsage {
	return &TokenUsage{
		input:  int(openaiUsage.PromptTokens),
		output: int(openaiUsage.CompletionTokens),
	}
}
