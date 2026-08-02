package share

import (
	"fmt"
	"html"
	"strings"

	"github.com/kandev/kandev/internal/i18n"
)

// kandevRepoURL is the public GitHub repo for the project — used in the
// share page's "Try kandev" CTA and brand link. Pointing at the repo
// instead of a marketing site signals "this is open source, here's the
// code" which is the actual value proposition for the audience that
// receives a shared task.
const kandevRepoURL = "https://github.com/kdlbs/kandev"

// Catalog keys the HTML and Markdown builders share. Keys used in exactly
// one place stay inline at the call site; these are here because the same
// copy has to render identically in both artifacts.
const (
	keyUntitledTask        = "share.untitledTask"
	keyNoMessages          = "share.noMessages"
	keyMessageCount        = "share.messageCount"
	keyToolOutput          = "share.toolOutput"
	keyToolOutputTruncated = "share.toolOutputTruncated"
	keyRoleAssistant       = "share.roleAssistant"
	keyRoleSystem          = "share.roleSystem"
)

// toolOutputKey picks between the plain and truncated labels. Two whole
// messages rather than a translated "Tool output" with " (truncated)"
// appended: a suffix built in Go is a sentence fragment no translator can
// reorder, and several languages need to.
func toolOutputKey(truncated bool) string {
	if truncated {
		return keyToolOutputTruncated
	}
	return keyToolOutput
}

// BuildShareHTML produces a self-contained styled HTML page that renders
// the snapshot as a real chat conversation: user bubbles right-aligned,
// assistant content left-aligned, consecutive same-role messages fused
// into one block, and tool calls collapsed inline rather than rendered
// as their own messages.
//
// locale is the share creator's, captured when the share was created. The
// page is a static file on GitHub, so there is no request to resolve a
// locale from when a reader opens it later — see the package docs on
// internal/i18n.
func BuildShareHTML(snap *Snapshot, locale string) string {
	if snap == nil {
		return "<!doctype html><title>kandev share</title>"
	}
	var b strings.Builder
	// Normalize rather than trust the caller: <html lang> and every lookup
	// below must agree on the locale, and i18n.T normalizes internally.
	fmt.Fprintf(&b, "<!doctype html>\n<html lang=%q>\n", i18n.Normalize(locale))
	writeHTMLHead(&b, snap, locale)
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
	writeHTMLHero(&b, snap, locale)
	b.WriteString("<main class=\"conv\">\n")
	writeHTMLConversation(&b, snap.Messages, locale)
	b.WriteString("</main>\n")
	writeHTMLFooter(&b, snap, locale)
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// resolvedTitle is the escaped page title, falling back to the localized
// "Untitled task". Shared by the <head> and the hero so the fallback and the
// escaping can never drift between them.
func resolvedTitle(snap *Snapshot, locale string) string {
	return html.EscapeString(nonEmpty(snap.Task.Title, i18n.T(locale, keyUntitledTask)))
}

func writeHTMLHead(b *strings.Builder, snap *Snapshot, locale string) {
	title := resolvedTitle(snap, locale)
	fmt.Fprintf(b, "<head>\n<meta charset=\"utf-8\">\n")
	fmt.Fprintf(b, "<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	// The title is escaped before it goes into the message so the catalog
	// entry stays a plain template the translator can reorder.
	fmt.Fprintf(b, "<title>%s</title>\n", i18n.Tf(locale, "share.pageTitle", map[string]any{"title": title}))
	fmt.Fprintf(b, "<meta name=\"description\" content=\"%s\">\n", title)
	b.WriteString("<style>")
	b.WriteString(shareCSS)
	b.WriteString("</style>\n</head>\n")
}

func writeHTMLHero(b *strings.Builder, snap *Snapshot, locale string) {
	title := resolvedTitle(snap, locale)
	b.WriteString("<header class=\"hero\">\n")
	b.WriteString("<div class=\"brand\"><a href=\"" + kandevRepoURL + "\" target=\"_blank\" rel=\"noopener\">kandev</a>")
	fmt.Fprintf(b, "<span class=\"brand-sep\">·</span><span class=\"brand-tag\">%s</span></div>\n",
		html.EscapeString(i18n.T(locale, "share.brandTag")))
	fmt.Fprintf(b, "<h1>%s</h1>\n", title)
	b.WriteString("<div class=\"badges\">")
	writeHTMLBadge(b, snap.Session.AgentType)
	writeHTMLBadge(b, snap.Session.Model)
	writeHTMLBadge(b, snap.Session.ExecutorType)
	writeHTMLBadge(b, i18n.Tf(locale, keyMessageCount, map[string]any{"count": len(snap.Messages)}))
	if completed := formatPtrTime(snap.Session.CompletedAt); completed != "" {
		writeHTMLBadge(b, completed)
	}
	b.WriteString("</div>\n")
	if len(snap.Redaction.AppliedRules) > 0 {
		// The emoji is not copy, so it stays in the format string.
		fmt.Fprintf(b, "<p class=\"redaction\">🛡️ %s <code>%s</code></p>\n",
			html.EscapeString(i18n.T(locale, "share.redacted")),
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

func writeHTMLConversation(b *strings.Builder, messages []Message, locale string) {
	groups := groupMessages(messages)
	if len(groups) == 0 {
		fmt.Fprintf(b, "<p class=\"empty\">%s</p>\n", html.EscapeString(i18n.T(locale, keyNoMessages)))
		return
	}
	for _, g := range groups {
		writeHTMLGroup(b, g, locale)
	}
}

func writeHTMLGroup(b *strings.Builder, g messageGroup, locale string) {
	cls, label, icon := messageRoleAttrs(g.role, locale)
	fmt.Fprintf(b, "<section class=\"group group-%s\">\n", cls)
	fmt.Fprintf(b, "<div class=\"avatar\" aria-hidden=\"true\">%s</div>\n", icon)
	b.WriteString("<div class=\"bubble\">\n")
	fmt.Fprintf(b, "<div class=\"role\">%s</div>\n", html.EscapeString(label))
	for _, block := range g.blocks {
		writeHTMLBlock(b, block, locale)
	}
	b.WriteString("</div>\n</section>\n")
}

// messageRoleAttrs returns the CSS class, the display label, and the avatar
// for a role. Only the label is copy: cls is a class name the stylesheet
// keys off, icon is an emoji, and the default branch echoes the raw role
// straight from the message store — all three stay untranslated.
func messageRoleAttrs(role, locale string) (cls, label, icon string) {
	switch role {
	case roleUser:
		return "user", i18n.T(locale, "share.roleYou"), "🧑"
	case roleAssistant:
		return "assistant", i18n.T(locale, keyRoleAssistant), "🤖"
	case roleSystem:
		return "system", i18n.T(locale, keyRoleSystem), "⚙️"
	}
	return "other", role, "•"
}

func writeHTMLBlock(b *strings.Builder, block Block, locale string) {
	switch block.Kind {
	case blockKindText:
		writeHTMLText(b, block.Text)
	case blockKindToolCall:
		writeHTMLToolCall(b, block)
	case blockKindToolResult:
		writeHTMLToolResult(b, block, locale)
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

func writeHTMLToolResult(b *strings.Builder, block Block, locale string) {
	out := strings.TrimRight(block.Output, "\n")
	if strings.TrimSpace(out) == "" {
		return
	}
	label := i18n.T(locale, toolOutputKey(block.Truncated))
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

func writeHTMLFooter(b *strings.Builder, snap *Snapshot, locale string) {
	b.WriteString("<footer class=\"page-footer\">\n")
	// The arrow is decoration, not copy — it stays outside the message.
	fmt.Fprintf(b, "<a href=\"%s\" target=\"_blank\" rel=\"noopener\" class=\"cta\">%s →</a>\n",
		kandevRepoURL, html.EscapeString(i18n.T(locale, "share.ctaTryKandev")))
	b.WriteString("<span class=\"foot-sep\">·</span>\n")
	b.WriteString("<a href=\"snapshot.json\" class=\"foot-link\">snapshot.json</a>\n")
	if snap.KandevVersion != "" {
		fmt.Fprintf(b, "<span class=\"foot-sep\">·</span>\n<span class=\"foot-version\">kandev %s</span>\n",
			html.EscapeString(snap.KandevVersion))
	}
	b.WriteString("</footer>\n")
}
