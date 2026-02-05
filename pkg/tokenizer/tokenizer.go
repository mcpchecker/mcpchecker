package tokenizer

import (
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

// tokenizer provides token counting using cl100k_base encoding.
// This encoding is used by GPT-4/GPT-3.5. For other models like Claude
// and Gemini, counts may differ by 10-30% due to different tokenization.
type tokenizer struct {
	enc     *tiktoken.Tiktoken
	initErr error
	mu      sync.Mutex
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

// initLocked lazily initializes the encoding. Assumes t.mu is already held.
// Caches both success and failure to prevent repeated initialization attempts.
func (t *tokenizer) initLocked() error {
	if t.enc != nil {
		return nil
	}
	if t.initErr != nil {
		return t.initErr
	}

	t.enc, t.initErr = tiktoken.GetEncoding("cl100k_base")
	return t.initErr
}

// CountTokens counts tokens in the given text.
func (t *tokenizer) CountTokens(text string) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.initLocked(); err != nil {
		return 0, err
	}

	tokens := t.enc.Encode(text, nil, nil)
	return len(tokens), nil
}

// EstimateTokens is a convenience function that uses the default tokenizer
func EstimateTokens(text string) (int, error) {
	return Get().CountTokens(text)
}
