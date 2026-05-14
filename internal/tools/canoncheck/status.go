package canoncheck

type Status string

const (
	StatusSupported        Status = "supported"
	StatusContradicted     Status = "contradicted"
	StatusUnsupported      Status = "unsupported"
	StatusNeedsMoreSources Status = "needs_more_sources"
)
