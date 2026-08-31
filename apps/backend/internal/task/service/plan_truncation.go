package service

import "unicode/utf8"

const (
	// planTruncationMinPriorChars is the floor below which a shrinking write
	// isn't worth flagging. A 300-char stub plan losing half its content is
	// not a review-round-costing event; both real WO-38 incidents were on
	// 40k+ char plans.
	planTruncationMinPriorChars = 2000

	// planTruncationMaxRetainRatio: this workflow's normal write is an
	// append ("appends its own section"), so a plan's content grows
	// monotonically across a build. A write that keeps less than half of the
	// prior document is anomalous by construction, not a normal edit. Both
	// real WO-38 incidents retained 22.8% and 23.9% of the prior plan — this
	// line catches both with wide margin.
	planTruncationMaxRetainRatio = 0.5
)

// planTruncationDetected reports whether newContent looks like an accidental
// destructive truncation of priorContent, rather than a deliberate edit or a
// legitimate (smaller) prune.
//
// Measured in runes, not bytes: len() on a Go string counts UTF-8 bytes, so a
// script change (e.g. an ASCII plan rewritten in CJK) can retain a small
// fraction of the document's characters while retaining most of its bytes —
// silently defeating this guard on exactly the kind of loss it exists to
// catch. Kandev ships zh-cn/zh-hk/zh-tw/pt-pt locales, so non-ASCII plan
// content is not hypothetical.
//
// This detector runs inside PlanService's per-task write lock (see
// docs/specs/tasks/system-design/plan-write-consistency.md), evaluated
// against the content the write's own read observed, so a concurrent writer
// cannot change what "prior" means out from under this decision.
func planTruncationDetected(priorContent, newContent string) bool {
	priorLen := utf8.RuneCountInString(priorContent)
	if priorLen < planTruncationMinPriorChars {
		return false
	}
	return float64(utf8.RuneCountInString(newContent)) < float64(priorLen)*planTruncationMaxRetainRatio
}
