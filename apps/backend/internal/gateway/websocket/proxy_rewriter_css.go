package websocket

import (
	"strings"
)

// This file holds the CSS URL rewriter. It lives apart from
// proxy_rewriter.go so the latter stays under the revive file-length limit.

// rewriteCSSURLsAt rewrites url(/...) and @import "/..." occurrences inside a
// standalone CSS document, resolving relative references against the given
// base directory (the stylesheet file's public directory).
func rewriteCSSURLsAt(body []byte, prefix, capability, basePath string) []byte {
	return []byte(rewriteCSSFragment(string(body), prefix, capability, basePath))
}

// cssScanner rewrites CSS URL references inside an arbitrary string (either
// a full CSS file or the contents of an inline style attribute) with a small
// tokenizer so only REAL url() tokens and @import strings are rewritten.
// String literals and comments are copied untouched — a CSS string like
// content:'url(/x)' is data, not a URL reference — and url(var(--x)) is left
// alone because var() cannot be a URL token. Unquoted url() tokens run to
// whitespace or ')', which keeps data: URLs intact (their ';' and ',' are
// URL content, e.g. url(data:image/png;base64,AAA)). Function names and
// identifiers may use CSS escapes (\75rl( == url(), matching browser
// preprocessing). Path-absolute references get the proxy prefix plus the
// capability; relative references keep their resolution but still get the
// capability appended so cookie-less CSS loads stay authorized. Scheme-bearing
// (incl. data:), network-relative, and fragment-only references are left
// untouched by rewriteURLReference.
type cssScanner struct {
	out        strings.Builder
	css        string
	prefix     string
	capability string
	basePath   string
	i          int
}

func rewriteCSSFragment(css, prefix, capability, basePath string) string {
	s := &cssScanner{css: css, prefix: prefix, capability: capability, basePath: basePath}
	for s.i < len(css) {
		if s.consumeComment() || s.consumeString() || s.consumeURL() || s.consumeImport() {
			continue
		}
		s.out.WriteByte(s.css[s.i])
		s.i++
	}
	return s.out.String()
}

// consumeComment copies a /* ... */ comment (or the remainder when
// unterminated) and reports whether it consumed anything.
func (s *cssScanner) consumeComment() bool {
	css := s.css
	if s.i+1 >= len(css) || css[s.i] != '/' || css[s.i+1] != '*' {
		return false
	}
	if end := strings.Index(css[s.i+2:], "*/"); end >= 0 {
		s.out.WriteString(css[s.i : s.i+2+end+2])
		s.i += 2 + end + 2
	} else {
		s.out.WriteString(css[s.i:])
		s.i = len(css)
	}
	return true
}

// consumeString copies a quoted CSS string (escape-aware) and reports whether
// it consumed anything.
func (s *cssScanner) consumeString() bool {
	css := s.css
	c := css[s.i]
	if c != '\'' && c != '"' {
		return false
	}
	j := s.i + 1
	for j < len(css) {
		if css[j] == '\\' && j+1 < len(css) {
			j += 2
			continue
		}
		if css[j] == c {
			break
		}
		j++
	}
	if j >= len(css) {
		s.out.WriteString(css[s.i:])
		s.i = len(css)
		return true
	}
	s.out.WriteString(css[s.i : j+1])
	s.i = j + 1
	return true
}

// consumeURL rewrites a url() function token (raw or escape-written function
// name) and reports whether it consumed anything at s.i.
func (s *cssScanner) consumeURL() bool {
	css := s.css
	c := css[s.i]
	if c != 'u' && c != 'U' && c != '\\' {
		return false
	}
	if s.i > 0 && isCSSIdentChar(css[s.i-1]) {
		return false // noturl(/x) is not a url() token
	}
	open := cssFuncOpen(css, s.i, "url")
	if open < 0 {
		return false
	}
	// The original function-name bytes are preserved in the output.
	start := s.i
	j := open + 1
	for j < len(css) && isCSSSpace(css[j]) {
		j++
	}
	spacing := css[open+1 : j]
	if j < len(css) && (css[j] == '\'' || css[j] == '"') {
		s.quotedURL(start, open, j, spacing)
		return true
	}
	s.unquotedURL(start, open, j, spacing)
	return true
}

// quotedURL rewrites url('...') with an escape-aware quoted token.
func (s *cssScanner) quotedURL(start, open, j int, spacing string) {
	css := s.css
	q := css[j]
	k := j + 1
	for k < len(css) {
		if css[k] == '\\' && k+1 < len(css) {
			k += 2
			continue
		}
		if css[k] == q {
			break
		}
		k++
	}
	if k >= len(css) {
		s.out.WriteString(css[start:])
		s.i = len(css)
		return
	}
	l := k + 1
	for l < len(css) && isCSSSpace(css[l]) {
		l++
	}
	if l < len(css) && css[l] == ')' {
		s.out.WriteString(css[start:open+1] + spacing + string(q) + rewriteCSSURLToken(css[j+1:k], s.prefix, s.capability, s.basePath) + string(q) + css[k+1:l] + ")")
		s.i = l + 1
		return
	}
	s.out.WriteString(css[start : k+1])
	s.i = k + 1
}

// unquotedURL rewrites url(token) with an unquoted token (runs to
// whitespace or ')', but a whitespace after a hex escape is the escape's
// terminator and part of the token; var() is not a URL token and is
// preserved).
func (s *cssScanner) unquotedURL(start, open, j int, spacing string) {
	css := s.css
	k := j
	for k < len(css) && css[k] != ')' {
		if css[k] == '\\' {
			_, adv := cssEscapeDecode(css, k)
			k += adv
			continue
		}
		if isCSSSpace(css[k]) {
			break
		}
		k++
	}
	l := k
	for l < len(css) && isCSSSpace(css[l]) {
		l++
	}
	if l < len(css) && css[l] == ')' {
		tok := css[j:k]
		decoded := cssDecodeToken(tok)
		if !strings.HasPrefix(strings.ToLower(decoded), "var(") {
			s.out.WriteString(css[start:open+1] + spacing + rewriteCSSURLToken(tok, s.prefix, s.capability, s.basePath) + css[k:l] + ")")
			s.i = l + 1
			return
		}
		s.out.WriteString(css[start : l+1])
		s.i = l + 1
		return
	}
	s.out.WriteString(css[start:k])
	s.i = k
}

// consumeImport rewrites @import "…"; strings (comments are CSS whitespace
// and are accepted between @import and the string) and reports whether it
// consumed anything.
func (s *cssScanner) consumeImport() bool {
	css := s.css
	if s.i+7 > len(css) || css[s.i] != '@' || !strings.EqualFold(css[s.i:s.i+7], "@import") {
		return false
	}
	j := skipCSSSpaceAndComments(css, s.i+7)
	if j < len(css) && (css[j] == '\'' || css[j] == '"') {
		q := css[j]
		k := j + 1
		for k < len(css) {
			if css[k] == '\\' && k+1 < len(css) {
				k += 2
				continue
			}
			if css[k] == q {
				break
			}
			k++
		}
		if k >= len(css) {
			s.out.WriteString(css[s.i:])
			s.i = len(css)
			return true
		}
		s.out.WriteString(css[s.i:j]) // "@import " + whitespace/comments
		s.out.WriteString(string(q) + rewriteCSSURLToken(css[j+1:k], s.prefix, s.capability, s.basePath) + string(q))
		s.i = k + 1
		return true
	}
	// @import url(...) — the url() case rewrites it.
	return false
}

// cssDecodeToken decodes every CSS escape in a url() token value: the
// browser preprocesses escapes before resolving the URL, so classification
// must see the decoded form (url(\2f asset.css) is url(/asset.css)).
func cssDecodeToken(tok string) string {
	var b strings.Builder
	for i := 0; i < len(tok); {
		if tok[i] == '\\' {
			dec, adv := cssEscapeDecode(tok, i)
			b.WriteString(dec)
			i += adv
			continue
		}
		b.WriteByte(tok[i])
		i++
	}
	return b.String()
}

// cssEscapeToken re-escapes CSS-significant characters in a rewritten URL so
// the emitted token survives CSS tokenization: whitespace, quotes, parens,
// backslashes, and control characters become \XX hex escapes (the trailing
// space is the optional hex-escape terminator and is consumed by the parser).
// Plain proxy URLs need no escaping; this covers decoded-token rewrites whose
// decoded value contained CSS-significant characters.
func cssEscapeToken(tok string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		if c == '\\' || c == '\'' || c == '"' || c == '(' || c == ')' || isCSSSpace(c) || c < 0x20 || c == 0x7F {
			b.WriteByte('\\')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xF])
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// rewriteCSSURLToken classifies a url() token value (with CSS escapes
// decoded for classification) and returns the bytes to emit: the decoded
// rewritten URL when the reference changed, or the ORIGINAL raw token bytes
// when it did not (external/scheme/network-relative references keep their
// spelling).
func rewriteCSSURLToken(rawToken, prefix, capability, basePath string) string {
	decoded := cssDecodeToken(rawToken)
	rewritten := rewriteURLReferenceBase(decoded, prefix, capability, basePath)
	if rewritten == decoded {
		return rawToken
	}
	return cssEscapeToken(rewritten)
}

// isCSSSpace reports whether c is CSS whitespace (per the CSS syntax spec).
func isCSSSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	}
	return false
}

// isCSSIdentChar reports whether c can be part of a CSS identifier (per the
// CSS syntax spec's ident-token: ASCII letters, digits, '-', '_', and
// non-ASCII code points). Used to require a token boundary before `url(` so
// `noturl(/x)` is not mistaken for a url() token.
func isCSSIdentChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c >= 0x80
}

// isCSSNameStart reports whether c can start a CSS identifier: ASCII letters,
// '_', or a non-ASCII code point (digits and '-' are excluded per the CSS
// syntax spec; escapes are handled separately by cssIdentAt).
func isCSSNameStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c >= 0x80
}

func isCSSHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func cssHexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}

// cssEscapeDecode decodes one CSS escape at css[i] (css[i] must be '\'):
// a hex escape (\75, up to 6 hex digits, optional whitespace terminator)
// yields its code point; an escaped non-newline character yields itself; an
// escaped newline is removed (it is not a character). Returns the decoded
// text and the number of bytes consumed.
func cssEscapeDecode(css string, i int) (string, int) {
	if i+1 >= len(css) {
		return "", 1
	}
	switch c := css[i+1]; {
	case isCSSHexDigit(c):
		return cssHexEscape(css, i)
	case c == '\n' || c == '\r' || c == '\f':
		return "", 2
	default:
		return string(c), 2
	}
}

// cssHexEscape decodes the \XXXXXX form: at most 6 hex digits with an
// optional single whitespace terminator; invalid code points become U+FFFD.
func cssHexEscape(css string, i int) (string, int) {
	j := i + 1
	v := 0
	for j < len(css) && j-i < 7 && isCSSHexDigit(css[j]) {
		v = v*16 + cssHexVal(css[j])
		j++
	}
	if j < len(css) && isCSSSpace(css[j]) {
		j++ // consume the optional whitespace terminator
	}
	if v == 0 || v > 0x10FFFF || v >= 0xD800 && v <= 0xDFFF {
		return "\uFFFD", j - i
	}
	return string(rune(v)), j - i
}

// cssIdentAt decodes the identifier (raw or escape-containing) starting at
// css[i], returning the decoded name, the byte length consumed, and whether
// an identifier was actually found. \75rl( and u\72l( both decode to "url",
// matching how the CSS tokenizer preprocesses escapes before recognizing
// function names.
func cssIdentAt(css string, i int) (string, int, bool) {
	if i >= len(css) {
		return "", 0, false
	}
	var b strings.Builder
	j := i
	first := true
loop:
	for j < len(css) {
		c := css[j]
		switch {
		case c == '\\':
			dec, adv := cssEscapeDecode(css, j)
			if dec == "" {
				break loop
			}
			if !first && !isCSSIdentChar(dec[0]) {
				break loop
			}
			b.WriteString(dec)
			j += adv
		case first && isCSSNameStart(c):
			b.WriteByte(c)
			j++
		case !first && isCSSIdentChar(c):
			b.WriteByte(c)
			j++
		default:
			break loop
		}
		first = false
	}
	if j == i {
		return "", 0, false
	}
	return b.String(), j - i, true
}

// cssFuncOpen returns the index of the '(' that terminates a CSS function
// token with the given decoded name starting at css[i] (which may use CSS
// escapes, e.g. \75rl( decodes to "url"), or -1 when css[i] does not start
// such a function. The caller is responsible for the identifier boundary
// check on the raw previous byte.
func cssFuncOpen(css string, i int, name string) int {
	dec, adv, ok := cssIdentAt(css, i)
	if !ok || !strings.EqualFold(dec, name) {
		return -1
	}
	if i+adv < len(css) && css[i+adv] == '(' {
		return i + adv
	}
	return -1
}

// skipCSSSpaceAndComments advances j past CSS whitespace and comments (both
// are whitespace per the CSS syntax spec), returning the new position.
func skipCSSSpaceAndComments(css string, j int) int {
	for j < len(css) {
		if isCSSSpace(css[j]) {
			j++
			continue
		}
		if css[j] == '/' && j+1 < len(css) && css[j+1] == '*' {
			if end := strings.Index(css[j+2:], "*/"); end >= 0 {
				j += 2 + end + 2
				continue
			}
		}
		break
	}
	return j
}
