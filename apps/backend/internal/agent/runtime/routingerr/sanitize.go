package routingerr

import (
	"regexp"
	"strings"
)

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

var (
	localUnixPathPattern    = regexp.MustCompile(`/(?:[^/\s"']+/)+[^\r\n\s"'<>]+`)
	localWindowsPathPattern = regexp.MustCompile(`(?i)[A-Z]:[\\/][^\r\n"']+`)
)

func redactLocalPaths(s string) string {
	s = localWindowsPathPattern.ReplaceAllStringFunc(s, func(path string) string {
		if len(path) >= 4 && path[2:4] == "//" {
			return path
		}
		return "[path-redacted]"
	})
	matches := localUnixPathPattern.FindAllStringIndex(s, -1)
	if len(matches) == 0 {
		return s
	}

	var redacted strings.Builder
	redacted.Grow(len(s))
	last := 0
	for _, match := range matches {
		start, end := match[0], match[1]
		redacted.WriteString(s[last:start])
		path := s[start:end]
		if start >= 2 && s[start-2:start] == ":/" {
			redacted.WriteString(path)
			last = end
			continue
		}
		redacted.WriteString(redactUnixPath(path))
		last = end
	}
	redacted.WriteString(s[last:])
	return redacted.String()
}

func redactUnixPath(path string) string {
	if strings.Contains(path, "[path-redacted]") {
		return path
	}
	switch {
	case strings.HasPrefix(path, "/Users/"):
		return "/Users/<redacted>/[path-redacted]"
	case strings.HasPrefix(path, "/home/"):
		return "/home/<redacted>/[path-redacted]"
	default:
		return "[path-redacted]"
	}
}

// Sanitize redacts likely credentials and local paths, then truncates to
// MaxRawExcerptBytes. The function is idempotent: applying it twice equals
// applying it once.
func Sanitize(s string) string {
	for _, r := range redactions {
		s = r.pattern.ReplaceAllString(s, r.replace)
	}
	s = redactLocalPaths(s)
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
