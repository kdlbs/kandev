package agents

// OpenAICompatibleProviderSpec describes how one agent accepts a Kandev-injected
// OpenAI-compatible provider (base URL + bearer key). Returned by
// OpenAICompatibleProviderAgent.OpenAICompatibleProvider; a nil return means the
// agent has no generic OpenAI-compatible provider and the profile fields stay
// inert. See docs/specs/agents/system-design/openai-compatible-providers.md.
//
// Injection is performed over the ACP wire: agentctl advertises
// clientCapabilities.auth._meta.gateway=true, then sends
// authenticate({methodId: AuthMethodID, _meta: {gateway: {baseUrl, headers,
// providerName}}}) after initialize.
type OpenAICompatibleProviderSpec struct {
	// AuthMethodID is the ACP auth methodId Kandev authenticates with
	// (codex-acp: "gateway").
	AuthMethodID string
	// ProviderName is the label passed to the agent for the synthesized
	// provider.
	ProviderName string
	// KeyEnvVar is the environment variable the agent also reads as a
	// credential fallback (codex-acp: "OPENAI_API_KEY"). Kandev sets it to the
	// revealed key so the utility/inference path and the agent's own lookup
	// both work even without the gateway authenticate call.
	KeyEnvVar string
}

// OpenAICompatibleProviderAgent is an optional capability for agents whose CLI
// exposes a generic OpenAI-compatible provider that Kandev can configure from an
// agent profile.
type OpenAICompatibleProviderAgent interface {
	// OpenAICompatibleProvider returns the injection spec, or nil when the
	// agent does not support a generic OpenAI-compatible provider.
	OpenAICompatibleProvider() *OpenAICompatibleProviderSpec
}

// OpenAICompatibleProviderSpecFor returns the agent's provider spec when it
// implements the capability and returns a non-nil spec.
func OpenAICompatibleProviderSpecFor(a Agent) *OpenAICompatibleProviderSpec {
	if p, ok := a.(OpenAICompatibleProviderAgent); ok {
		return p.OpenAICompatibleProvider()
	}
	return nil
}
