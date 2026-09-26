// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	workflowcfg "github.com/kandev/kandev/config/workflows"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// maxOnCommentReconcileAttempts bounds the read-modify-write retry loop in
// tryHealOnCommentRow. Exhausting it leaves the step unchanged and logs a
// warning rather than propagating an error — see healOnCommentRowWithRetry.
const maxOnCommentReconcileAttempts = 5

// healBuiltinWorkflowStepOnCommentFanOut reconciles workflow_steps.events on
// system-workflow rows whose steps were materialized before their embedded
// template declared an on_comment queue_run_for_each_participant fan-out
// (REQ-OFFICE-GATE-COMMENT-004). It is modelled on
// healBuiltinWorkflowStepOnAgentError: for every embedded template, for every
// step, for every on_comment queue_run_for_each_participant action that
// step's template declares (one per role), it finds every system-owned
// workflow_steps row materialized from that (template, step name) and
// appends the action if and only if OnComment does not already carry a
// queue_run_for_each_participant action for the same role — preserving every
// other declared action and leaving user-created or user-customised
// workflows (is_system = 0) alone.
func (r *Repository) healBuiltinWorkflowStepOnCommentFanOut() error {
	templates, err := workflowcfg.LoadTemplates()
	if err != nil {
		return fmt.Errorf("load embedded templates for on_comment fan-out healing: %w", err)
	}
	for _, tmpl := range templates {
		for _, step := range tmpl.Steps {
			for _, action := range templateOnCommentFanOutActions(step) {
				if err := r.healStepOnCommentFanOutAction(tmpl.ID, step.Name, action); err != nil {
					return fmt.Errorf("heal on_comment fan-out action for template %s step %s: %w", tmpl.ID, step.Name, err)
				}
			}
		}
	}
	return nil
}

// templateOnCommentFanOutActions returns the distinct on_comment
// queue_run_for_each_participant actions the step's template declares, keyed
// by role: a template step is expected to declare at most one such action per
// role, and a duplicate role is collapsed to the first occurrence.
func templateOnCommentFanOutActions(step wfmodels.StepDefinition) []wfmodels.GenericAction {
	var actions []wfmodels.GenericAction
	seen := map[string]struct{}{}
	for _, action := range step.Events.OnComment {
		if action.Type != wfmodels.GenericActionQueueRunForEachParticipant {
			continue
		}
		role := onCommentFanOutRole(action)
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		actions = append(actions, action)
	}
	return actions
}

// onCommentFanOutRole returns the trimmed role an on_comment
// queue_run_for_each_participant action fans out to.
func onCommentFanOutRole(action wfmodels.GenericAction) string {
	role, _ := action.Config["role"].(string)
	return strings.TrimSpace(role)
}

// healStepOnCommentFanOutAction finds every system-owned workflow_steps row
// materialized from (templateID, stepName) across every workspace and
// reconciles each independently. Rows are collected up front so the
// transactional retry loop below never runs with an open read cursor over
// the same table.
func (r *Repository) healStepOnCommentFanOutAction(templateID, stepName string, action wfmodels.GenericAction) error {
	targets, err := r.findSystemOwnedWorkflowSteps(templateID, stepName)
	if err != nil {
		return err
	}

	for _, target := range targets {
		if err := r.healOnCommentRowWithRetry(target.stepID, target.workflowID, templateID, stepName, action); err != nil {
			return err
		}
	}
	return nil
}

// healOnCommentRowWithRetry drives the CAS retry loop for a single
// workflow_steps row: a concurrent writer changing the row between read and
// write is retried a bounded number of times; exhausting the budget leaves
// the step unchanged, logs a warning naming the template, step and role, and
// returns nil so a single un-reconciled step never blocks backend startup.
func (r *Repository) healOnCommentRowWithRetry(stepID, workflowID, templateID, stepName string, action wfmodels.GenericAction) error {
	for attempt := 0; attempt < maxOnCommentReconcileAttempts; attempt++ {
		applied, retry, err := r.tryHealOnCommentRow(stepID, action)
		if err != nil {
			return err
		}
		if applied || !retry {
			return nil
		}
	}
	if r.log != nil {
		r.log.Warn("exhausted retries reconciling on_comment fan-out action; leaving step unchanged",
			zap.String("template_id", templateID),
			zap.String("step_name", stepName),
			zap.String("workflow_id", workflowID),
			zap.String("step_id", stepID),
			zap.String("role", onCommentFanOutRole(action)),
		)
	}
	return nil
}

// tryHealOnCommentRow makes one read-modify-write attempt for stepID. It
// reads the stored events blob, appends the on_comment fan-out action if no
// action for the same role already exists, and writes back only if the row's
// events column still matches what was read. applied is true when the row
// already had the action or the write succeeded (both are "nothing left to
// do" outcomes); retry is true only when a concurrent writer changed the row
// between the read and the write.
func (r *Repository) tryHealOnCommentRow(stepID string, action wfmodels.GenericAction) (applied, retry bool, err error) {
	if r.failOnCommentReconcileAttempts > 0 {
		r.failOnCommentReconcileAttempts--
		return false, true, nil
	}

	return r.tryHealWorkflowStepEvents(stepID, "on-comment-fanout", func(events *wfmodels.StepEvents) bool {
		if hasOnCommentFanOutRole(events.OnComment, onCommentFanOutRole(action)) {
			return true
		}
		events.OnComment = append(events.OnComment, action)
		return false
	})
}

// hasOnCommentFanOutRole reports whether actions already declares an
// on_comment queue_run_for_each_participant action for wantedRole.
func hasOnCommentFanOutRole(actions []wfmodels.GenericAction, wantedRole string) bool {
	for _, action := range actions {
		if action.Type != wfmodels.GenericActionQueueRunForEachParticipant {
			continue
		}
		if onCommentFanOutRole(action) == wantedRole {
			return true
		}
	}
	return false
}
