// Package lorethread owns the lore session lifecycle, including classification,
// summarization, Discord thread creation, and worker orchestration.
//
// It defines explicit interfaces for session storage, message fetching, LLM
// classification/summarization, thread creation, rate limiting, and time
// management. The Service struct composes these interfaces and provides
// methods for session management and lore processing.
//
// No goroutines or tickers are started during construction or init.
// Callers must explicitly invoke Start() or RunOnce() to begin processing.
package lorethread
