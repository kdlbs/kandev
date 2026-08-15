package routingerr

import "regexp"

// MaxRawExcerptBytes caps the sanitized excerpt before persistence.
const MaxRawExcerptBytes = 4096

const redactionMask = "***"

type redaction struct {
	pattern *regexp.Regexp
	replace string
}

var redactions = []redaction{
	// Provider diagnostics may include account/workspace links and opaque
	// session identifiers. These are not useful recovery details and must not
	// cross the lifecycle or message boundaries.
	// Keep only the scheme and host. This preserves the existing safe-endpoint
	// contract used by MCP diagnostics while dropping paths, query strings, and
	// fragments that can carry account or workspace identifiers.
	{regexp.MustCompile(`(https?://)(?:[^@\s/]+@)?([^/\s?#]+)[^\s]*`), "$1$2"},
	{regexp.MustCompile(`\b(?:wrk|ses|run)_[A-Za-z0-9_-]+\b`), "[redacted-id]"},
	{regexp.MustCompile(`sk-[A-Za-z0-9_-]{12,}`), "sk-" + redactionMask},
	{regexp.MustCompile(`github_pat_[A-Za-z0-9_]{50,}`), "github_pat_" + redactionMask},
	{regexp.MustCompile(`ghp_[A-Za-z0-9]{30,}`), "ghp_" + redactionMask},
	// Kandev personal access tokens, including the ?token=<PAT> form used by
	// headerless WS clients. The generic token/32-char rules below also catch
	// these; this keeps the credential type visible in redacted logs.
	{regexp.MustCompile(`kandev_pat_[A-Za-z0-9_]+`), "kandev_pat_" + redactionMask},
	{regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._\-+/=]{20,}`), "Bearer " + redactionMask},
	{regexp.MustCompile(`(?i)Authorization:\s*[^\r\n]+`), "Authorization: " + redactionMask},
	{regexp.MustCompile(`--api-key[= ]\S+`), "--api-key " + redactionMask},
	{regexp.MustCompile(`(?i)(password|secret|token)\s*[:=]\s*\S+`), "$1: " + redactionMask},
	{regexp.MustCompile(`[A-Za-z0-9+/=_-]{32,}`), redactionMask},
	{regexp.MustCompile(`/Users/[^/\s]+/`), "/Users/<redacted>/"},
	{regexp.MustCompile(`/home/[^/\s]+/`), "/home/<redacted>/"},
}

// Sanitize redacts likely credentials, normalizes home paths, and truncates
// to MaxRawExcerptBytes. The function is idempotent: applying it twice
// equals applying it once.
func Sanitize(s string) string {
	for _, r := range redactions {
		s = r.pattern.ReplaceAllString(s, r.replace)
	}
	if len(s) > MaxRawExcerptBytes {
		s = s[:MaxRawExcerptBytes]
	}
	return s
}

type sanitizedError struct {
	cause   error
	message string
}

func (e *sanitizedError) Error() string { return e.message }
func (e *sanitizedError) Unwrap() error { return e.cause }

// SanitizeError redacts an error message while preserving errors.Is/errors.As
// access to the original cause for control flow.
func SanitizeError(err error) error {
	if err == nil {
		return nil
	}
	message := Sanitize(err.Error())
	if message == err.Error() {
		return err
	}
	return &sanitizedError{cause: err, message: message}
}
