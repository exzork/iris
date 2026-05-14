package safety

type SafetyPipeline struct {
	Injection *InjectionFilter
	Secrets   *SecretRedactor
	Output    *OutputFilter
}

func NewSafetyPipeline() *SafetyPipeline {
	secrets := NewSecretRedactor()
	output := NewOutputFilter()
	output.Redactor = secrets

	return &SafetyPipeline{
		Injection: NewInjectionFilter(),
		Secrets:   secrets,
		Output:    output,
	}
}

// SanitizeRetrieved wraps untrusted retrieved content with injection neutralization.
func (p *SafetyPipeline) SanitizeRetrieved(content string) string {
	if p == nil || p.Injection == nil {
		return NewInjectionFilter().Neutralize(content)
	}
	return p.Injection.Neutralize(content)
}

// SanitizeToolOutput redacts secrets and neutralizes injection in tool outputs.
func (p *SafetyPipeline) SanitizeToolOutput(content string) string {
	if p == nil {
		p = NewSafetyPipeline()
	}

	secrets := p.Secrets
	if secrets == nil {
		secrets = NewSecretRedactor()
	}
	redacted := secrets.Redact(content)

	injection := p.Injection
	if injection == nil {
		injection = NewInjectionFilter()
	}

	return injection.Neutralize(redacted)
}

// SanitizeFinalResponse applies final output filtering before Discord send.
func (p *SafetyPipeline) SanitizeFinalResponse(content string) FilterResult {
	if p == nil || p.Output == nil {
		return NewOutputFilter().Apply(content)
	}
	return p.Output.Apply(content)
}
