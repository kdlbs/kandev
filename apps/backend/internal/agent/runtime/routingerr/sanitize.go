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
	{regexp.MustCompile(`sk-[A-Za-z0-9_-]{12,}`), "sk-" + redactionMask},
	{regexp.MustCompile(`github_pat_[A-Za-z0-9_]{50,}`), "github_pat_" + redactionMask},
	{regexp.MustCompile(`ghp_[A-Za-z0-9]{30,}`), "ghp_" + redactionMask},
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
