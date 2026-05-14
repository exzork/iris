package lorethread

// MetricsHooks provides callback functions for emitting metrics at key points.
type MetricsHooks struct {
	OnSessionOpened         func()
	OnSessionSkipped        func()
	OnClassifierFailure     func()
	OnSummaryFailure        func()
	OnThreadCreated         func()
	OnRateCapHit            func()
	OnCompaction            func()
}

// NewMetricsHooks creates a new MetricsHooks that emits to the given Metrics instance.
func NewMetricsHooks(m *Metrics) *MetricsHooks {
	if m == nil {
		return &MetricsHooks{
			OnSessionOpened:     func() {},
			OnSessionSkipped:    func() {},
			OnClassifierFailure: func() {},
			OnSummaryFailure:    func() {},
			OnThreadCreated:     func() {},
			OnRateCapHit:        func() {},
			OnCompaction:        func() {},
		}
	}
	return &MetricsHooks{
		OnSessionOpened:     m.IncrementSessionsOpened,
		OnSessionSkipped:    m.IncrementSessionsSkipped,
		OnClassifierFailure: m.IncrementClassifierFailures,
		OnSummaryFailure:    m.IncrementSummaryFailures,
		OnThreadCreated:     m.IncrementThreadsCreated,
		OnRateCapHit:        m.IncrementRateCapHits,
		OnCompaction:        m.IncrementCompactions,
	}
}

// NoOpMetricsHooks returns a MetricsHooks that does nothing.
func NoOpMetricsHooks() *MetricsHooks {
	return &MetricsHooks{
		OnSessionOpened:     func() {},
		OnSessionSkipped:    func() {},
		OnClassifierFailure: func() {},
		OnSummaryFailure:    func() {},
		OnThreadCreated:     func() {},
		OnRateCapHit:        func() {},
		OnCompaction:        func() {},
	}
}
