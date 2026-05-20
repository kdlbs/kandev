// Package summary builds the per-agent continuation-summary markdown
// blob that bridges context across taskless runs (heartbeats,
// lightweight routines).
//
// PR 1 of office-heartbeat-rework introduced this package. The builder
// is server-synthesised — the agent does not write the prose. We
// compose the markdown deterministically from structured inputs
// (run.result_json, workspace activity counts, blocker tasks, prior
// summary) so summaries survive agent failures, stay consistent
// across CLIs, and don't depend on prompt instructions the agent
// may rotate.
//
// See docs/specs/office-heartbeat-rework/spec.md §"Continuation
// summary contract" for the section schema.
package summary

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// maxSummaryBytes mirrors the storage cap in the continuation
// summaries repo (8 KB). The builder applies it as a defensive
// upper bound so a runaway "Recent decisions" list can't blow past
// the cap. The repo also truncates on write — duplicating here keeps
// the produced blob equally valid for any in-memory consumer.
const maxSummaryBytes = 8192

// fallbackNextAction is what we put in the "Next action" section when
// the input table can't infer a concrete next step. Gives the
// heartbeat prompt something non-empty to lean on.
const fallbackNextAction = "Continue monitoring."

// BlockerInput describes one blocked task surfaced in the "Open
// blockers" section. Title is required; Reason and SurfacedAt are
// best-effort metadata that get inlined when present.
type BlockerInput struct {
	Title      string
	Reason     string
	SurfacedAt time.Time
}

// DecisionInput describes one entry in the "Recent decisions"
// section. Text is the verbatim line; At is the wall-clock time the
// decision was committed (used to date-stamp the bullet).
type DecisionInput struct {
	Text string
	At   time.Time
}

// ActivityStats are the workspace activity counts since the prior
// summary's updated_at. Used to populate the "Active focus" opening.
type ActivityStats struct {
	CompletedRuns int
	FailedRuns    int
	OpenTasks     int
	InProgress    int
}

// BuildInputs aggregates everything BuildSummary needs. Callers
// populate this via LoadInputs in inputs.go (or by hand in tests).
type BuildInputs struct {
	// AgentProfileID and Scope are surfaced for traceability — the
	// builder does not consume them for content but downstream callers
	// (logs, telemetry) often want them adjacent to the produced blob.
	AgentProfileID string
	Scope          string

	// ActiveFocus is a short (1-2 line) free-form description of what
	// the coordinator is currently driving. Falls back to the prior
	// summary's first line when empty, then to a default boilerplate.
	ActiveFocus string

	Activity   ActivityStats
	Blockers   []BlockerInput
	Decisions  []DecisionInput
	NextAction string

	// PreviousSummary is the prior agent_continuation_summaries.content.
	// Used as a fallback "Active focus" line when the current run
	// produced nothing concrete.
	PreviousSummary string

	// LastRunStatus is the run's terminal status ("finished", "failed",
	// etc.). Drives the inferred next-action when NextAction is empty.
	LastRunStatus string
}

// BuildSummary composes the markdown blob. Deterministic — same
// inputs produce the same output. The result is capped at 8 KB; the
// repo applies the same cap on write (defensive duplication).
func BuildSummary(in BuildInputs) string {
	var b strings.Builder
	writeActiveFocus(&b, &in)
	writeBlockers(&b, in.Blockers)
	writeDecisions(&b, in.Decisions)
	writeNextAction(&b, &in)

	out := strings.TrimRight(b.String(), "\n") + "\n"
	if len(out) > maxSummaryBytes {
		return truncateUTF8(out, maxSummaryBytes)
	}
	return out
}

func writeActiveFocus(b *strings.Builder, in *BuildInputs) {
	b.WriteString("## Active focus\n")
	if in.ActiveFocus != "" {
		b.WriteString(strings.TrimSpace(in.ActiveFocus))
		b.WriteString("\n\n")
		writeActivityLine(b, in.Activity)
		return
	}
	if line := firstNonEmptyLine(in.PreviousSummary, "## Active focus"); line != "" {
		b.WriteString(line)
		b.WriteString("\n\n")
		writeActivityLine(b, in.Activity)
		return
	}
	b.WriteString("Workspace coordination — watching tasks, blockers, and agent activity.\n\n")
	writeActivityLine(b, in.Activity)
}

func writeActivityLine(b *strings.Builder, a ActivityStats) {
	if a == (ActivityStats{}) {
		b.WriteString("No new workspace activity since the last fire.\n\n")
		return
	}
	fmt.Fprintf(b,
		"Workspace activity: %d completed run(s), %d failed run(s), %d in-progress, %d open task(s).\n\n",
		a.CompletedRuns, a.FailedRuns, a.InProgress, a.OpenTasks,
	)
}

func writeBlockers(b *strings.Builder, blockers []BlockerInput) {
	b.WriteString("## Open blockers\n")
	if len(blockers) == 0 {
		b.WriteString("None.\n\n")
		return
	}
	for i := range blockers {
		writeBlockerLine(b, &blockers[i])
	}
	b.WriteString("\n")
}

func writeBlockerLine(b *strings.Builder, blk *BlockerInput) {
	b.WriteString("- ")
	if blk.Title == "" {
		b.WriteString("(untitled blocker)")
	} else {
		b.WriteString(blk.Title)
	}
	if blk.Reason != "" {
		fmt.Fprintf(b, " — %s", blk.Reason)
	}
	if !blk.SurfacedAt.IsZero() {
		fmt.Fprintf(b, " (surfaced %s)", blk.SurfacedAt.UTC().Format("2006-01-02"))
	}
	b.WriteString("\n")
}

func writeDecisions(b *strings.Builder, decisions []DecisionInput) {
	b.WriteString("## Recent decisions\n")
	if len(decisions) == 0 {
		b.WriteString("None recorded.\n\n")
		return
	}
	// Sort newest first; cap at 5 to match the spec's "last ~5" bound.
	sorted := make([]DecisionInput, len(decisions))
	copy(sorted, decisions)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].At.After(sorted[j].At)
	})
	if len(sorted) > 5 {
		sorted = sorted[:5]
	}
	for i := range sorted {
		writeDecisionLine(b, &sorted[i])
	}
	b.WriteString("\n")
}

func writeDecisionLine(b *strings.Builder, d *DecisionInput) {
	if !d.At.IsZero() {
		fmt.Fprintf(b, "- [%s] %s\n", d.At.UTC().Format("2006-01-02"), d.Text)
		return
	}
	fmt.Fprintf(b, "- %s\n", d.Text)
}

func writeNextAction(b *strings.Builder, in *BuildInputs) {
	b.WriteString("## Next action\n")
	if in.NextAction != "" {
		b.WriteString(strings.TrimSpace(in.NextAction))
		b.WriteString("\n")
		return
	}
	b.WriteString(inferNextAction(in))
	b.WriteString("\n")
}

// inferNextAction is the small decision table promised by the spec.
// We key on (workspace state, last run status) and fall back to the
// monitoring boilerplate when nothing concrete fits.
func inferNextAction(in *BuildInputs) string {
	if len(in.Blockers) > 0 {
		return "Resolve open blockers before assigning new work."
	}
	if in.LastRunStatus == "failed" {
		return "Investigate the previous run failure and decide whether to retry or escalate."
	}
	if in.Activity.FailedRuns > 0 {
		return "Triage recent failed runs and assign follow-ups."
	}
	if in.Activity.OpenTasks > 0 {
		return "Review the open task queue and dispatch the next priority item."
	}
	return fallbackNextAction
}

// firstNonEmptyLine returns the first non-blank, non-heading line of s.
// skipPrefix is a section heading we want to ignore (e.g. "## Active
// focus") so we don't echo it back as content. Returns "" when no such
// line exists.
func firstNonEmptyLine(s, skipPrefix string) string {
	for _, raw := range strings.Split(s, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "## ") {
			continue
		}
		if skipPrefix != "" && strings.HasPrefix(line, skipPrefix) {
			continue
		}
		return line
	}
	return ""
}

// truncateUTF8 cuts s to at most maxBytes at a UTF-8 rune boundary.
// Mirrors the helper in the repo so the summary package is self-
// contained and doesn't import sqlite.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut]
}
