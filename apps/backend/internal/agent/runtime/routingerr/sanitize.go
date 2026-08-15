package routingerr

import "regexp"

// MaxRawExcerptBytes caps the sanitized excerpt before persistence.
const MaxRawExcerptBytes = 4096

const redactionMask = "***"

type redaction struct {
	pattern *regexp.Regexp
	replace string
	// keep exempts matches that are known-safe identifiers. Only the catch-all
	// high-entropy rule uses it; typed credential rules must never exempt.
	keep *regexp.Regexp
}

// resourceIDPattern is a canonical UUID, the shape every Kandev task, session,
// workspace, and repository ID takes. These already appear in URLs and the UI,
// and masking them turns "repository X is misconfigured" into an error that can
// only be diagnosed by querying the database. No credential kandev issues is
// UUID-shaped.
var resourceIDPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
)

var redactions = []redaction{
	// Provider diagnostics may include account/workspace links and opaque
	// session identifiers. These are not useful recovery details and must not
	// cross the lifecycle or message boundaries.
	// Keep only the scheme and host. This preserves the existing safe-endpoint
	// contract used by MCP diagnostics while dropping paths, query strings, and
	// fragments that can carry account or workspace identifiers.
	{pattern: regexp.MustCompile(`(https?://)(?:[^@\s/]+@)?([^/\s?#]+)[^\s]*`), replace: "$1$2"},
	{pattern: regexp.MustCompile(`\b(?:wrk|ses|run)_[A-Za-z0-9_-]+\b`), replace: "[redacted-id]"},
	{pattern: regexp.MustCompile(`sk-[A-Za-z0-9_-]{12,}`), replace: "sk-" + redactionMask},
	{pattern: regexp.MustCompile(`github_pat_[A-Za-z0-9_]{50,}`), replace: "github_pat_" + redactionMask},
	{pattern: regexp.MustCompile(`ghp_[A-Za-z0-9]{30,}`), replace: "ghp_" + redactionMask},
	// Kandev personal access tokens, including the ?token=<PAT> form used by
	// headerless WS clients. The generic token/32-char rules below also catch
	// these; this keeps the credential type visible in redacted logs.
	{pattern: regexp.MustCompile(`kandev_pat_[A-Za-z0-9_]+`), replace: "kandev_pat_" + redactionMask},
	{pattern: regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._\-+/=]{20,}`), replace: "Bearer " + redactionMask},
	{pattern: regexp.MustCompile(`(?i)Authorization:\s*[^\r\n]+`), replace: "Authorization: " + redactionMask},
	{pattern: regexp.MustCompile(`--api-key[= ]\S+`), replace: "--api-key " + redactionMask},
	{pattern: regexp.MustCompile(`(?i)(password|secret|token)\s*[:=]\s*\S+`), replace: "$1: " + redactionMask},
	{pattern: regexp.MustCompile(`[A-Za-z0-9+/=_-]{32,}`), replace: redactionMask, keep: resourceIDPattern},
	{pattern: regexp.MustCompile(`/Users/[^/\s]+/`), replace: "/Users/<redacted>/"},
	{pattern: regexp.MustCompile(`/home/[^/\s]+/`), replace: "/home/<redacted>/"},
}

// Sanitize redacts likely credentials, normalizes home paths, and truncates
// to MaxRawExcerptBytes. The function is idempotent: applying it twice
// equals applying it once.
func Sanitize(s string) string {
	for _, r := range redactions {
		if r.keep == nil {
			s = r.pattern.ReplaceAllString(s, r.replace)
			continue
		}
		s = r.pattern.ReplaceAllStringFunc(s, func(match string) string {
			if r.keep.MatchString(match) {
				return match
			}
			return r.replace
		})
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
