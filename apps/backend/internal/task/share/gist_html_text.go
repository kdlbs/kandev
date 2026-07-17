package share

import (
	"html"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

var shareMarkdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

// writeHTMLText renders conversation text as GitHub-flavoured Markdown.
// Goldmark's safe default renderer omits embedded HTML and dangerous link
// destinations, which matters because shared snapshots contain agent and user
// supplied text.
func writeHTMLText(b *strings.Builder, text string) {
	t := strings.TrimSpace(text)
	if t == "" {
		return
	}
	b.WriteString("<div class=\"text\">\n")
	if err := shareMarkdown.Convert([]byte(t), b); err != nil {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(t))
		b.WriteString("</p>\n")
	}
	b.WriteString("</div>\n")
}
