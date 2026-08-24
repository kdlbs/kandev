package handlers

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/task/dto"
)

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
func planTruncationDetected(priorContent, newContent string) bool {
	priorLen := utf8.RuneCountInString(priorContent)
	if priorLen < planTruncationMinPriorChars {
		return false
	}
	return float64(utf8.RuneCountInString(newContent)) < float64(priorLen)*planTruncationMaxRetainRatio
}

// planTruncationWarning renders the agent-facing warning appended to a plan
// write's tool result when planTruncationDetected reports true. It states
// plainly that the write replaced the entire document — update_task_plan_kandev
// and create_task_plan_kandev have no partial-update mode — and names the
// prior revision number when it is known.
//
// priorRevisionNumber of 0 means the prior revision could not be established
// (the revision-history lookup itself failed after truncation was already
// detected off the plan content). Revision numbering starts at 1
// (NextTaskPlanRevisionNumber), so 0 is never a real revision — in that case
// the "in plan revision N" clause is omitted rather than naming a revision
// that cannot exist.
//
// It deliberately does NOT tell the caller to "recover" the content by
// calling an MCP tool: none of the four registered plan tools can read a
// past revision (get_task_plan_kandev returns the current, now-truncated,
// HEAD). Telling an agent to "recover it" here would send it to the only
// tool it has, get back the truncated document, and fall back to
// reconstructing the plan from memory — the exact WO-38 failure this guard
// exists to stop. Instead it says where the content lives (revision
// history, Kandev UI only) and that the caller cannot fetch it itself, so
// the caller stops and surfaces the loss instead of guessing.
func planTruncationWarning(priorContent, newContent string, priorRevisionNumber int) string {
	priorLen := utf8.RuneCountInString(priorContent)
	newLen := utf8.RuneCountInString(newContent)
	// dropped is always >= 0: the only caller, evaluatePlanWriteGuard, invokes this
	// after planTruncationDetected has already confirmed newLen < priorLen.
	dropped := priorLen - newLen
	droppedPct := float64(dropped) / float64(priorLen) * 100

	preservedIn := "the task's plan revision history"
	if priorRevisionNumber > 0 {
		preservedIn = fmt.Sprintf("plan revision %d, in the task's plan revision history", priorRevisionNumber)
	}

	return fmt.Sprintf(
		"WARNING: this write replaced %d chars with %d (dropped %d chars, %.0f%%). "+
			"Plan writes REPLACE THE ENTIRE DOCUMENT — there is no partial update or append "+
			"mode. The pre-write content is preserved in %s — recoverable from the Kandev UI, "+
			"but NOT fetchable through the MCP plan tools (get_task_plan_kandev returns the "+
			"current, now-truncated, content, not that revision). If this drop was not "+
			"intentional, stop and surface the loss rather than rewriting the plan from memory.",
		priorLen, newLen, dropped, droppedPct, preservedIn,
	)
}

// planWriteGuardResult carries the truncation-guard outcome for a plan
// create/update: whether the underlying revision write must be forced to
// append rather than coalesce, and the warning text (if any) to surface in
// the tool response.
type planWriteGuardResult struct {
	forceNewRevision bool
	warning          string
	priorRevision    int
}

// evaluatePlanWriteGuard compares a task's current plan content against an
// incoming write and decides whether it looks like an accidental
// whole-document truncation. It covers both create_task_plan_kandev (which
// upserts, so a create over an existing plan is the same destructive write
// through a different door) and update_task_plan_kandev.
//
// A failure to fetch the current plan is non-fatal: the write proceeds
// without a truncation warning, but it forces a new revision so a later
// successful write cannot coalesce into an unknown prior revision. A failure
// to list the revision history is handled differently:
// truncation has already been detected from the plan content at that point,
// so the guard still forces a new revision and still warns — it renders the
// warning without a specific revision number (see planTruncationWarning)
// instead of silently dropping the warning, because clearing
// forceNewRevision here would let this destructive write coalesce into, and
// overwrite, the only surviving copy of the pre-truncation content.
func (h *Handlers) evaluatePlanWriteGuard(ctx context.Context, taskID, newContent string) planWriteGuardResult {
	existing, err := h.planService.GetPlan(ctx, taskID)
	if err != nil {
		return planWriteGuardResult{forceNewRevision: true}
	}
	if existing == nil {
		return planWriteGuardResult{}
	}
	if !planTruncationDetected(existing.Content, newContent) {
		return planWriteGuardResult{}
	}

	priorRevisionNumber := 0
	if revisions, revErr := h.planService.ListRevisions(ctx, taskID); revErr == nil && len(revisions) > 0 {
		priorRevisionNumber = revisions[0].RevisionNumber
	}

	return planWriteGuardResult{
		forceNewRevision: true,
		warning:          planTruncationWarning(existing.Content, newContent, priorRevisionNumber),
		priorRevision:    priorRevisionNumber,
	}
}

// planWriteResponse extends the standard plan DTO with a truncation warning
// for the MCP write actions only. It deliberately does not touch
// dto.TaskPlanDTO itself — the browser plan editor (which has a visible diff
// and revision history, and uses TaskPlanDTO as-is) is unaffected.
// json.Marshal promotes an embedded pointer struct's exported fields to the
// top level, so a non-truncating write still marshals to the identical shape
// callers see today.
type planWriteResponse struct {
	*dto.TaskPlanDTO
	PlanWriteWarning    string `json:"plan_write_warning,omitempty"`
	PriorRevisionNumber int    `json:"prior_revision_number,omitempty"`
}

// planWritePayload wraps plan in a planWriteResponse when guard carries a
// warning, otherwise returns plan unwrapped so an unaffected write's
// response shape is unchanged.
func planWritePayload(plan *dto.TaskPlanDTO, guard planWriteGuardResult) interface{} {
	if guard.warning == "" {
		return plan
	}
	return planWriteResponse{
		TaskPlanDTO:         plan,
		PlanWriteWarning:    guard.warning,
		PriorRevisionNumber: guard.priorRevision,
	}
}
