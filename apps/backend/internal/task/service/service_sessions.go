package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
)

// ErrInvalidMarkSessionRead marks a MarkSessionRead failure as caused by bad
// caller input (empty ids, an unknown message, a message belonging to a
// different session) rather than an unexpected internal failure. The HTTP
// handler uses errors.Is against this to decide 400 vs a sanitized 500 —
// without it, a genuine database/repository error would otherwise be
// indistinguishable from a validation failure and leak as a 400 with the
// raw internal error text.
var ErrInvalidMarkSessionRead = errors.New("invalid mark-session-read request")

// SessionReadyPollInterval is how often WaitForSessionReady polls session state.
const SessionReadyPollInterval = 1 * time.Second

// SessionReadyMaxWait is the maximum time WaitForSessionReady waits before timing out.
const SessionReadyMaxWait = 90 * time.Second

// WaitForSessionReady polls the session state until the agent is ready to accept
// prompts. Returns nil when the session reaches WAITING_FOR_INPUT, or an error if
// it transitions to FAILED/CANCELLED/COMPLETED, the context is cancelled, or the
// timeout is exceeded. Used after ResumeTaskSession to gate the prompt retry until
// the agent has actually finished booting.
func (s *Service) WaitForSessionReady(ctx context.Context, sessionID string) error {
	deadline := time.Now().Add(SessionReadyMaxWait)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for session to become ready after resume")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(SessionReadyPollInterval):
		}
		session, err := s.sessions.GetTaskSession(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to check session state: %w", err)
		}
		switch session.State {
		case models.TaskSessionStateWaitingForInput:
			return nil
		case models.TaskSessionStateFailed:
			errMsg := session.ErrorMessage
			if errMsg == "" {
				errMsg = "session failed during resume"
			}
			return fmt.Errorf("session failed after resume: %s", errMsg)
		case models.TaskSessionStateCancelled, models.TaskSessionStateCompleted:
			return fmt.Errorf("session in unexpected state after resume: %s", session.State)
		default:
			// STARTING or RUNNING — keep polling
		}
	}
}

// ListTaskSessions returns all sessions for a task.
func (s *Service) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	if err := s.authorizeTaskID(ctx, taskID); err != nil {
		return nil, err
	}
	return s.sessions.ListTaskSessions(ctx, taskID)
}

// ListSessionsForPlugin returns sessions across tasks with the filters applied
// before pagination when the repository supports the optimized query. The
// fallback keeps lightweight repository adapters source compatible while the
// production SQLite repository uses one SQL query for the full selection.
func (s *Service) ListSessionsForPlugin(ctx context.Context, filter models.PluginSessionFilter) ([]*models.TaskSession, error) {
	if optimized, ok := s.sessions.(taskrepository.PluginSessionRepository); ok {
		return optimized.ListTaskSessionsForPlugin(ctx, filter)
	}
	return s.listSessionsForPluginFallback(ctx, filter)
}

//nolint:cyclop,gocognit,nestif,funlen // Compatibility adapters must preserve the full filter contract without repository support.
func (s *Service) listSessionsForPluginFallback(ctx context.Context, filter models.PluginSessionFilter) ([]*models.TaskSession, error) {
	taskIDs := append([]string(nil), filter.TaskIDs...)
	if len(taskIDs) == 0 {
		workspaceIDs := append([]string(nil), filter.WorkspaceIDs...)
		if len(workspaceIDs) == 0 {
			workspaces, err := s.workspaces.ListWorkspaces(ctx)
			if err != nil {
				return nil, err
			}
			workspaceIDs = make([]string, 0, len(workspaces))
			for _, workspace := range workspaces {
				workspaceIDs = append(workspaceIDs, workspace.ID)
			}
		}
		for _, workspaceID := range workspaceIDs {
			for page := 1; ; page++ {
				workspaceTaskCount := 0
				tasks, total, err := s.tasks.ListTasksByWorkspace(
					ctx, workspaceID, "", "", "", page, 200, "", true, true, false, true,
				)
				if err != nil {
					return nil, err
				}
				for _, task := range tasks {
					taskIDs = append(taskIDs, task.ID)
					workspaceTaskCount++
				}
				if len(tasks) == 0 || workspaceTaskCount >= total {
					break
				}
			}
		}
	} else if len(filter.WorkspaceIDs) > 0 {
		allowed := make(map[string]struct{}, len(filter.WorkspaceIDs))
		for _, workspaceID := range filter.WorkspaceIDs {
			allowed[workspaceID] = struct{}{}
		}
		filtered := taskIDs[:0]
		for _, taskID := range taskIDs {
			task, err := s.tasks.GetTask(ctx, taskID)
			if err != nil {
				if errors.Is(err, taskrepository.ErrTaskNotFound) {
					continue
				}
				return nil, err
			}
			if _, ok := allowed[task.WorkspaceID]; ok {
				filtered = append(filtered, taskID)
			}
		}
		taskIDs = filtered
	}

	stateSet := make(map[models.TaskSessionState]struct{}, len(filter.States))
	for _, state := range filter.States {
		stateSet[state] = struct{}{}
	}
	sessionSet := make(map[string]struct{}, len(filter.SessionIDs))
	for _, sessionID := range filter.SessionIDs {
		sessionSet[sessionID] = struct{}{}
	}
	var sessions []*models.TaskSession
	for _, taskID := range taskIDs {
		items, err := s.sessions.ListTaskSessions(ctx, taskID)
		if err != nil {
			return nil, err
		}
		for _, session := range items {
			if len(stateSet) > 0 {
				if _, ok := stateSet[session.State]; !ok {
					continue
				}
			}
			if len(sessionSet) > 0 {
				if _, ok := sessionSet[session.ID]; !ok {
					continue
				}
			}
			if filter.UpdatedSince != nil && session.UpdatedAt.Before(*filter.UpdatedSince) {
				continue
			}
			sessions = append(sessions, session)
		}
	}
	sort.Slice(sessions, func(i, j int) bool {
		if !sessions[i].StartedAt.Equal(sessions[j].StartedAt) {
			return sessions[i].StartedAt.After(sessions[j].StartedAt)
		}
		return sessions[i].ID < sessions[j].ID
	})
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(sessions) {
		return []*models.TaskSession{}, nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	end := offset + limit
	if end > len(sessions) {
		end = len(sessions)
	}
	return sessions[offset:end], nil
}

// GetTaskSession returns a single session by ID.
//
// Scoping authorizes off the row we just read rather than calling
// AuthorizeSessionAccess, which would re-read the same session: this is a hot
// path (every session-keyed HTTP route and several MCP tools land here).
func (s *Service) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeTaskID(ctx, session.TaskID); err != nil {
		return nil, err
	}
	return session, nil
}

// DeleteSessionAndPublishRemoval removes a session inside the repository's
// transaction and publishes the terminal event only after that commit.
// Internal rollback callers use this to keep the ordered stream authoritative.
func (s *Service) DeleteSessionAndPublishRemoval(ctx context.Context, sessionID string) error {
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := s.sessions.DeleteTaskSession(ctx, session); err != nil {
		return err
	}
	if s.eventBus == nil {
		return nil
	}
	return s.eventBus.Publish(ctx, events.SessionRemoved, bus.NewEvent(
		events.SessionRemoved,
		"task-service",
		map[string]interface{}{sessionEventFieldSessionID: session.ID, sessionEventFieldTaskID: session.TaskID},
	))
}

// GetExecutorRunningBySessionID returns the live executor row for sessionID,
// or models.ErrExecutorRunningNotFound if the session has none (e.g. it
// never started, or has since completed and been cleaned up). Exposed at
// the service layer — rather than requiring callers to reach into
// repository.ExecutorRepository directly — for the Host data API's
// acp_session_id fallback (ADR 0043): a session's ACP conversation id is
// normally read from TaskSession.Metadata["acp"]["session_id"], but that key
// is only populated once the agent has emitted a session_info frame;
// executors_running.resume_token carries the same id and survives on
// sessions that never got that far.
func (s *Service) GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error) {
	return s.executors.GetExecutorRunningBySessionID(ctx, sessionID)
}

func (s *Service) DismissLastAgentError(ctx context.Context, sessionID, stamp string) (*models.TaskSession, error) {
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeTaskID(ctx, session.TaskID); err != nil {
		return nil, err
	}
	lastErr, ok := models.LoadLastAgentError(session.Metadata)
	if !ok {
		return session, nil
	}
	if stamp != "" && !lastErr.MatchesStamp(stamp) {
		return session, nil
	}
	now := time.Now().UTC()
	updated, err := s.sessions.DismissLastAgentError(context.WithoutCancel(ctx), sessionID, lastErr, now)
	if err != nil {
		return nil, err
	}
	if !updated {
		return session, nil
	}
	if s.eventBus != nil {
		eventCtx := context.WithoutCancel(ctx)
		eventData := map[string]interface{}{
			"task_id":      session.TaskID,
			"session_id":   sessionID,
			"active":       false,
			"stamp":        lastErr.Stamp(),
			"dismissed_at": now.Format(time.RFC3339Nano),
		}
		if err := s.eventBus.Publish(eventCtx, events.TaskSessionErrorChanged, bus.NewEvent(
			events.TaskSessionErrorChanged,
			"task-service",
			eventData,
		)); err != nil {
			s.logger.Warn("failed to publish task session error dismissal event",
				zap.String("task_id", session.TaskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}
	return s.sessions.GetTaskSession(context.WithoutCancel(ctx), sessionID)
}

// MarkSessionRead advances sessionID's Slack-style read cursor to messageID.
// messageID must belong to sessionID — a stale id, a synthetic frontend-only
// row (e.g. the task-description placeholder), or a cross-session id is
// rejected rather than persisted, since a wrong cursor would hide genuinely
// unread messages behind a mispositioned "New" divider. Every rejection
// caused by bad caller input (as opposed to an unexpected repository/DB
// failure) wraps ErrInvalidMarkSessionRead so the HTTP handler can return
// 400 for the former and a sanitized, logged 500 for the latter.
func (s *Service) MarkSessionRead(ctx context.Context, sessionID, messageID string) (*models.TaskSession, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("%w: session_id is required", ErrInvalidMarkSessionRead)
	}
	if messageID == "" {
		return nil, fmt.Errorf("%w: message_id is required", ErrInvalidMarkSessionRead)
	}
	message, err := s.messages.GetMessage(ctx, messageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: message %s not found", ErrInvalidMarkSessionRead, messageID)
		}
		return nil, fmt.Errorf("failed to look up message %s: %w", messageID, err)
	}
	if message.TaskSessionID != sessionID {
		return nil, fmt.Errorf("%w: message %s does not belong to session %s", ErrInvalidMarkSessionRead, messageID, sessionID)
	}
	if err := s.sessions.UpdateTaskSessionLastReadMessageID(ctx, sessionID, messageID); err != nil {
		return nil, err
	}
	return s.sessions.GetTaskSession(ctx, sessionID)
}

// GetPrimarySession returns the primary session for a task.
func (s *Service) GetPrimarySession(ctx context.Context, taskID string) (*models.TaskSession, error) {
	if err := s.authorizeTaskID(ctx, taskID); err != nil {
		return nil, err
	}
	return s.sessions.GetPrimarySessionByTaskID(ctx, taskID)
}

// GetPrimarySessionIDsForTasks returns a map of task ID to primary session ID for the given task IDs.
// Tasks without a primary session are not included in the result.
func (s *Service) GetPrimarySessionIDsForTasks(ctx context.Context, taskIDs []string) (map[string]string, error) {
	return s.sessions.GetPrimarySessionIDsByTaskIDs(ctx, taskIDs)
}

// GetSessionCountsForTasks returns a map of task ID to session count for the given task IDs.
func (s *Service) GetSessionCountsForTasks(ctx context.Context, taskIDs []string) (map[string]int, error) {
	return s.sessions.GetSessionCountsByTaskIDs(ctx, taskIDs)
}

// GetPrimarySessionInfoForTasks returns a map of task ID to primary session info for the given task IDs.
func (s *Service) GetPrimarySessionInfoForTasks(ctx context.Context, taskIDs []string) (map[string]*models.TaskSession, error) {
	return s.sessions.GetPrimarySessionInfoByTaskIDs(ctx, taskIDs)
}

// GetPendingActionsForSessions returns pending user-action projections for
// sessions whose messages may not be loaded by list views.
func (s *Service) GetPendingActionsForSessions(ctx context.Context, sessionIDs []string) (map[string]models.TaskPendingAction, error) {
	return s.messages.GetPendingActionsBySessionIDs(ctx, sessionIDs)
}

// BatchGetSessionsForTasks returns all sessions for the given task IDs grouped
// by task ID. Wraps the repository batch loader so callers in the handler
// layer can derive primary session, session count, and per-task session info
// from one round trip instead of three.
func (s *Service) BatchGetSessionsForTasks(ctx context.Context, taskIDs []string) (map[string][]*models.TaskSession, error) {
	return s.sessions.BatchGetSessionsByTaskIDs(ctx, taskIDs)
}

// SetPrimarySession sets a session as the primary session for its task.
// This will unset any existing primary session for the same task.
func (s *Service) SetPrimarySession(ctx context.Context, sessionID string) error {
	if err := s.sessions.SetSessionPrimary(ctx, sessionID); err != nil {
		s.logger.Error("failed to set primary session",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return err
	}
	return nil
}

// UpdateSessionReviewStatus updates the review status of a session.
func (s *Service) UpdateSessionReviewStatus(ctx context.Context, sessionID string, status string) error {
	if err := s.sessions.UpdateSessionReviewStatus(ctx, sessionID, status); err != nil {
		s.logger.Error("failed to update session review status",
			zap.String("session_id", sessionID),
			zap.String("status", status),
			zap.Error(err))
		return err
	}
	return nil
}
