package streams

import "time"

const ProviderErrorSourceOpenCodeStderr = "opencode_stderr"

// ProviderError is the bounded, sanitized provider diagnostic that may cross
// the agentctl boundary. It intentionally contains no raw stderr or provider
// account/workspace identifiers.
type ProviderError struct {
	Source     string     `json:"source,omitempty"`
	ProviderID string     `json:"provider_id,omitempty"`
	ModelID    string     `json:"model_id,omitempty"`
	Message    string     `json:"message,omitempty"`
	OccurredAt time.Time  `json:"occurred_at,omitempty"`
	ResetAt    *time.Time `json:"reset_at,omitempty"`
}

func (e *ProviderError) Valid() bool {
	return e != nil &&
		e.Source != "" &&
		e.Message != "" &&
		!e.OccurredAt.IsZero()
}
