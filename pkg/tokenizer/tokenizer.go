package tokenizer

import (
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

// tokenizer provides token counting using cl100k_base encoding.
// This encoding is used by GPT-4/GPT-3.5. For other models like Claude
// and Gemini, counts may differ by 10-30% due to different tokenization.
type tokenizer struct {
	enc *tiktoken.Tiktoken
	mu  sync.Mutex
}

// Tokenizer is the public interface for token counting.
type Tokenizer interface {
	CountTokens(text string) (int, error)
}

var (
	defaultTokenizer *tokenizer
	once             sync.Once
)

// Get returns the singleton tokenizer instance.
func Get() Tokenizer {
	once.Do(func() {
		defaultTokenizer = &tokenizer{}
	})
	return defaultTokenizer
}

// init lazily initializes the encoding
func (t *tokenizer) init() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.enc != nil {
		return nil
	}

	var err error
	t.enc, err = tiktoken.GetEncoding("cl100k_base")
	return err
}

// CountTokens counts tokens in the given text.
func (t *tokenizer) CountTokens(text string) (int, error) {
	if err := t.init(); err != nil {
		return 0, err
	}

	tokens := t.enc.Encode(text, nil, nil)
	return len(tokens), nil
}

// EstimateTokens is a convenience function that uses the default tokenizer
func EstimateTokens(text string) (int, error) {
	return Get().CountTokens(text)
}
