// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	workflowcfg "github.com/kandev/kandev/config/workflows"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// maxParticipantSeatReconcileAttempts bounds the read-modify-write retry loop
// in tryHealParticipantSeatRow. Exhausting it leaves the step unchanged and
// logs a warning rather than propagating an error — see
// healParticipantSeatRowWithRetry.
const maxParticipantSeatReconcileAttempts = 5

// healBuiltinWorkflowStepParticipantSeats reconciles workflow_steps.events on
// system-workflow rows whose steps were materialized before their embedded
// template declared an ensure_participant_seat action for a role
// (AC-OFFICE-REVIEW-SEATS-005.6). It is modelled on
// healBuiltinWorkflowStepFlags: for every embedded template, for every step,
// for every participant role that step's template declares a seat-ensuring
// action for, it finds every system-owned workflow_steps row materialized
// from that (template, step name) and inserts the action if and only if that
// role isn't already present — preserving every other declared action and
// leaving user-created or user-customised workflows (is_system = 0) alone
// (AC-005.7).
func (r *Repository) healBuiltinWorkflowStepParticipantSeats() error {
	templates, err := workflowcfg.LoadTemplates()
	if err != nil {
		return fmt.Errorf("load embedded templates for participant seat healing: %w", err)
	}
	for _, tmpl := range templates {
		for _, step := range tmpl.Steps {
			for _, role := range templateSeatRoles(step) {
				if err := r.healStepParticipantSeatRole(tmpl.ID, step.Name, role); err != nil {
					return fmt.Errorf("heal participant seat action for template %s step %s role %s: %w", tmpl.ID, step.Name, role, err)
				}
			}
		}
	}
	return nil
}

// templateSeatRoles returns the distinct, non-empty participant roles step's
// template declares an ensure_participant_seat on_enter action for, in
// declaration order. A role that is empty or missing is a malformed template
// declaration, not this reconciler's concern (the engine's own runtime
// tolerance in EnsureParticipantSeatCallback handles it), so it is skipped.
func templateSeatRoles(step wfmodels.StepDefinition) []string {
	var roles []string
	seen := map[string]bool{}
	for _, action := range step.Events.OnEnter {
		if action.Type != wfmodels.OnEnterEnsureParticipantSeat {
			continue
		}
		role, _ := action.Config["role"].(string)
		role = strings.TrimSpace(role)
		if role == "" || seen[role] {
			continue
		}
		seen[role] = true
		roles = append(roles, role)
	}
	return roles
}

// healStepParticipantSeatRole finds every system-owned workflow_steps row
// materialized from (templateID, stepName) across every workspace and
// reconciles each independently. Rows are collected up front so the
// transactional retry loop below never runs with an open read cursor over
// the same table.
func (r *Repository) healStepParticipantSeatRole(templateID, stepName, role string) error {
	targets, err := r.findSystemOwnedWorkflowSteps(templateID, stepName)
	if err != nil {
		return err
	}

	for _, target := range targets {
		if err := r.healParticipantSeatRowWithRetry(target.stepID, target.workflowID, templateID, stepName, role); err != nil {
			return err
		}
	}
	return nil
}

// healParticipantSeatRowWithRetry drives the CAS retry loop for a single
// workflow_steps row (AC-OFFICE-REVIEW-SEATS-005.9): a concurrent writer
// changing the row between read and write is retried a bounded number of
// times; exhausting the budget leaves the step unchanged, logs a warning
// naming the workflow and step, and returns nil so a single un-reconciled
// step never blocks backend startup.
func (r *Repository) healParticipantSeatRowWithRetry(stepID, workflowID, templateID, stepName, role string) error {
	for attempt := 0; attempt < maxParticipantSeatReconcileAttempts; attempt++ {
		applied, retry, err := r.tryHealParticipantSeatRow(stepID, role)
		if err != nil {
			return err
		}
		if applied || !retry {
			return nil
		}
	}
	if r.log != nil {
		r.log.Warn("exhausted retries reconciling participant seat action; leaving step unchanged",
			zap.String("template_id", templateID),
			zap.String("step_name", stepName),
			zap.String("workflow_id", workflowID),
			zap.String("step_id", stepID),
			zap.String("role", role),
		)
	}
	return nil
}

// tryHealParticipantSeatRow makes one read-modify-write attempt for stepID.
// It reads the stored events blob, inserts the seat-ensuring action for role
// if absent, and writes back only if the row's events column still matches
// what was read — the read value doubles as the optimistic-concurrency
// guard. applied is true when the row already had the action or the write
// succeeded (both are "nothing left to do" outcomes); retry is true only
// when a concurrent writer changed the row between the read and the write.
func (r *Repository) tryHealParticipantSeatRow(stepID, role string) (applied, retry bool, err error) {
	if r.failParticipantSeatReconcileAttempts > 0 {
		r.failParticipantSeatReconcileAttempts--
		return false, true, nil
	}

	return r.tryHealWorkflowStepEvents(stepID, "participant seat", func(events *wfmodels.StepEvents) bool {
		if hasSeatRole(events.OnEnter, role) {
			return true
		}
		events.OnEnter = insertSeatAction(events.OnEnter, role)
		return false
	})
}

// hasSeatRole reports whether actions already declares an
// ensure_participant_seat action for role.
func hasSeatRole(actions []wfmodels.OnEnterAction, role string) bool {
	for _, action := range actions {
		if action.Type != wfmodels.OnEnterEnsureParticipantSeat {
			continue
		}
		existing, _ := action.Config["role"].(string)
		if existing == role {
			return true
		}
	}
	return false
}

// insertSeatAction returns a copy of actions with an ensure_participant_seat
// action for role inserted at the position AC-OFFICE-REVIEW-SEATS-005.8
// mandates: immediately before the first queue_run_for_each_participant
// action declaring the same role, or at the head of the sequence when no
// such fan-out action is present. This mirrors office-default.yml's own
// hand-authored ordering, so a reconciled step and a freshly-materialized
// one are identical in content and order.
func insertSeatAction(actions []wfmodels.OnEnterAction, role string) []wfmodels.OnEnterAction {
	seat := wfmodels.OnEnterAction{
		Type:   wfmodels.OnEnterEnsureParticipantSeat,
		Config: map[string]interface{}{"role": role},
	}
	insertAt := 0
	for i, action := range actions {
		if action.Type != wfmodels.OnEnterQueueRunForEachParticipant {
			continue
		}
		fanoutRole, _ := action.Config["role"].(string)
		if fanoutRole == role {
			insertAt = i
			break
		}
	}
	result := make([]wfmodels.OnEnterAction, 0, len(actions)+1)
	result = append(result, actions[:insertAt]...)
	result = append(result, seat)
	result = append(result, actions[insertAt:]...)
	return result
}
