package share

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var shareMarkdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(
		parser.WithASTTransformers(util.Prioritized(removeImagesTransformer{}, 1000)),
	),
)

type removeImagesTransformer struct{}

func (removeImagesTransformer) Transform(document *ast.Document, _ text.Reader, _ parser.Context) {
	removeImages(document)
}

func removeImages(node ast.Node) {
	for child := node.FirstChild(); child != nil; {
		next := child.NextSibling()
		if child.Kind() == ast.KindImage {
			node.RemoveChild(node, child)
		} else {
			removeImages(child)
		}
		child = next
	}
}

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
	_ = shareMarkdown.Convert([]byte(t), b)
	b.WriteString("</div>\n")
}
