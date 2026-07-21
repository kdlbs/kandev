package share

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// BuildGistREADME renders the README that appears at the top of the gist
// page on github.com. The primary user-facing rendering is share.html
// (served via gist.githack.com); the README is mostly a "go to the pretty
// view" pointer plus a markdown fallback for anyone who lands on the gist
// directly. renderedURL is the gist.githack.com link to share.html; pass
// "" if it's not known yet (the README is regenerated post-upload with
// the real link).
func BuildGistREADME(snap *Snapshot, renderedURL string) string {
	if snap == nil {
		return "# kandev share\n"
	}
	var b strings.Builder
	writeHero(&b, snap)
	writeRenderedCTA(&b, renderedURL)
	writeMetaDetails(&b, snap)
	writeRedactionNote(&b, snap)
	b.WriteString("---\n\n")
	writeConversation(&b, snap.Messages)
	writeFooter(&b, snap)
	return b.String()
}

// writeRenderedCTA injects a prominent link to the styled HTML view so
// visitors who land on the raw gist know where to go.
func writeRenderedCTA(b *strings.Builder, renderedURL string) {
	if renderedURL == "" {
		b.WriteString("> 📖 **Open `share.html`** for the rendered conversation.\n\n")
		return
	}
	fmt.Fprintf(b, "> ✨ **[Open the rendered view →](%s)** for a properly styled conversation.\n\n", renderedURL)
}

func writeHero(b *strings.Builder, snap *Snapshot) {
	fmt.Fprintf(b, "# %s\n\n", nonEmpty(snap.Task.Title, "Untitled task"))
	// Compact badge line: short, dense, scannable.
	badges := []string{}
	if v := snap.Session.AgentType; v != "" {
		badges = append(badges, fmt.Sprintf("<kbd>%s</kbd>", v))
	}
	if v := snap.Session.Model; v != "" {
		badges = append(badges, fmt.Sprintf("<kbd>%s</kbd>", v))
	}
	if v := snap.Session.ExecutorType; v != "" {
		badges = append(badges, fmt.Sprintf("<kbd>%s</kbd>", v))
	}
	badges = append(badges, fmt.Sprintf("<kbd>%d messages</kbd>", len(snap.Messages)))
	fmt.Fprintf(b, "<sub>%s</sub>\n\n", strings.Join(badges, " · "))
	// Pitch line — also doubles as marketing.
	b.WriteString("> 🚀 Shared from **[kandev](https://github.com/kdlbs/kandev)** — the open-source agentic dev environment.\n\n")
}

func writeMetaDetails(b *strings.Builder, snap *Snapshot) {
	rows := []struct{ k, v string }{
		{"Agent", snap.Session.AgentType},
		{"Model", snap.Session.Model},
		{"Executor", snap.Session.ExecutorType},
		{"Started", formatTime(snap.Session.StartedAt)},
		{"Completed", formatPtrTime(snap.Session.CompletedAt)},
		{"Workflow step", snap.Task.WorkflowStep},
	}
	written := 0
	b.WriteString("<details>\n<summary>📊 Session details</summary>\n\n")
	b.WriteString("|   |   |\n|---|---|\n")
	for _, r := range rows {
		if r.v == "" {
			continue
		}
		fmt.Fprintf(b, "| **%s** | %s |\n", r.k, r.v)
		written++
	}
	if written == 0 {
		b.WriteString("| _no metadata_ | |\n")
	}
	b.WriteString("\n</details>\n\n")
}

func writeRedactionNote(b *strings.Builder, snap *Snapshot) {
	if len(snap.Redaction.AppliedRules) == 0 {
		return
	}
	parts := make([]string, len(snap.Redaction.AppliedRules))
	for i, r := range snap.Redaction.AppliedRules {
		parts[i] = "`" + r + "`"
	}
	fmt.Fprintf(b, "> 🛡️ **Redacted before publish:** %s\n\n", strings.Join(parts, ", "))
}

func writeConversation(b *strings.Builder, messages []Message) {
	if len(messages) == 0 {
		b.WriteString("_(No messages.)_\n\n")
		return
	}
	for i, msg := range messages {
		if i > 0 {
			b.WriteString("\n")
		}
		writeMessage(b, msg)
	}
}

func writeMessage(b *strings.Builder, msg Message) {
	fmt.Fprintf(b, "### %s\n\n", messageHeading(msg.Role))
	wroteAny := false
	for _, block := range msg.Blocks {
		if writeBlock(b, block, msg.Role) {
			wroteAny = true
		}
	}
	if !wroteAny {
		b.WriteString("_(empty)_\n\n")
	}
}

func messageHeading(role string) string {
	switch role {
	case roleUser:
		return "🧑 User"
	case roleAssistant:
		return "🤖 Assistant"
	case roleSystem:
		return "⚙️ System"
	}
	return role
}

// writeBlock writes a single block. Returns true if it produced any output —
// callers use this to detect "all blocks were empty" so they can show a
// placeholder instead of a bare heading.
func writeBlock(b *strings.Builder, block Block, role string) bool {
	switch block.Kind {
	case blockKindText:
		return writeText(b, block.Text, role)
	case blockKindToolCall:
		return writeToolCall(b, block)
	case blockKindToolResult:
		return writeToolResult(b, block)
	case blockKindDiff:
		return writeDiff(b, block)
	}
	return false
}

// writeText renders prose. User text is wrapped in a blockquote so the
// reader gets a visual accent that distinguishes the question from the
// agent's answer; assistant text is rendered as plain markdown so its
// own headings/lists/code blocks survive the round trip.
func writeText(b *strings.Builder, text, role string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	if role == roleUser {
		for _, line := range strings.Split(t, "\n") {
			fmt.Fprintf(b, "> %s\n", line)
		}
		b.WriteString("\n")
		return true
	}
	b.WriteString(t)
	b.WriteString("\n\n")
	return true
}

func writeToolCall(b *strings.Builder, block Block) bool {
	name := escapeHTML(nonEmpty(block.ToolName, "tool"))
	summary := strings.TrimSpace(block.Text)
	if summary != "" {
		fmt.Fprintf(b, "<details>\n<summary>🔧 <strong>%s</strong> — %s</summary>\n\n", name, escapeHTML(summary))
	} else {
		fmt.Fprintf(b, "<details>\n<summary>🔧 <strong>%s</strong></summary>\n\n", name)
	}
	if len(block.Args) > 0 {
		// Render via an HTML <pre> rather than a triple-backtick fence so a
		// JSON arg containing literal ``` sequences (commands, code snippets)
		// can't break out of the code block and corrupt downstream rendering.
		fmt.Fprintf(b, "<pre><code class=\"language-json\">%s</code></pre>\n",
			escapeHTML(prettyJSON(block.Args)))
	}
	b.WriteString("\n</details>\n\n")
	return true
}

func writeToolResult(b *strings.Builder, block Block) bool {
	if strings.TrimSpace(block.Output) == "" {
		return false
	}
	label := "📤 Tool output"
	if block.Truncated {
		label += " <em>(truncated)</em>"
	}
	// HTML <pre> avoids the triple-backtick collision when the tool output
	// itself contains a code fence.
	fmt.Fprintf(b, "<details>\n<summary>%s</summary>\n\n<pre><code>%s</code></pre>\n\n</details>\n\n",
		label, escapeHTML(strings.TrimRight(block.Output, "\n")))
	return true
}

func writeDiff(b *strings.Builder, block Block) bool {
	if strings.TrimSpace(block.UnifiedDiff) == "" {
		return false
	}
	path := nonEmpty(block.Path, "diff")
	fmt.Fprintf(b, "**📝 `%s`**\n\n```diff\n%s\n```\n\n", path, strings.TrimRight(block.UnifiedDiff, "\n"))
	return true
}

func writeFooter(b *strings.Builder, snap *Snapshot) {
	b.WriteString("\n---\n\n")
	b.WriteString("<sub>📦 Raw export: [`snapshot.json`](#file-snapshot-json) · ")
	b.WriteString("Built with [kandev](https://github.com/kdlbs/kandev)")
	if snap.KandevVersion != "" {
		fmt.Fprintf(b, " %s", snap.KandevVersion)
	}
	b.WriteString("</sub>\n")
}

// escapeHTML escapes characters that would break out of a <summary> tag.
// Markdown inside HTML is mostly inert, so this is enough.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func formatPtrTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatTime(*t)
}

func prettyJSON(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}
