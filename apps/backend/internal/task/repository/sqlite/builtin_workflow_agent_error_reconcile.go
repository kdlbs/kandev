// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"encoding/json"
	"fmt"
	"strings"

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
// and only if the same normalized action isn't already present — preserving
// every other declared action and leaving
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

// templateAgentErrorActions returns the distinct queue_run on_agent_error
// actions the step's template declares. The key includes the full normalized
// action config so actions that target different tasks or carry different
// payloads are not collapsed. Empty target and task_id values are normalized
// to the same defaults used by the workflow engine.
func templateAgentErrorActions(step wfmodels.StepDefinition) []wfmodels.GenericAction {
	var actions []wfmodels.GenericAction
	seen := map[string]struct{}{}
	for _, action := range step.Events.OnAgentError {
		if action.Type != wfmodels.GenericActionQueueRun {
			continue
		}
		key := agentErrorActionKey(action)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
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
// equivalent normalized action already exists, and writes back only if the
// row's events column still matches what was read — the
// read value doubles as the optimistic-concurrency guard. applied is true
// when the row already had the action or the write succeeded (both are
// "nothing left to do" outcomes); retry is true only when a concurrent
// writer changed the row between the read and the write.
func (r *Repository) tryHealAgentErrorRow(stepID string, action wfmodels.GenericAction) (applied, retry bool, err error) {
	if r.failAgentErrorReconcileAttempts > 0 {
		r.failAgentErrorReconcileAttempts--
		return false, true, nil
	}

	return r.tryHealWorkflowStepEvents(stepID, "agent-error", func(events *wfmodels.StepEvents) bool {
		if hasAgentErrorEscalation(events.OnAgentError, action) {
			return true
		}
		events.OnAgentError = append(events.OnAgentError, action)
		return false
	})
}

// hasAgentErrorEscalation reports whether actions already declares the same
// normalized queue_run on_agent_error action as wanted.
func hasAgentErrorEscalation(actions []wfmodels.GenericAction, wanted wfmodels.GenericAction) bool {
	wantedKey := agentErrorActionKey(wanted)
	for _, action := range actions {
		if action.Type != wfmodels.GenericActionQueueRun {
			continue
		}
		if agentErrorActionKey(action) == wantedKey {
			return true
		}
	}
	return false
}

const (
	// These defaults must stay aligned with engine.readQueueRunConfig. The
	// reconciler does not import the engine package because the repository
	// layer must not depend on its runtime implementation.
	defaultAgentErrorTarget = "primary"
	defaultAgentErrorTaskID = "this"
)

// agentErrorActionKey returns a stable representation of the queue_run action
// as the engine evaluates it. JSON gives map keys a deterministic order and
// normalizes YAML/JSON numeric values across template materialization.
func agentErrorActionKey(action wfmodels.GenericAction) string {
	normalized := action
	config := make(map[string]interface{}, len(action.Config)+4)
	for key, value := range action.Config {
		config[key] = value
	}

	target, _ := config["target"].(string)
	target = strings.TrimSpace(target)
	if target == "" {
		target = defaultAgentErrorTarget
	}
	config["target"] = target

	taskID, _ := config["task_id"].(string)
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		taskID = defaultAgentErrorTaskID
	}
	config["task_id"] = taskID

	reason, _ := config["reason"].(string)
	config["reason"] = reason

	if payload, ok := config["payload"].(map[string]interface{}); ok {
		config["payload"] = payload
	} else {
		config["payload"] = nil
	}
	normalized.Config = config

	encoded, err := json.Marshal(normalized)
	if err == nil {
		return string(encoded)
	}
	// Template and persisted configs are JSON-compatible. Keep a deterministic
	// fallback for malformed in-memory values so one bad action cannot stop
	// startup reconciliation.
	return fmt.Sprintf("%s:%#v", normalized.Type, normalized.Config)
}
