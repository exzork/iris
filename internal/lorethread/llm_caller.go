package lorethread

import "context"

// LLMCaller abstracts LLM communication for testing and production use.
// Implementations must handle context timeouts and return errors on failure.
type LLMCaller interface {
	// Call invokes the LLM with a system prompt and user prompt.
	// Returns the LLM's response as a string, or an error if the call fails.
	Call(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}
