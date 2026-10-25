package handlers

import (
	"fmt"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
)

// planTruncationWarning renders the agent-facing warning appended to a plan
// write's tool result when the plan service reports TruncationDetected. It
// states plainly that the write replaced the entire document, names
// update_task_plan_kandev's mode="append" as the way to add a section
// without resubmitting the whole document, and names the prior revision
// number when it is known. This detector never runs on an append write
// (see plan_service.go), so every write it can flag was necessarily a
// replace.
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
// This helper remains for compatibility with older non-guarded callers. The
// current agent handlers reject a suspicious reduction before this warning
// can be produced and expose task-scoped revision recovery tools. If a legacy
// caller does receive the warning, it points to those tools and never asks
// the agent to reconstruct content from memory.
func planTruncationWarning(replacedRunes, newRunes, priorRevisionNumber int) string {
	// dropped is always >= 0: the only caller renders this after the plan
	// service has already confirmed newRunes < replacedRunes.
	dropped := replacedRunes - newRunes
	droppedPct := float64(dropped) / float64(replacedRunes) * 100

	if priorRevisionNumber <= 0 {
		return fmt.Sprintf(
			"WARNING: this write replaced %d chars with %d (dropped %d chars, %.0f%%). "+
				"This replace-mode write overwrote the entire document; use "+
				"update_task_plan_kandev with mode=\"append\" to add a section without "+
				"resubmitting the whole document next time. Kandev could not verify which "+
				"prior revision contains the pre-write content. Read the current plan and "+
				"list_task_plan_revisions_kandev before retrying. If this drop was not "+
				"intentional, fetch the identified revision when available rather than "+
				"rewriting the plan from memory.",
			replacedRunes, newRunes, dropped, droppedPct,
		)
	}

	return fmt.Sprintf(
		"WARNING: this write replaced %d chars with %d (dropped %d chars, %.0f%%). "+
			"This replace-mode write overwrote the entire document; use "+
			"update_task_plan_kandev with mode=\"append\" to add a section without "+
			"resubmitting the whole document next time. The pre-write content is "+
			"preserved in %s. Use list_task_plan_revisions_kandev and "+
			"get_task_plan_revision_kandev to inspect that snapshot. If this drop was "+
			"not intentional, restore it only after checking the current plan and both "+
			"snapshot versions; do not rewrite the plan from memory.",
		replacedRunes, newRunes, dropped, droppedPct,
		fmt.Sprintf("plan revision %d, in the task's plan revision history", priorRevisionNumber),
	)
}

// planReadResponse extends the standard plan DTO with the agent-only opaque
// write version. It deliberately does not touch
// dto.TaskPlanDTO itself — the browser plan editor (which has a visible diff
// and revision history, and uses TaskPlanDTO as-is) is unaffected.
type planReadResponse struct {
	*dto.TaskPlanDTO
	Version string `json:"version,omitempty"`
}

// planWriteResponse extends the standard plan DTO with the committed version
// and, for compatibility with older callers, the existing truncation fields.
type planWriteResponse struct {
	*dto.TaskPlanDTO
	Version             string `json:"version,omitempty"`
	PlanWriteWarning    string `json:"plan_write_warning,omitempty"`
	PriorRevisionNumber int    `json:"prior_revision_number,omitempty"`
}

func planReadPayload(plan *models.TaskPlan) interface{} {
	return planReadResponse{TaskPlanDTO: dto.TaskPlanFromModel(plan), Version: plan.WriteVersion}
}

// planWritePayload always includes the committed version for agent writes.
func planWritePayload(plan *dto.TaskPlanDTO, version, warning string, priorRevision int) interface{} {
	if version == "" && warning == "" {
		return plan
	}
	return planWriteResponse{
		TaskPlanDTO:         plan,
		Version:             version,
		PlanWriteWarning:    warning,
		PriorRevisionNumber: priorRevision,
	}
}
