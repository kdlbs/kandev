package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// ErrTaskNotFound is the sentinel that run-cleanup paths check to distinguish
// "the task is already gone — fine, drop the run row anyway" from a real
// upstream failure. TaskDeleter implementations should wrap this when the
// task domain reports a missing row, so this package can recognize the case
// via errors.Is without importing the task repository package (see
// backendapp's taskDeleterAdapter for the production wiring).
var ErrTaskNotFound = errors.New("automation: task not found for cleanup")

// TaskDeleter deletes a task and cleans up its resources.
// Satisfied by *taskservice.Service; injected to avoid a cyclic import.
// Implementations should return errors wrapping ErrTaskNotFound when the
// task is already gone.
type TaskDeleter interface {
	DeleteTask(ctx context.Context, id string) error
}

// Service coordinates automation operations.
type Service struct {
	store       *Store
	eventBus    bus.EventBus
	logger      *logger.Logger
	taskDeleter TaskDeleter // optional; nil-safe

	// runLocks serializes run creation (RecordRun, the concurrency-cap skip
	// insert) against DeleteAllRuns per automation ID. Without this, a run
	// created between DeleteAllRuns' task-id snapshot and its final row
	// purge would have its row deleted without its task ever reaching the
	// TaskDeleter — orphaning the task. Entries are never removed: growth is
	// bounded by the number of distinct automation IDs (~160 B per entry).
	runLocks sync.Map // automationID (string) -> *sync.Mutex
}

// NewService creates a new automation service.
func NewService(store *Store, eventBus bus.EventBus, log *logger.Logger) *Service {
	return &Service{
		store:    store,
		eventBus: eventBus,
		logger:   log,
	}
}

// Store returns the underlying store (for scheduler/poller access).
func (s *Service) Store() *Store {
	return s.store
}

// SetTaskDeleter wires the task deletion handler for run cleanup.
// Optional: when nil, run deletion skips task teardown.
func (s *Service) SetTaskDeleter(d TaskDeleter) {
	s.taskDeleter = d
}

// --- Automation CRUD ---

// CreateAutomation creates an automation with its initial triggers.
func (s *Service) CreateAutomation(ctx context.Context, req *CreateAutomationRequest) (*Automation, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	maxRuns := req.MaxConcurrentRuns
	if maxRuns <= 0 {
		maxRuns = 1
	}

	mode := req.ExecutionMode
	if !mode.Valid() {
		mode = ExecutionModeTask
	}
	// Workflow + step are required for task-mode automations (the run is
	// surfaced on the kanban and needs a starting column). Run-mode is
	// ephemeral and bypasses the workflow entirely.
	if mode == ExecutionModeTask {
		if req.WorkflowID == "" {
			return nil, fmt.Errorf("workflow_id is required")
		}
		if req.WorkflowStepID == "" {
			return nil, fmt.Errorf("workflow_step_id is required")
		}
	}
	a := &Automation{
		WorkspaceID:       req.WorkspaceID,
		Name:              req.Name,
		Description:       req.Description,
		WorkflowID:        req.WorkflowID,
		WorkflowStepID:    req.WorkflowStepID,
		AgentProfileID:    req.AgentProfileID,
		ExecutorProfileID: req.ExecutorProfileID,
		RepositoryID:      req.RepositoryID,
		Prompt:            req.Prompt,
		TaskTitleTemplate: req.TaskTitleTemplate,
		ExecutionMode:     mode,
		Enabled:           true,
		MaxConcurrentRuns: maxRuns,
	}
	if err := s.store.CreateAutomation(ctx, a); err != nil {
		return nil, fmt.Errorf("create automation: %w", err)
	}

	// Create initial triggers.
	for _, ts := range req.Triggers {
		t := &AutomationTrigger{
			AutomationID: a.ID,
			Type:         ts.Type,
			Config:       ts.Config,
			Enabled:      ts.Enabled,
		}
		if err := s.store.CreateTrigger(ctx, t); err != nil {
			s.logger.Error("failed to create trigger during automation creation",
				zap.String("automation_id", a.ID),
				zap.String("type", string(ts.Type)),
				zap.Error(err))
		}
	}

	return s.store.GetAutomation(ctx, a.ID)
}

// GetAutomation retrieves an automation by ID.
func (s *Service) GetAutomation(ctx context.Context, id string) (*Automation, error) {
	return s.store.GetAutomation(ctx, id)
}

// ListAutomations returns all automations for a workspace.
func (s *Service) ListAutomations(ctx context.Context, workspaceID string) ([]*Automation, error) {
	return s.store.ListAutomations(ctx, workspaceID)
}

// UpdateAutomation applies partial updates.
func (s *Service) UpdateAutomation(ctx context.Context, id string, req *UpdateAutomationRequest) (*Automation, error) {
	if err := s.store.UpdateAutomation(ctx, id, req); err != nil {
		return nil, err
	}
	return s.store.GetAutomation(ctx, id)
}

// DeleteAutomation removes an automation.
func (s *Service) DeleteAutomation(ctx context.Context, id string) error {
	return s.store.DeleteAutomation(ctx, id)
}

// EnableAutomation sets enabled = true.
func (s *Service) EnableAutomation(ctx context.Context, id string) error {
	enabled := true
	return s.store.UpdateAutomation(ctx, id, &UpdateAutomationRequest{Enabled: &enabled})
}

// DisableAutomation sets enabled = false.
func (s *Service) DisableAutomation(ctx context.Context, id string) error {
	enabled := false
	return s.store.UpdateAutomation(ctx, id, &UpdateAutomationRequest{Enabled: &enabled})
}

// --- Trigger CRUD ---

// AddTrigger adds a trigger to an automation.
func (s *Service) AddTrigger(ctx context.Context, req *AddTriggerRequest) (*AutomationTrigger, error) {
	if req.AutomationID == "" {
		return nil, fmt.Errorf("automation_id is required")
	}
	t := &AutomationTrigger{
		AutomationID: req.AutomationID,
		Type:         req.Type,
		Config:       req.Config,
		Enabled:      req.Enabled,
	}
	if err := s.store.CreateTrigger(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTrigger updates a trigger.
func (s *Service) UpdateTrigger(ctx context.Context, id string, req *UpdateTriggerRequest) error {
	return s.store.UpdateTrigger(ctx, id, req)
}

// DeleteTrigger removes a trigger.
func (s *Service) DeleteTrigger(ctx context.Context, id string) error {
	return s.store.DeleteTrigger(ctx, id)
}

// --- Run queries ---

// ListRuns returns recent runs for an automation.
func (s *Service) ListRuns(ctx context.Context, automationID string, limit int) ([]*AutomationRun, error) {
	return s.store.ListRuns(ctx, automationID, limit)
}

// GetRun returns a single run by ID, or nil if not found.
func (s *Service) GetRun(ctx context.Context, id string) (*AutomationRun, error) {
	return s.store.GetRun(ctx, id)
}

// automationRunLock returns an unlock func for the per-automation mutex that
// serializes run creation (createRunLocked) against DeleteAllRuns.
func (s *Service) automationRunLock(automationID string) func() {
	v, _ := s.runLocks.LoadOrStore(automationID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// createRunLocked persists a run row while holding the per-automation lock
// that DeleteAllRuns also acquires. Without this, a run created between
// DeleteAllRuns' task-id snapshot and its final row purge would be deleted
// without its task ever reaching the TaskDeleter, orphaning the task.
func (s *Service) createRunLocked(ctx context.Context, run *AutomationRun) error {
	defer s.automationRunLock(run.AutomationID)()
	return s.store.CreateRun(ctx, run)
}

// DeleteRun removes a single run and its associated task (if any).
// Task deletion is best-effort: a not-found error is silently ignored so
// stale/orphaned run rows are always removable by the user.
func (s *Service) DeleteRun(ctx context.Context, runID string) error {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}
	if run != nil && run.TaskID != "" && s.taskDeleter != nil {
		if delErr := s.taskDeleter.DeleteTask(ctx, run.TaskID); delErr != nil {
			if !errors.Is(delErr, ErrTaskNotFound) {
				return fmt.Errorf("delete task: %w", delErr)
			}
			s.logger.Debug("run task already gone, continuing delete",
				zap.String("run_id", runID),
				zap.String("task_id", run.TaskID))
		}
	}
	return s.store.DeleteRun(ctx, runID)
}

// DeleteAllRuns removes every run for an automation, deleting each associated
// task first. Task deletion is best-effort: not-found errors are ignored.
func (s *Service) DeleteAllRuns(ctx context.Context, automationID string) error {
	defer s.automationRunLock(automationID)()
	if s.taskDeleter != nil {
		taskIDs, err := s.store.ListRunTaskIDs(ctx, automationID)
		if err != nil {
			return fmt.Errorf("list run task ids: %w", err)
		}
		for _, taskID := range taskIDs {
			if delErr := s.taskDeleter.DeleteTask(ctx, taskID); delErr != nil {
				if !errors.Is(delErr, ErrTaskNotFound) {
					return fmt.Errorf("delete task %s: %w", taskID, delErr)
				}
				s.logger.Debug("run task already gone, skipping",
					zap.String("automation_id", automationID),
					zap.String("task_id", taskID))
			}
		}
	}
	return s.store.DeleteAllRuns(ctx, automationID)
}

// --- Trigger firing ---

// FireTrigger publishes an AutomationTriggered event for the given trigger.
// The orchestrator handles task creation in response.
func (s *Service) FireTrigger(ctx context.Context, automationID, triggerID string, triggerType TriggerType, triggerData json.RawMessage, dedupKey string) error {
	// Check dedup.
	if dedupKey != "" {
		exists, err := s.store.HasRunWithDedupKey(ctx, automationID, dedupKey)
		if err != nil {
			return fmt.Errorf("check dedup: %w", err)
		}
		if exists {
			s.logger.Debug("skipping duplicate trigger",
				zap.String("automation_id", automationID),
				zap.String("dedup_key", dedupKey))
			return nil
		}
	}

	// Enforce max_concurrent_runs: a run is "active" while still in
	// task_created (succeeded/failed/skipped don't count). If at the cap,
	// record a skipped run so the user can see the cap kicked in.
	skipped, capErr := s.maybeSkipForConcurrencyCap(ctx, automationID, triggerID, triggerType, triggerData, dedupKey)
	if capErr != nil {
		return capErr
	}

	// Record that the trigger was evaluated now that the cap check itself
	// has succeeded. Scheduled triggers use this to pace themselves against
	// their cron interval (CronScheduler.shouldFire): if this were updated
	// before the cap check (or despite the cap check returning an error), an
	// infrastructural failure in the check would look like a completed
	// evaluation and suppress the next attempt until the full cron interval
	// elapses, instead of retrying on the next scheduler tick. And if it
	// were only updated on an actual fire, a trigger stuck behind
	// max_concurrent_runs would look "overdue" again on every subsequent
	// tick and get re-evaluated — and re-skipped — far more often than its
	// configured schedule.
	now := time.Now().UTC()
	if updateErr := s.store.UpdateTriggerEvaluatedAt(ctx, triggerID, now); updateErr != nil {
		s.logger.Warn("failed to update last_evaluated_at",
			zap.String("trigger_id", triggerID), zap.Error(updateErr))
	}
	if skipped {
		return nil
	}

	evt := &AutomationTriggeredEvent{
		AutomationID: automationID,
		TriggerID:    triggerID,
		TriggerType:  triggerType,
		TriggerData:  triggerData,
		DedupKey:     dedupKey,
	}

	if updateErr := s.store.UpdateLastTriggered(ctx, automationID, now); updateErr != nil {
		s.logger.Warn("failed to update last_triggered_at",
			zap.String("automation_id", automationID), zap.Error(updateErr))
	}

	event := bus.NewEvent(events.AutomationTriggered, "automation_service", evt)
	if err := s.eventBus.Publish(ctx, events.AutomationTriggered, event); err != nil {
		return fmt.Errorf("publish automation triggered: %w", err)
	}

	s.logger.Info("automation trigger fired",
		zap.String("automation_id", automationID),
		zap.String("trigger_id", triggerID),
		zap.String("type", string(triggerType)))
	return nil
}

// RecordRun records a trigger run outcome.
func (s *Service) RecordRun(ctx context.Context, run *AutomationRun) error {
	return s.createRunLocked(ctx, run)
}

// maybeSkipForConcurrencyCap enforces max_concurrent_runs. Returns (skipped,
// err). When skipped, a "skipped" run row is persisted so the user can see
// the cap kicked in.
func (s *Service) maybeSkipForConcurrencyCap(ctx context.Context, automationID, triggerID string, triggerType TriggerType, triggerData json.RawMessage, dedupKey string) (bool, error) {
	a, err := s.store.GetAutomation(ctx, automationID)
	if err != nil || a == nil {
		return false, nil // not our problem here; FireTrigger will hit it again downstream
	}
	if a.MaxConcurrentRuns <= 0 {
		return false, nil
	}
	active, err := s.store.CountActiveRuns(ctx, automationID)
	if err != nil {
		return false, fmt.Errorf("count active runs: %w", err)
	}
	if active < a.MaxConcurrentRuns {
		return false, nil
	}
	skipRun := &AutomationRun{
		AutomationID: automationID,
		TriggerID:    triggerID,
		TriggerType:  triggerType,
		Status:       RunStatusSkipped,
		DedupKey:     dedupKey,
		TriggerData:  triggerData,
		ErrorMessage: fmt.Sprintf("max_concurrent_runs=%d reached", a.MaxConcurrentRuns),
	}
	if recErr := s.createRunLocked(ctx, skipRun); recErr != nil {
		s.logger.Warn("failed to record skipped run", zap.Error(recErr))
	}
	s.logger.Info("automation trigger skipped: concurrency cap reached",
		zap.String("automation_id", automationID),
		zap.Int("active", active),
		zap.Int("max", a.MaxConcurrentRuns))
	return true, nil
}

// MarkRunFailedByTaskID transitions a still-pending run (task_created) into
// the failed state. Used when something downstream of task creation aborts
// the run, e.g. a permission prompt for a run-mode automation.
func (s *Service) MarkRunFailedByTaskID(ctx context.Context, taskID, errMsg string) error {
	return s.store.MarkRunFailedByTaskID(ctx, taskID, errMsg)
}

// MarkRunSucceededByTaskID transitions a still-pending run (task_created)
// into the succeeded state when the launched agent finishes cleanly.
func (s *Service) MarkRunSucceededByTaskID(ctx context.Context, taskID string) error {
	return s.store.MarkRunSucceededByTaskID(ctx, taskID)
}

// GetWebhookSecret returns the webhook secret for an automation.
func (s *Service) GetWebhookSecret(ctx context.Context, id string) (string, error) {
	a, err := s.store.GetAutomation(ctx, id)
	if err != nil {
		return "", err
	}
	if a == nil {
		return "", fmt.Errorf("automation not found: %s", id)
	}
	return a.WebhookSecret, nil
}
