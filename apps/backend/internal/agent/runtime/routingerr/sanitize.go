package routingerr

import "regexp"

// MaxRawExcerptBytes caps the sanitized excerpt before persistence.
const MaxRawExcerptBytes = 4096

const redactionMask = "***"

type redaction struct {
	pattern *regexp.Regexp
	replace string
}

// credentialRedactions matches only credential-shaped patterns: API keys,
// personal access tokens, auth headers, and password/secret/token
// assignments. It excludes the URL rewrite, the opaque session-id rule, the
// 32-plus-char catch-all, and home-path normalization, all of which are safe
// to apply to provider diagnostics but would mangle unrelated content (commit
// SHAs, UUIDs, base64 samples, URLs with paths) in user-authored text.
var credentialRedactions = []redaction{
	{regexp.MustCompile(`sk-[A-Za-z0-9_-]{12,}`), "sk-" + redactionMask},
	{regexp.MustCompile(`github_pat_[A-Za-z0-9_]{50,}`), "github_pat_" + redactionMask},
	{regexp.MustCompile(`ghp_[A-Za-z0-9]{30,}`), "ghp_" + redactionMask},
	// Kandev personal access tokens, including the ?token=<PAT> form used by
	// headerless WS clients.
	{regexp.MustCompile(`kandev_pat_[A-Za-z0-9_]+`), "kandev_pat_" + redactionMask},
	{regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._\-+/=]{20,}`), "Bearer " + redactionMask},
	{regexp.MustCompile(`(?i)Authorization:\s*[^\r\n]+`), "Authorization: " + redactionMask},
	{regexp.MustCompile(`--api-key[= ]\S+`), "--api-key " + redactionMask},
	// Tolerates an optional quote around the key, a qualifier prefix/suffix on
	// the keyword itself (AWS_SECRET_ACCESS_KEY, SECRET_KEY), and a quoted or
	// bare value. The value alternation consumes a quoted value through its
	// closing quote so an embedded quote character does not truncate the
	// match early and leave the value's tail in cleartext. The bare-value
	// fallback allows embedded quote characters for the same reason but stops
	// before a trailing structural delimiter (closing brace/bracket/paren,
	// comma, semicolon) so a value directly followed by one of those, as in
	// `{"password": "***"}` after an earlier redaction pass, does not consume
	// it and stays idempotent.
	{regexp.MustCompile(`(?i)["']?([A-Za-z0-9_.-]*(?:password|secret|token|api[_-]?key)[A-Za-z0-9_.-]*)["']?\s*[:=]\s*(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s}\])>,;]+)`), "$1: " + redactionMask},
	// URL userinfo (user:pass@host) carries a live credential even though the
	// rest of the URL does not. Unlike the full Sanitize tier's URL rewrite,
	// this masks only the userinfo and keeps the path and query intact. The
	// scheme is not restricted to http(s): any scheme://user:pass@ form
	// (postgres, mysql, redis, ...) carries the same live credential.
	{regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^@\s/]+@`), "$1" + redactionMask + "@"},
}

var redactions = append(append([]redaction{
	// Provider diagnostics may include account/workspace links and opaque
	// session identifiers. These are not useful recovery details and must not
	// cross the lifecycle or message boundaries.
	// Keep only the scheme and host. This preserves the existing safe-endpoint
	// contract used by MCP diagnostics while dropping paths, query strings, and
	// fragments that can carry account or workspace identifiers.
	{regexp.MustCompile(`(https?://)(?:[^@\s/]+@)?([^/\s?#]+)[^\s]*`), "$1$2"},
	{regexp.MustCompile(`\b(?:wrk|ses|run)_[A-Za-z0-9_-]+\b`), "[redacted-id]"},
}, credentialRedactions...), []redaction{
	{regexp.MustCompile(`[A-Za-z0-9+/=_-]{32,}`), redactionMask},
	{regexp.MustCompile(`/Users/[^/\s]+/`), "/Users/<redacted>/"},
	{regexp.MustCompile(`/home/[^/\s]+/`), "/home/<redacted>/"},
}...)

func applyRedactions(s string, rules []redaction) string {
	for _, r := range rules {
		s = r.pattern.ReplaceAllString(s, r.replace)
	}
	if len(s) > MaxRawExcerptBytes {
		s = s[:MaxRawExcerptBytes]
	}
	return s
}

// Sanitize redacts likely credentials, normalizes home paths, and truncates
// to MaxRawExcerptBytes. The function is idempotent: applying it twice
// equals applying it once. Use this for provider stdout/stderr and other
// diagnostic text, where collapsing opaque identifiers and paths is
// acceptable collateral.
func Sanitize(s string) string {
	return applyRedactions(s, redactions)
}

// SanitizeCredentials redacts only credential-shaped patterns (API keys,
// tokens, auth headers, password/secret/token assignments) and truncates to
// MaxRawExcerptBytes. Unlike Sanitize, it leaves URLs, opaque IDs, and any
// other 32-plus-char run untouched, so it is safe to apply to user-authored
// text (a task description, a plan) that must not carry a live credential
// across a provider boundary but should otherwise survive intact. It is
// idempotent for the same reason Sanitize is.
func SanitizeCredentials(s string) string {
	return applyRedactions(s, credentialRedactions)
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
