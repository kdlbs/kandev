package streams

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// MaxProviderMessageBytes bounds a sanitized provider diagnostic message.
const MaxProviderMessageBytes = 2048

// MaxCompleteProviderDiagnosticBytes caps the raw text used to attest an exact
// provider diagnostic identity. The sanitized display message may be larger.
const MaxCompleteProviderDiagnosticBytes = 1024

var (
	providerMessageURLPattern        = regexp.MustCompile(`(?i)https?://[^\s]+`)
	providerMessageIdentifierPattern = regexp.MustCompile(`(?i)\b(?:wrk|ses|run)_[A-Za-z0-9_-]+\b`)
)

// SanitizeProviderMessage redacts likely credentials (via
// routingerr.SanitizeFullUnbounded), strips URLs, redacts workspace/session/run
// identifiers, collapses internal whitespace, and trims trailing punctuation
// from a raw provider-supplied error string, bounding it to
// MaxProviderMessageBytes. It is the single sanitized-projection transform the
// ACP transport layer and the recovery-evidence layer must observe
// identically, so both call this rather than each keeping their own copy. A
// raw ACP RequestError.Message is adapter-defined and may itself embed a
// credential the same way error.data can, so credential redaction runs before
// any other transform. The unbounded tier is used because this function
// applies its own MaxProviderMessageBytes cut below, on top of any redaction
// the shared tier already performed.
func SanitizeProviderMessage(message string) string {
	message = routingerr.SanitizeFullUnbounded(message)
	message = providerMessageURLPattern.ReplaceAllString(message, "")
	message = providerMessageIdentifierPattern.ReplaceAllString(message, "[redacted]")
	message = strings.Join(strings.Fields(message), " ")
	message = strings.TrimSpace(strings.TrimRight(message, ".:;,-"))
	if len(message) > MaxProviderMessageBytes {
		message = message[:MaxProviderMessageBytes]
	}
	return message
}

// IsCompleteProviderDiagnostic reports whether raw is safe to use as an exact
// diagnostic identity. Identity is complete only when the original valid
// UTF-8 text is short and the display sanitizer leaves it unchanged.
func IsCompleteProviderDiagnostic(raw string) bool {
	return raw != "" && utf8.ValidString(raw) &&
		len(raw) <= MaxCompleteProviderDiagnosticBytes &&
		SanitizeProviderMessage(raw) == raw
}

const (
	ProviderErrorSourceOpenCodeStderr = "opencode_stderr"
	// ProviderErrorSourceOpenCodeACP marks a provider diagnostic projected
	// from a structured ACP service-failure response.
	ProviderErrorSourceOpenCodeACP = "opencode_acp"
	// ProviderErrorSourceCodexACP marks a safe diagnostic correlated with a
	// terminal Codex ACP prompt error.
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
	// DiagnosticIdentityComplete is set only when the adapter extracted the
	// complete raw diagnostic and sanitization made no change to it.
	DiagnosticIdentityComplete bool `json:"diagnostic_identity_complete,omitempty"`
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
