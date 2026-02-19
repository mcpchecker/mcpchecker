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
	return u.input
}

func (u *TokenUsage) GetOutput() int {
	return u.output
}

// Add combines another TokenUsage into this one.
func (u *TokenUsage) Add(other *TokenUsage) {
	if other == nil || u == other {
		return
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	other.mu.Lock()
	defer other.mu.Unlock()

	u.input += other.input
	u.output += other.output
}

// FromOpenAIUsage converts OpenAI SDK usage to TokenUsage.
// The resulting TokenUsage represents a single API call (TotalCalls=1).
func FromOpenAIUsage(openaiUsage openai.CompletionUsage) *TokenUsage {
	return &TokenUsage{
		input:  int(openaiUsage.PromptTokens),
		output: int(openaiUsage.CompletionTokens),
	}
}
