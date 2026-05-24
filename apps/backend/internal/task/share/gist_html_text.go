package share

import (
	"fmt"
	"html"
	"strings"
)

// writeHTMLText renders prose with minimal inline-markdown support: fenced
// code blocks (```lang … ```) become <pre><code class="lang-X"> blocks,
// and inline `code` spans become <code> tags. Anything else is escaped and
// wrapped in <p>. We intentionally stop short of a full markdown parser —
// the snapshot is plain agent output, not a publishing surface.
func writeHTMLText(b *strings.Builder, text string) {
	t := strings.TrimSpace(text)
	if t == "" {
		return
	}
	b.WriteString("<div class=\"text\">\n")
	for _, segment := range splitFencedCodeBlocks(t) {
		if segment.fenced {
			writeFencedCode(b, segment.lang, segment.body)
			continue
		}
		writeProse(b, segment.body)
	}
	b.WriteString("</div>\n")
}

// textSegment is either a fenced code block or a run of prose paragraphs.
type textSegment struct {
	fenced bool
	lang   string
	body   string
}

// splitFencedCodeBlocks walks `text` line by line and pulls fenced code
// blocks out as standalone segments so the surrounding prose can render
// as paragraphs. Recognises only the triple-backtick form; tilde fences
// and indent-based code blocks aren't worth the complexity here.
func splitFencedCodeBlocks(text string) []textSegment {
	var out []textSegment
	var prose []string
	var code []string
	inFence := false
	lang := ""
	flushProse := func() {
		if len(prose) > 0 {
			out = append(out, textSegment{body: strings.Join(prose, "\n")})
			prose = nil
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "```") {
			if inFence {
				out = append(out, textSegment{fenced: true, lang: lang, body: strings.Join(code, "\n")})
				code = nil
				lang = ""
				inFence = false
				continue
			}
			flushProse()
			lang = strings.TrimSpace(strings.TrimPrefix(line, "```"))
			inFence = true
			continue
		}
		if inFence {
			code = append(code, line)
		} else {
			prose = append(prose, line)
		}
	}
	if inFence { // unterminated fence — render what we got so we don't drop content
		out = append(out, textSegment{fenced: true, lang: lang, body: strings.Join(code, "\n")})
	}
	flushProse()
	return out
}

func writeFencedCode(b *strings.Builder, lang, body string) {
	body = strings.TrimRight(body, "\n")
	cls := "code"
	if lang != "" {
		cls = "code lang-" + sanitiseLang(lang)
	}
	fmt.Fprintf(b, "<pre class=\"%s\"><code>%s</code></pre>\n", cls, html.EscapeString(body))
}

// sanitiseLang keeps language tags to a safe character set so they can go
// into a class attribute without escaping. Anything outside [a-z0-9-_] is
// dropped.
func sanitiseLang(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// writeProse renders a prose run as paragraphs split on blank lines, with
// inline backtick spans converted to <code>. Soft newlines inside a
// paragraph become <br>.
func writeProse(b *strings.Builder, text string) {
	text = strings.Trim(text, "\n")
	if text == "" {
		return
	}
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		fmt.Fprintf(b, "<p>%s</p>\n", inlineFormat(para))
	}
}

// inlineFormat handles inline `code` and preserves soft line breaks as <br>.
// Everything else is HTML-escaped.
func inlineFormat(s string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, "`")
		if i < 0 {
			b.WriteString(htmlEscapeMultiline(s))
			return b.String()
		}
		b.WriteString(htmlEscapeMultiline(s[:i]))
		rest := s[i+1:]
		end := strings.Index(rest, "`")
		if end < 0 {
			// Unmatched backtick — render the rest as-is so we don't drop chars.
			b.WriteString(htmlEscapeMultiline(s[i:]))
			return b.String()
		}
		fmt.Fprintf(&b, "<code class=\"inline\">%s</code>", html.EscapeString(rest[:end]))
		s = rest[end+1:]
	}
}

// htmlEscapeMultiline escapes for HTML while preserving soft line breaks as
// <br> tags so paragraph internals keep their shape.
func htmlEscapeMultiline(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "\n", "<br>")
}
