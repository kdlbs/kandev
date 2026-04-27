package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// Concurrency policy constants.
const (
	ConcurrencySkipIfActive   = "skip_if_active"
	ConcurrencyCoalesceActive = "coalesce_if_active"
	ConcurrencyAlwaysCreate   = "always_create"
)

// Run status constants.
const (
	RunStatusReceived    = "received"
	RunStatusTaskCreated = "task_created"
	RunStatusSkipped     = "skipped"
	RunStatusCoalesced   = "coalesced"
	RunStatusFailed      = "failed"
	RunStatusDone        = "done"
	RunStatusCancelled   = "cancelled"
)

// TickScheduledTriggers queries due cron triggers, claims each, and dispatches.
func (s *Service) TickScheduledTriggers(ctx context.Context, now time.Time) error {
	triggers, err := s.repo.GetDueTriggers(ctx, now)
	if err != nil {
		return fmt.Errorf("get due triggers: %w", err)
	}
	for _, trigger := range triggers {
		if err := s.processCronTrigger(ctx, trigger, now); err != nil {
			s.logger.Error("process cron trigger",
				zap.String("trigger_id", trigger.ID), zap.Error(err))
		}
	}
	return nil
}

func (s *Service) processCronTrigger(ctx context.Context, trigger *models.RoutineTrigger, now time.Time) error {
	if trigger.NextRunAt == nil {
		return nil
	}
	claimed, err := s.repo.ClaimTrigger(ctx, trigger.ID, *trigger.NextRunAt)
	if err != nil || !claimed {
		return err
	}
	next, err := nextCronTick(trigger.CronExpression, trigger.Timezone, now)
	if err != nil {
		s.logger.Error("compute next cron tick", zap.Error(err))
	} else {
		_ = s.repo.UpdateTriggerNextRun(ctx, trigger.ID, &next)
	}
	routine, err := s.GetRoutineFromConfig(ctx, trigger.RoutineID)
	if err != nil {
		return fmt.Errorf("get routine: %w", err)
	}
	_, err = s.DispatchRoutineRun(ctx, routine, trigger, "cron", nil)
	return err
}

// DispatchRoutineRun resolves variables, applies concurrency policy, creates run.
func (s *Service) DispatchRoutineRun(
	ctx context.Context,
	routine *models.Routine,
	trigger *models.RoutineTrigger,
	source string,
	provided map[string]string,
) (*models.RoutineRun, error) {
	now := time.Now().UTC()
	defaults := parseDeclaredDefaults(routine.Variables)
	vars := resolveVariables(now, defaults, provided)

	tmpl := parseTaskTemplate(routine.TaskTemplate)
	title := interpolateTemplate(tmpl.Title, vars)
	description := interpolateTemplate(tmpl.Description, vars)
	fingerprint := computeFingerprint(title, description, routine.AssigneeAgentInstanceID)

	triggerID := ""
	if trigger != nil {
		triggerID = trigger.ID
	}
	payloadJSON, _ := json.Marshal(vars)

	run := &models.RoutineRun{
		RoutineID:           routine.ID,
		TriggerID:           triggerID,
		Source:              source,
		Status:              RunStatusReceived,
		TriggerPayload:      string(payloadJSON),
		DispatchFingerprint: fingerprint,
		StartedAt:           &now,
	}
	if err := s.repo.CreateRoutineRun(ctx, run); err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}

	status, err := s.applyConcurrencyPolicy(ctx, routine, run, fingerprint)
	if err != nil {
		return run, err
	}
	if status != "" {
		return run, nil
	}

	run.Status = RunStatusTaskCreated
	run.LinkedTaskID = fmt.Sprintf("task-%s", run.ID[:8])
	if err := s.repo.UpdateRunStatus(ctx, run.ID, run.Status, run.LinkedTaskID); err != nil {
		return run, fmt.Errorf("update run status: %w", err)
	}

	routine.LastRunAt = &now

	s.logger.Info("routine run dispatched",
		zap.String("routine", routine.Name),
		zap.String("run_id", run.ID),
		zap.String("title", title))
	return run, nil
}

func (s *Service) applyConcurrencyPolicy(
	ctx context.Context,
	routine *models.Routine,
	run *models.RoutineRun,
	fingerprint string,
) (string, error) {
	if routine.ConcurrencyPolicy == ConcurrencyAlwaysCreate {
		return "", nil
	}
	active, err := s.repo.GetActiveRunForFingerprint(ctx, routine.ID, fingerprint)
	if err != nil {
		return "", fmt.Errorf("check active run: %w", err)
	}
	if active == nil {
		return "", nil
	}
	switch routine.ConcurrencyPolicy {
	case ConcurrencySkipIfActive:
		_ = s.repo.UpdateRunStatus(ctx, run.ID, RunStatusSkipped, "")
		run.Status = RunStatusSkipped
		return RunStatusSkipped, nil
	case ConcurrencyCoalesceActive:
		_ = s.repo.UpdateRunCoalesced(ctx, run.ID, active.ID)
		run.Status = RunStatusCoalesced
		run.CoalescedIntoRunID = active.ID
		return RunStatusCoalesced, nil
	}
	return "", nil
}

// FireManual dispatches a routine run from a manual trigger.
func (s *Service) FireManual(
	ctx context.Context, routineID string, variableValues map[string]string,
) (*models.RoutineRun, error) {
	routine, err := s.GetRoutineFromConfig(ctx, routineID)
	if err != nil {
		return nil, fmt.Errorf("get routine: %w", err)
	}
	return s.DispatchRoutineRun(ctx, routine, nil, "manual", variableValues)
}

// SyncRunStatus updates a linked run when its task reaches a terminal state.
func (s *Service) SyncRunStatus(ctx context.Context, taskID, terminalStatus string) error {
	// This is called when a task completes; for now we log it.
	s.logger.Info("sync run status",
		zap.String("task_id", taskID),
		zap.String("status", terminalStatus))
	return nil
}

// GetTriggerByPublicID returns a trigger by its public ID (for webhook lookup).
func (s *Service) GetTriggerByPublicID(ctx context.Context, publicID string) (*models.RoutineTrigger, error) {
	return s.repo.GetTriggerByPublicID(ctx, publicID)
}

// -- Trigger management for service layer --

// CreateRoutineTrigger creates a trigger and computes next_run_at for cron.
func (s *Service) CreateRoutineTrigger(ctx context.Context, t *models.RoutineTrigger) error {
	if t.Kind == "cron" && t.CronExpression != "" {
		next, err := nextCronTick(t.CronExpression, t.Timezone, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
		t.NextRunAt = &next
	}
	return s.repo.CreateRoutineTrigger(ctx, t)
}

// ListRoutineTriggers returns triggers for a routine.
func (s *Service) ListRoutineTriggers(ctx context.Context, routineID string) ([]*models.RoutineTrigger, error) {
	return s.repo.ListTriggersByRoutineID(ctx, routineID)
}

// DeleteRoutineTrigger deletes a trigger.
func (s *Service) DeleteRoutineTrigger(ctx context.Context, id string) error {
	return s.repo.DeleteRoutineTrigger(ctx, id)
}

// ListRoutineRuns returns paginated runs for a routine.
func (s *Service) ListRoutineRuns(ctx context.Context, routineID string, limit, offset int) ([]*models.RoutineRun, error) {
	return s.repo.ListRuns(ctx, routineID, limit, offset)
}

// ListAllRoutineRuns returns recent runs across all routines in a workspace.
func (s *Service) ListAllRoutineRuns(ctx context.Context, wsID string, limit int) ([]*models.RoutineRun, error) {
	return s.repo.ListAllRuns(ctx, wsID, limit)
}

// -- helpers --

type taskTemplate struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func parseTaskTemplate(raw string) taskTemplate {
	var tmpl taskTemplate
	_ = json.Unmarshal([]byte(raw), &tmpl)
	return tmpl
}

func parseDeclaredDefaults(variablesJSON string) map[string]string {
	defaults := make(map[string]string)
	var vars map[string]struct {
		Default string `json:"default"`
	}
	if err := json.Unmarshal([]byte(variablesJSON), &vars); err != nil {
		return defaults
	}
	for k, v := range vars {
		if v.Default != "" {
			defaults[k] = v.Default
		}
	}
	return defaults
}

func computeFingerprint(title, description, assignee string) string {
	h := sha256.Sum256([]byte(title + "|" + description + "|" + assignee))
	return fmt.Sprintf("%x", h[:16])
}
