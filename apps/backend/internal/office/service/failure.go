package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
)

// Inbox item kinds for office-agent-error-handling.
const (
	InboxKindAgentRunFailed           = "agent_run_failed"
	InboxKindAgentPausedAfterFails    = "agent_paused_after_failures"
	autoPauseReasonPrefix             = "Auto-paused:"
	RunReasonManualResumeAfterFailure = "manual_resume_after_failure"
)

// HandleAgentFailure is the v1 office failure path: every agent error
// is treated as terminal. The run is marked failed with the verbatim
// error message, the consecutive-failure counter is incremented, and
// when it crosses the effective threshold the agent is auto-paused.
//
// No retry is scheduled — the user resolves via Resume session in the
// chat or Mark fixed in the inbox.
func (s *Service) HandleAgentFailure(
	ctx context.Context,
	run *models.Run,
	errorMessage string,
) error {
	if err := s.repo.MarkRunFailed(ctx, run.ID, errorMessage); err != nil {
		return fmt.Errorf("mark run failed: %w", err)
	}

	count, err := s.repo.IncrementAgentConsecutiveFailures(ctx, run.AgentProfileID)
	if err != nil {
		s.logger.Warn("failed to increment consecutive failures",
			zap.String("agent", run.AgentProfileID), zap.Error(err))
		count = 0
	}

	threshold, err := s.repo.GetEffectiveFailureThreshold(ctx, run.AgentProfileID)
	if err != nil {
		s.logger.Warn("failed to read effective failure threshold",
			zap.String("agent", run.AgentProfileID), zap.Error(err))
		threshold = 3
	}

	s.publishRunFailed(ctx, run, errorMessage, count, threshold)

	if count >= threshold {
		if err := s.autoPauseAgent(ctx, run.AgentProfileID, count, errorMessage); err != nil {
			s.logger.Error("auto-pause failed",
				zap.String("agent", run.AgentProfileID), zap.Error(err))
		}
	}

	return nil
}

// RecordAgentSuccess resets the consecutive-failure counter for the
// agent. Called from the AgentTurnMessageSaved bridge so any
// successful turn (which is what produces the bridged comment) clears
// the counter regardless of which task succeeded.
func (s *Service) RecordAgentSuccess(ctx context.Context, agentID string) {
	if agentID == "" {
		return
	}
	if err := s.repo.ResetAgentConsecutiveFailures(ctx, agentID); err != nil {
		s.logger.Warn("reset consecutive failures failed",
			zap.String("agent", agentID), zap.Error(err))
	}
}

// MarkAgentRunFailedFixed dismisses the per-task inbox entry, clears
// the FAILED state on the (task, agent) session, and re-queues a
// run for that pair. Used by the inbox "Mark fixed" action.
func (s *Service) MarkAgentRunFailedFixed(
	ctx context.Context, userID, runID string,
) error {
	if err := s.repo.DismissInboxItem(ctx, userID, InboxKindAgentRunFailed, runID); err != nil {
		return fmt.Errorf("dismiss: %w", err)
	}
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		// Run vanished (e.g. cancelled by reactivity) — dismissal
		// alone is the best we can do. No retry needed.
		s.logger.Info("mark fixed: run not found, dismissed only",
			zap.String("run_id", runID))
		return nil
	}
	taskID := taskIDFromRunPayload(run.Payload)
	if taskID == "" {
		return nil
	}
	return s.requeueRunForTask(ctx, run.AgentProfileID, taskID)
}

// MarkAgentPausedFixed unpauses an auto-paused agent, clears the
// counter, dismisses the inbox entry, and re-queues task_assigned
// runs for every task whose current assignee is still this agent
// and whose most recent run is failed.
func (s *Service) MarkAgentPausedFixed(
	ctx context.Context, userID, agentID string,
) error {
	if err := s.repo.DismissInboxItem(ctx, userID, InboxKindAgentPausedAfterFails, agentID); err != nil {
		return fmt.Errorf("dismiss: %w", err)
	}

	agent, err := s.repo.GetAgentInstance(ctx, agentID)
	if err != nil {
		return fmt.Errorf("get agent: %w", err)
	}
	if !strings.HasPrefix(agent.PauseReason, autoPauseReasonPrefix) {
		// Already cleared by something else; nothing to do.
		return nil
	}

	if err := s.repo.UpdateAgentStatusFields(ctx, agentID, string(agent.Status), ""); err != nil {
		return fmt.Errorf("clear pause reason: %w", err)
	}
	if err := s.repo.ResetAgentConsecutiveFailures(ctx, agentID); err != nil {
		s.logger.Warn("reset counter on unpause failed",
			zap.String("agent", agentID), zap.Error(err))
	}

	// Re-queue runs for the tasks affected by the pause AND
	// auto-dismiss the prior per-task failure entries so unpausing
	// doesn't resurface them in the inbox. The pause entry is the
	// single source of truth for this batch of failures; once the
	// user marks it fixed, the per-task entries are no longer
	// actionable on their own.
	runIDs, err := s.repo.ListFailedRunsForAgent(ctx, agentID)
	if err != nil {
		return fmt.Errorf("list failed runs: %w", err)
	}
	seenTasks := map[string]bool{}
	for _, wID := range runIDs {
		// Auto-dismiss every prior failed run row so it doesn't
		// re-emerge in the inbox when the agent unpauses.
		_ = s.repo.DismissInboxItem(ctx, autoDismissUserID, InboxKindAgentRunFailed, wID)
		w, err := s.repo.GetRun(ctx, wID)
		if err != nil {
			continue
		}
		taskID := taskIDFromRunPayload(w.Payload)
		if taskID == "" || seenTasks[taskID] {
			continue
		}
		seenTasks[taskID] = true
		if err := s.requeueRunForTask(ctx, agentID, taskID); err != nil {
			s.logger.Warn("requeue on unpause failed",
				zap.String("agent", agentID), zap.String("task_id", taskID),
				zap.Error(err))
		}
	}
	return nil
}

// IsInboxItemDismissed delegates to the repository — exposed so the
// inbox query layer (and tests) can check dismissal status without
// reaching past the service boundary.
func (s *Service) IsInboxItemDismissed(
	ctx context.Context, userID, kind, itemID string,
) (bool, error) {
	return s.repo.IsInboxItemDismissed(ctx, userID, kind, itemID)
}

// GetRun exposes the run repo read so tests and the
// inbox query layer can fetch a run by id.
func (s *Service) GetRun(
	ctx context.Context, id string,
) (*models.Run, error) {
	return s.repo.GetRun(ctx, id)
}

// FailedRunInboxRow is the slim view of one failed run ready for
// the inbox layer. Service-package shape — main.go adapts it to the
// dashboard package's FailureInboxRow.
type FailedRunInboxRow struct {
	RunID          string
	AgentProfileID string
	AgentName      string
	WorkspaceID    string
	TaskID         string
	ErrorMessage   string
	FailedAt       time.Time
}

// PausedAgentInboxRow is the slim view of one auto-paused agent.
type PausedAgentInboxRow struct {
	AgentID             string
	AgentName           string
	WorkspaceID         string
	PauseReason         string
	UpdatedAt           time.Time
	ConsecutiveFailures int
}

// ListFailedRunInboxRows returns failed runs for the workspace
// that aren't dismissed by the given user, excluding agents currently
// auto-paused. Used by the dashboard inbox.
func (s *Service) ListFailedRunInboxRows(
	ctx context.Context, workspaceID, userID string,
) ([]FailedRunInboxRow, error) {
	rows, err := s.repo.ListFailedRunsForInbox(ctx, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]FailedRunInboxRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, FailedRunInboxRow{
			RunID:          r.RunID,
			AgentProfileID: r.AgentProfileID,
			AgentName:      r.AgentName,
			WorkspaceID:    r.WorkspaceID,
			TaskID:         r.TaskID,
			ErrorMessage:   r.ErrorMessage,
			FailedAt:       r.FailedAt,
		})
	}
	return out, nil
}

// ListPausedAgentInboxRows returns auto-paused agents for the
// workspace that aren't dismissed by the given user.
func (s *Service) ListPausedAgentInboxRows(
	ctx context.Context, workspaceID, userID string,
) ([]PausedAgentInboxRow, error) {
	rows, err := s.repo.ListAutoPausedAgentsForInbox(ctx, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]PausedAgentInboxRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, PausedAgentInboxRow{
			AgentID:             r.AgentID,
			AgentName:           r.AgentName,
			WorkspaceID:         r.WorkspaceID,
			PauseReason:         r.PauseReason,
			UpdatedAt:           r.UpdatedAt,
			ConsecutiveFailures: r.ConsecutiveFailures,
		})
	}
	return out, nil
}

// OnAssigneeChanged is called by the reactivity layer when a task's
// assignee changes from oldAgentID to newAgentID. The per-task inbox
// entry for the OLD pair is auto-dismissed since it's no longer
// actionable (the failure happened on a different agent than the one
// currently assigned). The agent's consecutive-failure counter is
// NOT reset — the root cause may still be unfixed.
func (s *Service) OnAssigneeChanged(
	ctx context.Context, taskID, oldAgentID string,
) {
	if taskID == "" || oldAgentID == "" {
		return
	}
	runIDs, err := s.repo.ListFailedRunsForAgent(ctx, oldAgentID)
	if err != nil {
		return
	}
	for _, wID := range runIDs {
		w, err := s.repo.GetRun(ctx, wID)
		if err != nil {
			continue
		}
		if taskIDFromRunPayload(w.Payload) != taskID {
			continue
		}
		// Dismiss for both the default user and the auto-dismiss
		// sentinel so the entry vanishes for everyone.
		_ = s.repo.DismissInboxItem(ctx, autoDismissUserID, InboxKindAgentRunFailed, w.ID)
	}
}

// autoDismissUserID is the sentinel written for system-driven
// dismissals (auto-dismiss on assignee change). The inbox query
// treats this row as a global dismissal.
const autoDismissUserID = "_auto"

func (s *Service) autoPauseAgent(
	ctx context.Context, agentID string, count int, errorMessage string,
) error {
	agent, err := s.repo.GetAgentInstance(ctx, agentID)
	if err != nil {
		return fmt.Errorf("get agent: %w", err)
	}
	reason := fmt.Sprintf("%s %d consecutive failures. Last error: %s",
		autoPauseReasonPrefix, count, truncateForReason(errorMessage))
	if err := s.repo.UpdateAgentStatusFields(
		ctx, agentID, string(models.AgentStatusPaused), reason,
	); err != nil {
		return fmt.Errorf("set pause reason: %w", err)
	}
	s.logger.Warn("agent auto-paused",
		zap.String("agent", agentID), zap.String("name", agent.Name),
		zap.Int("consecutive_failures", count))
	s.publishAgentAutoPaused(ctx, agent, count, errorMessage)
	return nil
}

func (s *Service) requeueRunForTask(
	ctx context.Context, agentID, taskID string,
) error {
	payload := mustJSONString(map[string]string{"task_id": taskID})
	return s.QueueRun(ctx, agentID, RunReasonManualResumeAfterFailure, payload, "")
}

func (s *Service) publishRunFailed(
	ctx context.Context, run *models.Run,
	errorMessage string, count, threshold int,
) {
	if s.eb == nil {
		return
	}
	data := map[string]interface{}{
		"run_id":               run.ID,
		"agent_profile_id":     run.AgentProfileID,
		"task_id":              taskIDFromRunPayload(run.Payload),
		"error_message":        errorMessage,
		"consecutive_failures": count,
		"threshold":            threshold,
		"finished_at":          time.Now().UTC().Format(time.RFC3339),
	}
	_ = s.eb.Publish(ctx, "office.run.failed",
		bus.NewEvent("office.run.failed", "office-failure", data))
}

func (s *Service) publishAgentAutoPaused(
	ctx context.Context, agent *models.AgentInstance,
	count int, errorMessage string,
) {
	if s.eb == nil {
		return
	}
	data := map[string]interface{}{
		"agent_profile_id":     agent.ID,
		"workspace_id":         agent.WorkspaceID,
		"consecutive_failures": count,
		"last_error":           errorMessage,
		"paused_at":            time.Now().UTC().Format(time.RFC3339),
	}
	_ = s.eb.Publish(ctx, "office.agent.auto_paused",
		bus.NewEvent("office.agent.auto_paused", "office-failure", data))
}

func taskIDFromRunPayload(payload string) string {
	if payload == "" {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return ""
	}
	if v, ok := m["task_id"].(string); ok {
		return v
	}
	return ""
}

func mustJSONString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func truncateForReason(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
