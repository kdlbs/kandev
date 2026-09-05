package streams

import (
	"regexp"
	"strings"
	"time"
)

// MaxProviderMessageBytes bounds a sanitized provider diagnostic message.
const MaxProviderMessageBytes = 2048

var (
	providerMessageURLPattern        = regexp.MustCompile(`(?i)https?://[^\s]+`)
	providerMessageIdentifierPattern = regexp.MustCompile(`(?i)\b(?:wrk|ses|run)_[A-Za-z0-9_-]+\b`)
)

// SanitizeProviderMessage strips URLs, redacts workspace/session/run
// identifiers, collapses internal whitespace, and trims trailing punctuation
// from a raw provider-supplied error string, bounding it to
// MaxProviderMessageBytes. It is the single sanitized-projection transform
// the ACP transport layer and the recovery-evidence layer must observe
// identically, so both call this rather than each keeping their own copy.
func SanitizeProviderMessage(message string) string {
	message = providerMessageURLPattern.ReplaceAllString(message, "")
	message = providerMessageIdentifierPattern.ReplaceAllString(message, "[redacted]")
	message = strings.Join(strings.Fields(message), " ")
	message = strings.TrimSpace(strings.TrimRight(message, ".:;,-"))
	if len(message) > MaxProviderMessageBytes {
		message = message[:MaxProviderMessageBytes]
	}
	return message
}

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
	// ProviderErrorSourceACPPrompt marks a safe diagnostic projected from a
	// terminal ACP session/prompt JSON-RPC error.
	ProviderErrorSourceACPPrompt = "acp_prompt"
)

// ProviderError is the bounded, sanitized provider diagnostic that may cross
// the agentctl boundary. It intentionally contains no raw stderr or provider
// account/workspace identifiers. RemediationURL is only ever populated by the
// adapter-specific allowlist validator; it is never reconstructed from prose.
type ProviderError struct {
	Source     string `json:"source,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
	ModelID    string `json:"model_id,omitempty"`
	// RPCCode is the JSON-RPC error code from a terminal ACP prompt error,
	// carried verbatim. JSON-RPC forbids code 0, so 0 means absent.
	RPCCode int `json:"rpc_code,omitempty"`
	// ErrorKind is the adapter-declared error kind from a terminal ACP prompt
	// error's structured Data, allowlisted and validated before it crosses the
	// boundary.
	ErrorKind      string     `json:"error_kind,omitempty"`
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
