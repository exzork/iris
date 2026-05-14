package lorethread

import (
	"sync/atomic"
)

// Metrics holds atomic counters for lore thread observability.
type Metrics struct {
	SessionsOpenedTotal       atomic.Int64
	SessionsSkippedTotal      atomic.Int64
	ClassifierFailuresTotal   atomic.Int64
	SummaryFailuresTotal      atomic.Int64
	ThreadsCreatedTotal       atomic.Int64
	RateCapHitsTotal          atomic.Int64
	CompactionsTotal          atomic.Int64
}

// NewMetrics creates a new Metrics instance.
func NewMetrics() *Metrics {
	return &Metrics{}
}

// IncrementSessionsOpened increments the sessions opened counter.
func (m *Metrics) IncrementSessionsOpened() {
	m.SessionsOpenedTotal.Add(1)
}

// IncrementSessionsSkipped increments the sessions skipped counter.
func (m *Metrics) IncrementSessionsSkipped() {
	m.SessionsSkippedTotal.Add(1)
}

// IncrementClassifierFailures increments the classifier failures counter.
func (m *Metrics) IncrementClassifierFailures() {
	m.ClassifierFailuresTotal.Add(1)
}

// IncrementSummaryFailures increments the summary failures counter.
func (m *Metrics) IncrementSummaryFailures() {
	m.SummaryFailuresTotal.Add(1)
}

// IncrementThreadsCreated increments the threads created counter.
func (m *Metrics) IncrementThreadsCreated() {
	m.ThreadsCreatedTotal.Add(1)
}

// IncrementRateCapHits increments the rate cap hits counter.
func (m *Metrics) IncrementRateCapHits() {
	m.RateCapHitsTotal.Add(1)
}

// IncrementCompactions increments the compactions counter.
func (m *Metrics) IncrementCompactions() {
	m.CompactionsTotal.Add(1)
}

// GetSnapshot returns a snapshot of current metric values.
func (m *Metrics) GetSnapshot() map[string]int64 {
	return map[string]int64{
		"sessions_opened":       m.SessionsOpenedTotal.Load(),
		"sessions_skipped":      m.SessionsSkippedTotal.Load(),
		"classifier_failures":   m.ClassifierFailuresTotal.Load(),
		"summary_failures":      m.SummaryFailuresTotal.Load(),
		"threads_created":       m.ThreadsCreatedTotal.Load(),
		"rate_cap_hits":         m.RateCapHitsTotal.Load(),
		"compactions":           m.CompactionsTotal.Load(),
	}
}
