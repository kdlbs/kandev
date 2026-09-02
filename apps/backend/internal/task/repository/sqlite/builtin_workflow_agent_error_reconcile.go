// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	workflowcfg "github.com/kandev/kandev/config/workflows"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// maxAgentErrorReconcileAttempts bounds the read-modify-write retry loop in
// tryHealAgentErrorRow. Exhausting it leaves the step unchanged and logs a
// warning rather than propagating an error — see
// healAgentErrorRowWithRetry.
const maxAgentErrorReconcileAttempts = 5

// healBuiltinWorkflowStepOnAgentError reconciles workflow_steps.events on
// system-workflow rows whose steps were materialized before their embedded
// template declared an on_agent_error escalation action (WO-05-2). It is
// modelled on healBuiltinWorkflowStepParticipantSeats: for every embedded
// template, for every step, for every on_agent_error action that step's
// template declares, it finds every system-owned workflow_steps row
// materialized from that (template, step name) and appends the action if
// and only if an action targeting the same (target, reason) isn't already
// present — preserving every other declared action and leaving
// user-created or user-customised workflows (is_system = 0) alone.
func (r *Repository) healBuiltinWorkflowStepOnAgentError() error {
	templates, err := workflowcfg.LoadTemplates()
	if err != nil {
		return fmt.Errorf("load embedded templates for on_agent_error healing: %w", err)
	}
	for _, tmpl := range templates {
		for _, step := range tmpl.Steps {
			r.warnNonQueueRunAgentErrorActions(tmpl.ID, step)
			for _, action := range templateAgentErrorActions(step) {
				if err := r.healStepOnAgentErrorAction(tmpl.ID, step.Name, action); err != nil {
					return fmt.Errorf("heal on_agent_error action for template %s step %s: %w", tmpl.ID, step.Name, err)
				}
			}
		}
	}
	return nil
}

// warnNonQueueRunAgentErrorActions logs any on_agent_error action this
// healer does not reconcile (everything but queue_run, per
// templateAgentErrorActions) so a future template adding a different action
// type doesn't leave materialized rows silently un-reconciled.
func (r *Repository) warnNonQueueRunAgentErrorActions(templateID string, step wfmodels.StepDefinition) {
	if r.log == nil {
		return
	}
	for _, action := range step.Events.OnAgentError {
		if action.Type == wfmodels.GenericActionQueueRun {
			continue
		}
		r.log.Warn("on_agent_error healer only reconciles queue_run actions; skipping",
			zap.String("template_id", templateID),
			zap.String("step_name", step.Name),
			zap.String("action_type", string(action.Type)),
		)
	}
}

// templateAgentErrorActions returns the distinct on_agent_error actions
// step's template declares, keyed by (target, reason) so a duplicate
// declaration in the template only reconciles once. A queue_run action with
// an empty target is a malformed template declaration, not this
// reconciler's concern, so it is skipped.
func templateAgentErrorActions(step wfmodels.StepDefinition) []wfmodels.GenericAction {
	var actions []wfmodels.GenericAction
	seen := map[string]bool{}
	for _, action := range step.Events.OnAgentError {
		if action.Type != wfmodels.GenericActionQueueRun {
			continue
		}
		target, _ := action.Config["target"].(string)
		if target == "" {
			continue
		}
		reason, _ := action.Config["reason"].(string)
		key := target + "\x00" + reason
		if seen[key] {
			continue
		}
		seen[key] = true
		actions = append(actions, action)
	}
	return actions
}

// healStepOnAgentErrorAction finds every system-owned workflow_steps row
// materialized from (templateID, stepName) across every workspace and
// reconciles each independently. Rows are collected up front so the
// transactional retry loop below never runs with an open read cursor over
// the same table.
func (r *Repository) healStepOnAgentErrorAction(templateID, stepName string, action wfmodels.GenericAction) error {
	targets, err := r.findSystemOwnedWorkflowSteps(templateID, stepName)
	if err != nil {
		return err
	}

	for _, target := range targets {
		if err := r.healAgentErrorRowWithRetry(target.stepID, target.workflowID, templateID, stepName, action); err != nil {
			return err
		}
	}
	return nil
}

// healAgentErrorRowWithRetry drives the CAS retry loop for a single
// workflow_steps row: a concurrent writer changing the row between read and
// write is retried a bounded number of times; exhausting the budget leaves
// the step unchanged, logs a warning naming the workflow and step, and
// returns nil so a single un-reconciled step never blocks backend startup.
func (r *Repository) healAgentErrorRowWithRetry(stepID, workflowID, templateID, stepName string, action wfmodels.GenericAction) error {
	target, _ := action.Config["target"].(string)
	reason, _ := action.Config["reason"].(string)
	for attempt := 0; attempt < maxAgentErrorReconcileAttempts; attempt++ {
		applied, retry, err := r.tryHealAgentErrorRow(stepID, action)
		if err != nil {
			return err
		}
		if applied || !retry {
			return nil
		}
	}
	if r.log != nil {
		r.log.Warn("exhausted retries reconciling on_agent_error action; leaving step unchanged",
			zap.String("template_id", templateID),
			zap.String("step_name", stepName),
			zap.String("workflow_id", workflowID),
			zap.String("step_id", stepID),
			zap.String("target", target),
			zap.String("reason", reason),
		)
	}
	return nil
}

// tryHealAgentErrorRow makes one read-modify-write attempt for stepID. It
// reads the stored events blob, appends the on_agent_error action if no
// existing action already targets the same (target, reason), and writes
// back only if the row's events column still matches what was read — the
// read value doubles as the optimistic-concurrency guard. applied is true
// when the row already had the action or the write succeeded (both are
// "nothing left to do" outcomes); retry is true only when a concurrent
// writer changed the row between the read and the write.
func (r *Repository) tryHealAgentErrorRow(stepID string, action wfmodels.GenericAction) (applied, retry bool, err error) {
	if r.failAgentErrorReconcileAttempts > 0 {
		r.failAgentErrorReconcileAttempts--
		return false, true, nil
	}

	tx, err := r.db.Beginx()
	if err != nil {
		return false, false, fmt.Errorf("begin agent-error reconciliation tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var rawEvents sql.NullString
	if err := tx.QueryRow(tx.Rebind(`SELECT events FROM workflow_steps WHERE id = ?`), stepID).Scan(&rawEvents); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, false, nil
		}
		return false, false, fmt.Errorf("read step events: %w", err)
	}

	var events wfmodels.StepEvents
	if rawEvents.String != "" {
		if err := json.Unmarshal([]byte(rawEvents.String), &events); err != nil {
			return false, false, fmt.Errorf("parse step events: %w", err)
		}
	}

	target, _ := action.Config["target"].(string)
	reason, _ := action.Config["reason"].(string)
	if hasAgentErrorEscalation(events.OnAgentError, target, reason) {
		return true, false, nil
	}
	events.OnAgentError = append(events.OnAgentError, action)

	updated, err := json.Marshal(events)
	if err != nil {
		return false, false, fmt.Errorf("marshal step events: %w", err)
	}

	// events is TEXT and nullable; NULL requires "IS NULL" on both dialects
	// since neither treats a bound parameter after IS/= as NULL-matching.
	guardClause := "events = ?"
	guardArg := any(rawEvents.String)
	if !rawEvents.Valid {
		guardClause = "events IS NULL"
		guardArg = nil
	}
	args := []any{string(updated), time.Now().UTC(), stepID}
	if guardArg != nil {
		args = append(args, guardArg)
	}
	res, err := tx.Exec(tx.Rebind(`
		UPDATE workflow_steps SET events = ?, updated_at = ?
		WHERE id = ? AND `+guardClause), args...)
	if err != nil {
		return false, false, fmt.Errorf("write step events: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, false, fmt.Errorf("check agent-error reconciliation write: %w", err)
	}
	if affected == 0 {
		return false, true, nil
	}
	if err := tx.Commit(); err != nil {
		return false, false, fmt.Errorf("commit agent-error reconciliation: %w", err)
	}
	return true, false, nil
}

// hasAgentErrorEscalation reports whether actions already declares a
// queue_run on_agent_error action targeting (target, reason).
func hasAgentErrorEscalation(actions []wfmodels.GenericAction, target, reason string) bool {
	for _, action := range actions {
		if action.Type != wfmodels.GenericActionQueueRun {
			continue
		}
		existingTarget, _ := action.Config["target"].(string)
		existingReason, _ := action.Config["reason"].(string)
		if existingTarget == target && existingReason == reason {
			return true
		}
	}
	return false
}
