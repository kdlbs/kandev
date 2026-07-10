package share

import (
	"fmt"
	"html"
	"strings"
)

// kandevRepoURL is the public GitHub repo for the project — used in the
// share page's "Try kandev" CTA and brand link. Pointing at the repo
// instead of a marketing site signals "this is open source, here's the
// code" which is the actual value proposition for the audience that
// receives a shared task.
const kandevRepoURL = "https://github.com/kdlbs/kandev"

// BuildShareHTML produces a self-contained styled HTML page that renders
// the snapshot as a real chat conversation: user bubbles right-aligned,
// assistant content left-aligned, consecutive same-role messages fused
// into one block, and tool calls collapsed inline rather than rendered
// as their own messages.
func BuildShareHTML(snap *Snapshot) string {
	if snap == nil {
		return "<!doctype html><title>kandev share</title>"
	}
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n")
	writeHTMLHead(&b, snap)
	b.WriteString("<body>\n")
	// Duplicate the stylesheet at the top of <body> as defense-in-depth: some
	// gist-rendering proxies copy only the body content into their own page,
	// dropping <head> and any <style> tags it contains. gist.githack.com
	// serves the raw file unchanged so <head> styles work, but we keep the
	// inline copy in case we ever route through a body-only renderer (or one
	// is added by a downstream embedder). <style> in <body> is non-conforming
	// HTML but every browser applies it without complaint, and CSS rules are
	// idempotent so the duplicate is harmless.
	b.WriteString("<style>")
	b.WriteString(shareCSS)
	b.WriteString("</style>\n")
	writeHTMLHero(&b, snap)
	b.WriteString("<main class=\"conv\">\n")
	writeHTMLConversation(&b, snap.Messages)
	b.WriteString("</main>\n")
	writeHTMLFooter(&b, snap)
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

func writeHTMLHead(b *strings.Builder, snap *Snapshot) {
	title := html.EscapeString(nonEmpty(snap.Task.Title, "Untitled task"))
	fmt.Fprintf(b, "<head>\n<meta charset=\"utf-8\">\n")
	fmt.Fprintf(b, "<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(b, "<title>%s — kandev share</title>\n", title)
	fmt.Fprintf(b, "<meta name=\"description\" content=\"%s\">\n", title)
	b.WriteString("<style>")
	b.WriteString(shareCSS)
	b.WriteString("</style>\n</head>\n")
}

func writeHTMLHero(b *strings.Builder, snap *Snapshot) {
	title := html.EscapeString(nonEmpty(snap.Task.Title, "Untitled task"))
	b.WriteString("<header class=\"hero\">\n")
	b.WriteString("<div class=\"brand\"><a href=\"" + kandevRepoURL + "\" target=\"_blank\" rel=\"noopener\">kandev</a>")
	b.WriteString("<span class=\"brand-sep\">·</span><span class=\"brand-tag\">shared task</span></div>\n")
	fmt.Fprintf(b, "<h1>%s</h1>\n", title)
	b.WriteString("<div class=\"badges\">")
	writeHTMLBadge(b, snap.Session.AgentType)
	writeHTMLBadge(b, snap.Session.Model)
	writeHTMLBadge(b, snap.Session.ExecutorType)
	writeHTMLBadge(b, fmt.Sprintf("%d messages", len(snap.Messages)))
	if completed := formatPtrTime(snap.Session.CompletedAt); completed != "" {
		writeHTMLBadge(b, completed)
	}
	b.WriteString("</div>\n")
	if len(snap.Redaction.AppliedRules) > 0 {
		fmt.Fprintf(b, "<p class=\"redaction\">🛡️ Redacted: <code>%s</code></p>\n",
			html.EscapeString(strings.Join(snap.Redaction.AppliedRules, ", ")))
	}
	b.WriteString("</header>\n")
}

func writeHTMLBadge(b *strings.Builder, text string) {
	if text == "" {
		return
	}
	fmt.Fprintf(b, "<span class=\"badge\">%s</span>", html.EscapeString(text))
}

// messageGroup is a run of consecutive messages sharing the same role.
// Blocks from every member are flattened in order so the whole run renders
// inside a single bubble.
type messageGroup struct {
	role   string
	blocks []Block
}

// groupMessages collapses consecutive same-role messages so the renderer
// can draw one bubble per group. Empty messages (no blocks) are dropped
// rather than producing empty groups.
func groupMessages(messages []Message) []messageGroup {
	var groups []messageGroup
	for _, m := range messages {
		if len(m.Blocks) == 0 {
			continue
		}
		if n := len(groups); n > 0 && groups[n-1].role == m.Role {
			groups[n-1].blocks = append(groups[n-1].blocks, m.Blocks...)
			continue
		}
		groups = append(groups, messageGroup{
			role:   m.Role,
			blocks: append([]Block(nil), m.Blocks...),
		})
	}
	return groups
}

func writeHTMLConversation(b *strings.Builder, messages []Message) {
	groups := groupMessages(messages)
	if len(groups) == 0 {
		b.WriteString("<p class=\"empty\">No messages.</p>\n")
		return
	}
	for _, g := range groups {
		writeHTMLGroup(b, g)
	}
}

func writeHTMLGroup(b *strings.Builder, g messageGroup) {
	cls, label, icon := messageRoleAttrs(g.role)
	fmt.Fprintf(b, "<section class=\"group group-%s\">\n", cls)
	fmt.Fprintf(b, "<div class=\"avatar\" aria-hidden=\"true\">%s</div>\n", icon)
	b.WriteString("<div class=\"bubble\">\n")
	fmt.Fprintf(b, "<div class=\"role\">%s</div>\n", html.EscapeString(label))
	for _, block := range g.blocks {
		writeHTMLBlock(b, block)
	}
	b.WriteString("</div>\n</section>\n")
}

func messageRoleAttrs(role string) (cls, label, icon string) {
	switch role {
	case roleUser:
		return "user", "You", "🧑"
	case roleAssistant:
		return "assistant", "Assistant", "🤖"
	case roleSystem:
		return "system", "System", "⚙️"
	}
	return "other", role, "•"
}

func writeHTMLBlock(b *strings.Builder, block Block) {
	switch block.Kind {
	case blockKindText:
		writeHTMLText(b, block.Text)
	case blockKindToolCall:
		writeHTMLToolCall(b, block)
	case blockKindToolResult:
		writeHTMLToolResult(b, block)
	case blockKindDiff:
		writeHTMLDiff(b, block)
	}
}

func writeHTMLToolCall(b *strings.Builder, block Block) {
	name := html.EscapeString(nonEmpty(block.ToolName, "tool"))
	summary := html.EscapeString(strings.TrimSpace(block.Text))
	// Closed by default — tool calls are noise unless the reader cares.
	b.WriteString("<details class=\"tool tool-call\">\n<summary>")
	b.WriteString("<span class=\"tool-icon\">🔧</span>")
	fmt.Fprintf(b, "<span class=\"tool-name\">%s</span>", name)
	if summary != "" {
		fmt.Fprintf(b, "<span class=\"tool-summary\">%s</span>", summary)
	}
	b.WriteString("<span class=\"tool-chev\" aria-hidden=\"true\">▸</span>")
	b.WriteString("</summary>\n")
	if len(block.Args) > 0 {
		fmt.Fprintf(b, "<pre class=\"args\"><code>%s</code></pre>\n",
			html.EscapeString(prettyJSON(block.Args)))
	}
	b.WriteString("</details>\n")
}

func writeHTMLToolResult(b *strings.Builder, block Block) {
	out := strings.TrimRight(block.Output, "\n")
	if strings.TrimSpace(out) == "" {
		return
	}
	label := "Tool output"
	if block.Truncated {
		label += " (truncated)"
	}
	b.WriteString("<details class=\"tool tool-result\">\n<summary>")
	b.WriteString("<span class=\"tool-icon\">📤</span>")
	fmt.Fprintf(b, "<span class=\"tool-name\">%s</span>", html.EscapeString(label))
	b.WriteString("<span class=\"tool-chev\" aria-hidden=\"true\">▸</span>")
	b.WriteString("</summary>\n")
	fmt.Fprintf(b, "<pre class=\"output\"><code>%s</code></pre>\n", html.EscapeString(out))
	b.WriteString("</details>\n")
}

func writeHTMLDiff(b *strings.Builder, block Block) {
	if strings.TrimSpace(block.UnifiedDiff) == "" {
		return
	}
	path := html.EscapeString(nonEmpty(block.Path, "diff"))
	fmt.Fprintf(b, "<div class=\"diff\">\n<div class=\"diff-head\"><span class=\"tool-icon\">📝</span>")
	fmt.Fprintf(b, "<code>%s</code></div>\n<pre class=\"diff-body\">", path)
	for _, line := range strings.Split(strings.TrimRight(block.UnifiedDiff, "\n"), "\n") {
		writeHTMLDiffLine(b, line)
	}
	b.WriteString("</pre>\n</div>\n")
}

func writeHTMLDiffLine(b *strings.Builder, line string) {
	cls := "diff-ctx"
	switch {
	case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
		cls = "diff-file"
	case strings.HasPrefix(line, "@@"):
		cls = "diff-hunk"
	case strings.HasPrefix(line, "+"):
		cls = "diff-add"
	case strings.HasPrefix(line, "-"):
		cls = "diff-del"
	}
	fmt.Fprintf(b, "<span class=\"%s\">%s</span>\n", cls, html.EscapeString(line))
}

func writeHTMLFooter(b *strings.Builder, snap *Snapshot) {
	b.WriteString("<footer class=\"page-footer\">\n")
	b.WriteString("<a href=\"" + kandevRepoURL + "\" target=\"_blank\" rel=\"noopener\" class=\"cta\">Try kandev on GitHub →</a>\n")
	b.WriteString("<span class=\"foot-sep\">·</span>\n")
	b.WriteString("<a href=\"snapshot.json\" class=\"foot-link\">snapshot.json</a>\n")
	if snap.KandevVersion != "" {
		fmt.Fprintf(b, "<span class=\"foot-sep\">·</span>\n<span class=\"foot-version\">kandev %s</span>\n",
			html.EscapeString(snap.KandevVersion))
	}
	b.WriteString("</footer>\n")
}
