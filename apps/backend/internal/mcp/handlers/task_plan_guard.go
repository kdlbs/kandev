package handlers

import (
	"context"
	"fmt"

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
func planTruncationDetected(priorContent, newContent string) bool {
	priorLen := len(priorContent)
	if priorLen < planTruncationMinPriorChars {
		return false
	}
	return float64(len(newContent)) < float64(priorLen)*planTruncationMaxRetainRatio
}

// planTruncationWarning renders the agent-facing warning appended to a plan
// write's tool result when planTruncationDetected reports true. It states
// plainly that the write replaced the entire document — update_task_plan_kandev
// and create_task_plan_kandev have no partial-update mode — and names the
// prior revision number so the caller (or a human) can recover the dropped
// content from plan history instead of silently losing it.
func planTruncationWarning(priorContent, newContent string, priorRevisionNumber int) string {
	priorLen := len(priorContent)
	newLen := len(newContent)
	dropped := priorLen - newLen
	droppedPct := float64(dropped) / float64(priorLen) * 100
	return fmt.Sprintf(
		"WARNING: this write replaced %d chars with %d (dropped %d chars, %.0f%%). "+
			"Plan writes REPLACE THE ENTIRE DOCUMENT — there is no partial update or append "+
			"mode. If this drop was not intentional, the pre-write content is preserved in "+
			"plan revision %d; recover it before writing again.",
		priorLen, newLen, dropped, droppedPct, priorRevisionNumber,
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
// Lookup failures are non-fatal: if the current plan or its revision history
// can't be fetched, the write still proceeds without a truncation warning
// rather than blocking on a guard that itself failed.
func (h *Handlers) evaluatePlanWriteGuard(ctx context.Context, taskID, newContent string) planWriteGuardResult {
	existing, err := h.planService.GetPlan(ctx, taskID)
	if err != nil || existing == nil {
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
