package usage

import (
	"testing"

	"github.com/openai/openai-go/v2"
	"github.com/stretchr/testify/assert"
)

func TestTokenUsageAdd(t *testing.T) {
	testCases := map[string]struct {
		initial  *TokenUsage
		toAdd    []*TokenUsage
		expected *TokenUsage
	}{
		"single addition": {
			initial: &TokenUsage{},
			toAdd: []*TokenUsage{
				{
					input:  1,
					output: 2,
				},
			},
			expected: &TokenUsage{
				input:  1,
				output: 2,
			},
		},
		"multiple additions": {
			initial: &TokenUsage{},
			toAdd: []*TokenUsage{
				{
					input:  1,
					output: 2,
				},
				{
					input:  1,
					output: 2,
				},
			},
			expected: &TokenUsage{
				input:  2,
				output: 4,
			},
		},
		"multiple additions not empty": {
			initial: &TokenUsage{
				input:  1,
				output: 2,
			},
			toAdd: []*TokenUsage{
				{
					input:  1,
					output: 2,
				},
			},
			expected: &TokenUsage{
				input:  2,
				output: 4,
			},
		},
		"nil addition": {
			initial: &TokenUsage{input: 100, output: 50},
			toAdd:   []*TokenUsage{nil},
			expected: &TokenUsage{
				input:  100,
				output: 50,
			},
		},
	}

	for tcName, tc := range testCases {
		t.Run(tcName, func(t *testing.T) {
			agg := tc.initial
			for _, usage := range tc.toAdd {
				agg.Add(usage)
			}

			assert.Equal(t, tc.expected, agg)
		})
	}
}

func TestTokenUsage_EdgeCase_AddSelf(t *testing.T) {
	tu := &TokenUsage{
		input:  1,
		output: 2,
	}

	tu.Add(tu)

	assert.Equal(t, 2, tu.GetInput())
	assert.Equal(t, 4, tu.GetOutput())
}

func TestFromOpenAIUsage(t *testing.T) {
	testCases := map[string]struct {
		input    openai.CompletionUsage
		expected *TokenUsage
	}{
		"basic conversion": {
			input: openai.CompletionUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
			expected: &TokenUsage{
				input:  100,
				output: 50,
			},
		},
		"zero values": {
			input: openai.CompletionUsage{
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
			},
			expected: &TokenUsage{
				input:  0,
				output: 0,
			},
		},
	}

	for tcName, tc := range testCases {
		t.Run(tcName, func(t *testing.T) {
			result := FromOpenAIUsage(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
