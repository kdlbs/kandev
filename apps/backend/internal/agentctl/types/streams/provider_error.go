package streams

import "time"

const (
	ProviderErrorSourceOpenCodeStderr = "opencode_stderr"
	// ProviderErrorSourceOpenCodeACP marks a provider diagnostic projected
	// from a structured ACP service-failure response.
	ProviderErrorSourceOpenCodeACP = "opencode_acp"
	// ProviderErrorSourceCodexACP marks a safe diagnostic reconstructed from
	// Codex ACP metadata and its matching capacity message.
	ProviderErrorSourceCodexACP = "codex_acp"
	// ProviderErrorSourceCursorACP marks Cursor's bounded HTTP/2 stream-reset
	// diagnostic reconstructed from its terminal ACP control chunk.
	ProviderErrorSourceCursorACP = "cursor_acp"
)

// ProviderError is the bounded, sanitized provider diagnostic that may cross
// the agentctl boundary. It intentionally contains no raw stderr or provider
// account/workspace identifiers. RemediationURL is only ever populated by the
// adapter-specific allowlist validator; it is never reconstructed from prose.
type ProviderError struct {
	Source         string     `json:"source,omitempty"`
	ProviderID     string     `json:"provider_id,omitempty"`
	ModelID        string     `json:"model_id,omitempty"`
	Message        string     `json:"message,omitempty"`
	RemediationURL string     `json:"remediation_url,omitempty"`
	OccurredAt     time.Time  `json:"occurred_at,omitempty"`
	ResetAt        *time.Time `json:"reset_at,omitempty"`
}

func (e *ProviderError) Valid() bool {
	return e != nil &&
		e.Source != "" &&
		e.Message != "" &&
		!e.OccurredAt.IsZero()
}
