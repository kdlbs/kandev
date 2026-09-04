package routingerr

import (
	"regexp"
	"strings"
)

// MaxRawExcerptBytes caps the sanitized excerpt before persistence.
const MaxRawExcerptBytes = 4096

const redactionMask = "***"

// redaction transforms a string, applied in sequence by applyRedactions.
type redaction func(string) string

// literalRedaction wraps a fixed regexp substitution as a redaction.
func literalRedaction(pattern, replace string) redaction {
	re := regexp.MustCompile(pattern)
	return func(s string) string { return re.ReplaceAllString(s, replace) }
}

// credentialRedactions matches only credential-shaped patterns: API keys,
// personal access tokens, auth headers, and password/secret/token
// assignments. It excludes the URL rewrite, the opaque session-id rule, the
// 32-plus-char catch-all, and home-path normalization, all of which are safe
// to apply to provider diagnostics but would mangle unrelated content (commit
// SHAs, UUIDs, base64 samples, URLs with paths) in user-authored text.
var credentialRedactions = []redaction{
	literalRedaction(`sk-[A-Za-z0-9_-]{12,}`, "sk-"+redactionMask),
	literalRedaction(`github_pat_[A-Za-z0-9_]{50,}`, "github_pat_"+redactionMask),
	literalRedaction(`ghp_[A-Za-z0-9]{30,}`, "ghp_"+redactionMask),
	// Kandev personal access tokens, including the ?token=<PAT> form used by
	// headerless WS clients.
	literalRedaction(`kandev_pat_[A-Za-z0-9_]+`, "kandev_pat_"+redactionMask),
	literalRedaction(`(?i)Bearer\s+[A-Za-z0-9._\-+/=]{20,}`, "Bearer "+redactionMask),
	literalRedaction(`(?i)Authorization:\s*[^\r\n]+`, "Authorization: "+redactionMask),
	literalRedaction(`--api-key[= ]\S+`, "--api-key "+redactionMask),
	redactAssignments,
	// URL userinfo (user:pass@host) carries a live credential even though the
	// rest of the URL does not. Unlike the full Sanitize tier's URL rewrite,
	// this masks only the userinfo and keeps the path and query intact. The
	// scheme is not restricted to http(s): any scheme://user:pass@ form
	// (postgres, mysql, redis, ...) carries the same live credential. The
	// userinfo class includes '@' so the match backtracks to the LAST '@'
	// before the next '/' or whitespace, matching how a real credential
	// embedding a raw '@' actually terminates.
	literalRedaction(`(?i)([a-z][a-z0-9+.-]*://)[^\s/]+@`, "$1"+redactionMask+"@"),
}

var redactions = append(append([]redaction{
	// Provider diagnostics may include account/workspace links and opaque
	// session identifiers. These are not useful recovery details and must not
	// cross the lifecycle or message boundaries.
	// Keep only the scheme and host. This preserves the existing safe-endpoint
	// contract used by MCP diagnostics while dropping paths, query strings, and
	// fragments that can carry account or workspace identifiers.
	literalRedaction(`(https?://)(?:[^@\s/]+@)?([^/\s?#]+)[^\s]*`, "$1$2"),
	literalRedaction(`\b(?:wrk|ses|run)_[A-Za-z0-9_-]+\b`, "[redacted-id]"),
}, credentialRedactions...), []redaction{
	literalRedaction(`[A-Za-z0-9+/=_-]{32,}`, redactionMask),
	literalRedaction(`/Users/[^/\s]+/`, "/Users/<redacted>/"),
	literalRedaction(`/home/[^/\s]+/`, "/home/<redacted>/"),
}...)

// credentialAssignmentKey matches a password/secret/token/api-key field name
// plus its `:`/`=` separator, but not the value: the value is consumed
// separately by scanValue, which needs to make quote- and delimiter-aware
// decisions a single regexp alternation cannot express reliably (see
// redactAssignments).
var credentialAssignmentKey = regexp.MustCompile(`(?i)["']?([A-Za-z0-9_.-]*(?:password|secret|token|api[_-]?key)[A-Za-z0-9_.-]*)["']?\s*[:=]\s*`)

// redactAssignments replaces each password/secret/token/api-key assignment
// with its key name and a mask, consuming the value with scanValue so an
// embedded quote or leading structural delimiter cannot truncate the match
// early and leave part of the credential in cleartext.
func redactAssignments(s string) string {
	matches := credentialAssignmentKey.FindAllStringSubmatchIndex(s, -1)
	if matches == nil {
		return s
	}
	var b strings.Builder
	last := 0
	for _, loc := range matches {
		start, end, keyStart, keyEnd := loc[0], loc[1], loc[2], loc[3]
		if start < last {
			// Already consumed as part of a preceding assignment's value.
			continue
		}
		b.WriteString(s[last:start])
		b.WriteString(s[keyStart:keyEnd])
		b.WriteString(": ")
		b.WriteString(redactionMask)
		last = end + scanValue(s[end:])
	}
	b.WriteString(s[last:])
	return b.String()
}

// delimiterBytes stop a bare (unquoted) value: a closing brace/bracket/paren,
// angle bracket, comma, or semicolon. A value directly followed by one of
// these, as in `{"password": "***"}` after an earlier redaction pass, must
// not consume it so repeated redaction stays idempotent.
const delimiterBytes = "}])>,;"

func isSpaceByte(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	default:
		return false
	}
}

func isDelimiterByte(c byte) bool {
	return strings.IndexByte(delimiterBytes, c) >= 0
}

// scanValue returns the byte length of the credential value starting at s[0].
// A value starting with a quote is scanned by scanQuoted; anything else,
// including a value whose first character is itself a structural delimiter
// (`,`, `}`, ...), is scanned by scanBare so at least one character is always
// consumed.
func scanValue(s string) int {
	if len(s) == 0 {
		return 0
	}
	if s[0] == '"' || s[0] == '\'' {
		if n, ok := scanQuoted(s); ok {
			return n
		}
	}
	return scanBare(s)
}

// scanBare consumes a bare (unquoted) value: at least one character
// regardless of its class, then any run of characters that are neither
// whitespace nor a structural delimiter.
func scanBare(s string) int {
	n := len(s)
	i := 0
	for i < n {
		c := s[i]
		if isSpaceByte(c) {
			break
		}
		if i > 0 && isDelimiterByte(c) {
			break
		}
		i++
	}
	return i
}

// scanQuoted consumes a quoted value starting at s[0], honoring backslash
// escapes so an escaped quote does not end the string early. It returns
// ok=false if no closing quote is found before a newline or the end of s, so
// the caller falls back to bare scanning (matching how an unterminated quote
// was already handled before this function existed).
//
// A closing quote only ends the value if what follows is a delimiter or the
// end of the string. Otherwise the quote did not actually terminate the
// credential (for example a value with an embedded, unescaped quote
// character followed directly by more content), and scanning continues via
// scanContinuation to find the real boundary.
func scanQuoted(s string) (int, bool) {
	n := len(s)
	quote := s[0]
	j := 1
	for j < n {
		switch {
		case s[j] == '\\' && j+1 < n:
			j += 2
		case s[j] == quote:
			j++
			return extendPastQuote(s, j), true
		case s[j] == '\n' || s[j] == '\r':
			return 0, false
		default:
			j++
		}
	}
	return 0, false
}

// extendPastQuote is called once a closing quote is found at s[:j]. See
// scanQuoted for when the scan needs to continue past that quote.
func extendPastQuote(s string, j int) int {
	n := len(s)
	if j >= n || isSpaceByte(s[j]) || isDelimiterByte(s[j]) {
		return j
	}
	return j + scanContinuation(s[j:])
}

// scanContinuation scans the trailing content directly after a quote that
// turned out not to terminate the value. It behaves like scanBare (stopping
// at whitespace or a structural delimiter) but re-enters scanQuoted on
// another quote character, since that one might be the real terminator.
func scanContinuation(s string) int {
	n := len(s)
	i := 0
	for i < n {
		c := s[i]
		if isSpaceByte(c) {
			return i
		}
		if c == '"' || c == '\'' {
			if extra, ok := scanQuoted(s[i:]); ok {
				return i + extra
			}
			i++
			continue
		}
		if isDelimiterByte(c) {
			return i
		}
		i++
	}
	return i
}

func applyRedactions(s string, rules []redaction) string {
	for _, r := range rules {
		s = r(s)
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
