package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
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
	// MarkRunFailed bypasses transitionRunTerminal (this is the office v1
	// failure path, not FailRun), so the checkout release that lives there
	// has to be duplicated here — otherwise every agent-error terminal
	// transition leaks the task checkout the same way FinishRun used to.
	s.releaseTaskCheckoutForRun(ctx, run)
	// Leave "working" before the auto-pause decision below, not after: the
	// reset is a working → idle CAS, so running it first lets a subsequent
	// auto-pause overwrite idle with paused, while running it last would
	// find the agent already paused and silently do nothing. Ordering it
	// here keeps the agent out of a stuck "working" even if the failure
	// bookkeeping that follows errors out.
	s.clearAgentWorking(ctx, run.AgentProfileID, run.ID)

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

// MarkAgentRunFailedFixed clears the FAILED state on the (task, agent)
// session, re-queues a run for that pair, and then dismisses the inbox entry.
// Used by the inbox "Mark fixed" action.
func (s *Service) MarkAgentRunFailedFixed(
	ctx context.Context, userID, runID string,
) error {
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		// Run vanished (e.g. cancelled by reactivity) — dismissal
		// alone is the best we can do. No retry needed.
		if dismissErr := s.repo.DismissInboxItem(
			ctx, userID, InboxKindAgentRunFailed, runID,
		); dismissErr != nil {
			return fmt.Errorf("dismiss: %w", dismissErr)
		}
		s.logger.Info("mark fixed: run not found, dismissed only",
			zap.String("run_id", runID))
		return nil
	}
	taskID := taskIDFromRunPayload(run.Payload)
	if taskID == "" {
		return s.repo.DismissInboxItem(ctx, userID, InboxKindAgentRunFailed, runID)
	}
	if err := s.requeueRunForTask(ctx, run.AgentProfileID, taskID, runID); err != nil {
		return fmt.Errorf("requeue run: %w", err)
	}
	if err := s.repo.DismissInboxItem(ctx, userID, InboxKindAgentRunFailed, runID); err != nil {
		return fmt.Errorf("dismiss: %w", err)
	}
	return nil
}

// MarkAgentPausedFixed unpauses an auto-paused agent, clears the
// counter, dismisses the inbox entry, and re-queues task_assigned
// runs for every task whose current assignee is still this agent
// and whose most recent run is failed.
func (s *Service) MarkAgentPausedFixed(
	ctx context.Context, userID, agentID string,
) error {
	agent, err := s.repo.GetAgentInstance(ctx, agentID)
	if err != nil {
		return fmt.Errorf("get agent: %w", err)
	}
	autoPaused := strings.HasPrefix(agent.PauseReason, autoPauseReasonPrefix)
	recoveries, err := s.loadPauseRecoveries(ctx, agent, autoPaused)
	if err != nil {
		return err
	}
	if !autoPaused && len(recoveries) == 0 {
		// Both the pause marker and its durable recovery work are gone.
		return s.repo.DismissInboxItem(ctx, userID, InboxKindAgentPausedAfterFails, agentID)
	}
	if err := s.repo.DismissInboxItem(
		ctx, userID, InboxKindAgentPausedAfterFails, agentID,
	); err != nil {
		return fmt.Errorf("dismiss: %w", err)
	}

	if autoPaused {
		if err := s.clearAutoPause(ctx, agent); err != nil {
			return err
		}
		if err := s.repo.ResetAgentConsecutiveFailures(ctx, agentID); err != nil {
			s.logger.Warn("reset counter on unpause failed",
				zap.String("agent", agentID), zap.Error(err))
		}
	}
	return s.recoverPausedTasks(ctx, agentID, recoveries)
}

func (s *Service) loadPauseRecoveries(
	ctx context.Context, agent *models.AgentInstance, autoPaused bool,
) ([]officesqlite.AgentPauseRecovery, error) {
	recoveries, err := s.repo.ListAgentPauseRecoveries(ctx, agent.ID)
	if err != nil {
		return nil, fmt.Errorf("list pause recoveries: %w", err)
	}
	if len(recoveries) == 0 && autoPaused {
		// Populate the snapshot for auto-paused agents created before this
		// table was introduced, then use the same safe recovery path.
		if err := s.repo.ReplaceAgentPauseRecoveries(
			ctx, agent.ID, agent.ConsecutiveFailures,
		); err != nil {
			return nil, fmt.Errorf("capture pause recoveries: %w", err)
		}
		recoveries, err = s.repo.ListAgentPauseRecoveries(ctx, agent.ID)
		if err != nil {
			return nil, fmt.Errorf("reload pause recoveries: %w", err)
		}
	}
	return recoveries, nil
}

func (s *Service) clearAutoPause(
	ctx context.Context, agent *models.AgentInstance,
) error {
	for attempt := 0; attempt < 2; attempt++ {
		changed, err := s.clearAutoPauseAttempt(ctx, agent)
		if err != nil {
			return err
		}
		if changed {
			return nil
		}

		current, err := s.repo.GetAgentInstance(ctx, agent.ID)
		if err != nil {
			return fmt.Errorf("reload agent status: %w", err)
		}
		if !strings.HasPrefix(current.PauseReason, autoPauseReasonPrefix) {
			return nil
		}
		agent = current
	}
	return fmt.Errorf("clear pause reason: agent status changed")
}

func (s *Service) clearAutoPauseAttempt(
	ctx context.Context, agent *models.AgentInstance,
) (bool, error) {
	if agent.Status == models.AgentStatusPaused {
		return s.unpauseAgentIfCurrent(ctx, agent)
	}
	return s.clearPauseReasonIfCurrent(ctx, agent)
}

func (s *Service) unpauseAgentIfCurrent(
	ctx context.Context, agent *models.AgentInstance,
) (bool, error) {
	changed, err := s.repo.UpdateAgentStatusFieldsIfCurrent(
		ctx, agent.ID, string(models.AgentStatusPaused),
		string(models.AgentStatusIdle), "",
	)
	if err != nil {
		return false, fmt.Errorf("unpause agent: %w", err)
	}
	if changed {
		s.publishAgentStatusChanged(
			ctx, agent.ID, agent.WorkspaceID, string(models.AgentStatusIdle),
		)
	}
	return changed, nil
}

func (s *Service) clearPauseReasonIfCurrent(
	ctx context.Context, agent *models.AgentInstance,
) (bool, error) {
	changed, err := s.repo.ClearAgentPauseReasonIfCurrent(
		ctx, agent.ID, string(agent.Status),
	)
	if err != nil {
		return false, fmt.Errorf("clear pause reason: %w", err)
	}
	if changed {
		s.publishAgentStatusChanged(
			ctx, agent.ID, agent.WorkspaceID, string(agent.Status),
		)
	}
	return changed, nil
}

func (s *Service) recoverPausedTasks(
	ctx context.Context, agentID string,
	recoveries []officesqlite.AgentPauseRecovery,
) error {
	var recoveryErrs []error
	for _, recovery := range recoveries {
		if err := s.recoverPausedTask(ctx, agentID, recovery); err != nil {
			recoveryErrs = append(recoveryErrs,
				fmt.Errorf("recover task %s: %w", recovery.TaskID, err))
		}
	}
	return errors.Join(recoveryErrs...)
}

func (s *Service) recoverPausedTask(
	ctx context.Context, agentID string,
	recovery officesqlite.AgentPauseRecovery,
) error {
	fields, err := s.repo.GetTaskExecutionFields(ctx, recovery.TaskID)
	if errors.Is(err, officesqlite.ErrTaskNotFound) {
		return s.discardPauseRecovery(ctx, recovery)
	}
	if err != nil {
		return fmt.Errorf("load task: %w", err)
	}
	if fields.AssigneeAgentProfileID != agentID {
		return s.discardPauseRecovery(ctx, recovery)
	}

	latest, err := s.repo.GetLatestRunForAgentTask(ctx, agentID, recovery.TaskID)
	if err != nil {
		return fmt.Errorf("load latest run: %w", err)
	}
	if latest == nil || latest.ID != recovery.FailedRunID ||
		latest.Status != models.RunStatusFailed {
		return s.discardPauseRecovery(ctx, recovery)
	}
	if err := s.requeueRunForTask(
		ctx, agentID, recovery.TaskID, recovery.FailedRunID,
	); err != nil {
		return fmt.Errorf("requeue task: %w", err)
	}
	if err := s.repo.DeleteAgentPauseRecovery(
		ctx, agentID, recovery.TaskID,
	); err != nil {
		return fmt.Errorf("delete recovery: %w", err)
	}
	_ = s.repo.DismissInboxItem(
		ctx, autoDismissUserID, InboxKindAgentRunFailed, recovery.FailedRunID,
	)
	return nil
}

func (s *Service) discardPauseRecovery(
	ctx context.Context, recovery officesqlite.AgentPauseRecovery,
) error {
	if err := s.repo.DeleteAgentPauseRecovery(
		ctx, recovery.AgentID, recovery.TaskID,
	); err != nil {
		return fmt.Errorf("delete recovery: %w", err)
	}
	_ = s.repo.DismissInboxItem(
		ctx, autoDismissUserID, InboxKindAgentRunFailed, recovery.FailedRunID,
	)
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
	if err := s.repo.ReplaceAgentPauseRecoveries(ctx, agentID, count); err != nil {
		return fmt.Errorf("capture pause recoveries: %w", err)
	}
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

// requeueRunForTask is a manual resume: the failed run's own id is the
// occurrence identity (falling back to the task id if it is somehow empty),
// so a duplicate "Mark fixed" click dedupes instead of double-queuing.
func (s *Service) requeueRunForTask(
	ctx context.Context, agentID, taskID, failedRunID string,
) error {
	payload := mustJSONString(map[string]string{"task_id": taskID})
	identity := failedRunID
	if identity == "" {
		identity = taskID
	}
	key := fmt.Sprintf("%s:%s:%s", RunReasonManualResumeAfterFailure, agentID, identity)
	_, err := s.QueueRun(ctx, agentID, RunReasonManualResumeAfterFailure, payload, key)
	return err
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
