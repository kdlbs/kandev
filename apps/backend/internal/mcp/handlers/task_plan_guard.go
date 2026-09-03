package handlers

import (
	"fmt"

	"github.com/kandev/kandev/internal/task/dto"
)

// planTruncationWarning renders the agent-facing warning appended to a plan
// write's tool result when the plan service reports TruncationDetected. It
// states plainly that the write replaced the entire document —
// update_task_plan_kandev and create_task_plan_kandev have no partial-update
// mode — and names the prior revision number when it is known.
//
// replacedRunes/newRunes are rune counts (not byte lengths): a script change
// (e.g. an ASCII plan rewritten in CJK) can retain a small fraction of the
// document's characters while retaining most of its bytes, so counting bytes
// would silently defeat this guard on exactly the kind of loss it exists to
// catch. Kandev ships zh-cn/zh-hk/zh-tw/pt-pt locales, so non-ASCII plan
// content is not hypothetical. The plan service computes these counts inside
// its write's critical section, off the content it actually replaced.
//
// priorRevisionNumber of 0 means the prior revision could not be established.
// The revision lookup can fail, or the latest revision can differ from the
// replaced HEAD because of historical divergence. Revision numbering starts
// at 1 (NextTaskPlanRevisionNumber), so 0 is never a real revision. In that
// case the warning does not make an unverified preservation claim.
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
func planTruncationWarning(replacedRunes, newRunes, priorRevisionNumber int) string {
	// dropped is always >= 0: the only caller renders this after the plan
	// service has already confirmed newRunes < replacedRunes.
	dropped := replacedRunes - newRunes
	droppedPct := float64(dropped) / float64(replacedRunes) * 100

	if priorRevisionNumber <= 0 {
		return fmt.Sprintf(
			"WARNING: this write replaced %d chars with %d (dropped %d chars, %.0f%%). "+
				"Plan writes REPLACE THE ENTIRE DOCUMENT — there is no partial update or append "+
				"mode. Kandev could not verify which prior revision contains the pre-write content. "+
				"The MCP plan tools cannot fetch past revisions. If this drop was not intentional, "+
				"stop and inspect the task's revision history in the Kandev UI rather than rewriting "+
				"the plan from memory.",
			replacedRunes, newRunes, dropped, droppedPct,
		)
	}

	return fmt.Sprintf(
		"WARNING: this write replaced %d chars with %d (dropped %d chars, %.0f%%). "+
			"Plan writes REPLACE THE ENTIRE DOCUMENT — there is no partial update or append "+
			"mode. The pre-write content is preserved in %s — recoverable from the Kandev UI, "+
			"but NOT fetchable through the MCP plan tools (get_task_plan_kandev returns the "+
			"current, now-truncated, content, not that revision). If this drop was not "+
			"intentional, stop and surface the loss rather than rewriting the plan from memory.",
		replacedRunes, newRunes, dropped, droppedPct,
		fmt.Sprintf("plan revision %d, in the task's plan revision history", priorRevisionNumber),
	)
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

// planWritePayload wraps plan in a planWriteResponse when warning is
// non-empty, otherwise returns plan unwrapped so an unaffected write's
// response shape is unchanged.
func planWritePayload(plan *dto.TaskPlanDTO, warning string, priorRevision int) interface{} {
	if warning == "" {
		return plan
	}
	return planWriteResponse{
		TaskPlanDTO:         plan,
		PlanWriteWarning:    warning,
		PriorRevisionNumber: priorRevision,
	}
}
