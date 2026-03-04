package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// handleGitEvent handles unified git events and dispatches to appropriate handler
func (s *Service) handleGitEvent(ctx context.Context, data watcher.GitEventData) {
	s.logger.Debug("handling git event",
		zap.String("type", string(data.Type)),
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID))

	if data.SessionID == "" {
		s.logger.Debug("missing session_id for git event",
			zap.String("task_id", data.TaskID),
			zap.String("type", string(data.Type)))
		return
	}

	switch data.Type {
	case lifecycle.GitEventTypeStatusUpdate:
		s.handleGitStatusUpdate(ctx, data)
	case lifecycle.GitEventTypeCommitCreated:
		s.handleGitCommitCreated(ctx, data)
	case lifecycle.GitEventTypeCommitsReset:
		s.handleGitCommitsReset(ctx, data)
	case lifecycle.GitEventTypeSnapshotCreated:
		// Snapshot events are published from orchestrator, no need to handle here
		s.logger.Debug("received snapshot_created event, no action needed",
			zap.String("session_id", data.SessionID))
	default:
		s.logger.Warn("unknown git event type",
			zap.String("type", string(data.Type)),
			zap.String("session_id", data.SessionID))
	}
}

// handleGitStatusUpdate handles git status updates by forwarding them to the frontend.
// In the live model, git status is not persisted to DB - the frontend queries agentctl directly.
func (s *Service) handleGitStatusUpdate(ctx context.Context, data watcher.GitEventData) {
	if data.Status == nil {
		s.logger.Debug("missing status data for git status update",
			zap.String("task_id", data.TaskID))
		return
	}

	// Forward status_update event to WebSocket subject for frontend
	// The frontend uses this for real-time updates during active sessions
	if s.eventBus != nil {
		event := bus.NewEvent(events.GitWSEvent, "orchestrator", &data)
		_ = s.eventBus.Publish(ctx, events.BuildGitWSEventSubject(data.SessionID), event)
	}

	// Push detection: when ahead goes from >0 to 0, a push happened
	s.trackPushAndAssociatePR(ctx, data)
}

// trackPushAndAssociatePR detects git pushes by tracking the "ahead" count.
// When ahead transitions from >0 to 0 with a remote branch set, a push occurred.
func (s *Service) trackPushAndAssociatePR(ctx context.Context, data watcher.GitEventData) {
	prevAheadVal, loaded := s.pushTracker.Swap(data.SessionID, data.Status.Ahead)
	if !loaded {
		return // first status update for this session, skip
	}
	prevAhead, ok := prevAheadVal.(int)
	if !ok || prevAhead <= 0 {
		return
	}
	// Push detected: ahead went from >0 to 0
	if data.Status.Ahead == 0 && data.Status.RemoteBranch != "" {
		go s.detectPushAndAssociatePR(
			context.Background(),
			data.SessionID,
			data.TaskID,
			data.Status.Branch,
		)
	}
}

// handleContextWindowUpdated handles context window updates and persists them to session metadata
func (s *Service) handleContextWindowUpdated(ctx context.Context, data watcher.ContextWindowData) {
	s.logger.Debug("handling context window update",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.TaskSessionID),
		zap.Int64("size", data.ContextWindowSize),
		zap.Int64("used", data.ContextWindowUsed))

	if data.TaskSessionID == "" {
		s.logger.Debug("missing session_id for context window update",
			zap.String("task_id", data.TaskID))
		return
	}

	contextWindowData := map[string]interface{}{
		"size":       data.ContextWindowSize,
		"used":       data.ContextWindowUsed,
		"remaining":  data.ContextWindowRemaining,
		"efficiency": data.ContextEfficiency,
	}

	// Persist to database asynchronously. Read the session inside the goroutine
	// to get the latest metadata (avoids race with setSessionPlanMode etc.).
	go func() {
		session, err := s.repo.GetTaskSession(context.Background(), data.TaskSessionID)
		if err != nil {
			s.logger.Debug("no task session for context window update",
				zap.String("session_id", data.TaskSessionID),
				zap.Error(err))
			return
		}
		if session.Metadata == nil {
			session.Metadata = make(map[string]interface{})
		}
		session.Metadata["context_window"] = contextWindowData
		if err := s.repo.UpdateSessionMetadata(context.Background(), session.ID, session.Metadata); err != nil {
			s.logger.Error("failed to update session with context window",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", session.ID),
				zap.Error(err))
		} else {
			s.logger.Debug("persisted context window to session",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", session.ID))
		}
	}()

	// Broadcast context window update so the frontend can update in real-time.
	// This uses the existing session.state_changed event with metadata included.
	if s.eventBus != nil {
		_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
			events.TaskSessionStateChanged,
			"orchestrator",
			map[string]interface{}{
				"task_id":    data.TaskID,
				"session_id": data.TaskSessionID,
				"metadata": map[string]interface{}{
					"context_window": contextWindowData,
				},
			},
		))
	}
}

// handlePermissionRequest handles permission request events and saves as message
func (s *Service) handlePermissionRequest(ctx context.Context, data watcher.PermissionRequestData) {
	s.logger.Debug("handling permission request",
		zap.String("task_id", data.TaskID),
		zap.String("pending_id", data.PendingID),
		zap.String("title", data.Title))

	if data.TaskSessionID == "" {
		s.logger.Warn("missing session_id for permission_request",
			zap.String("task_id", data.TaskID),
			zap.String("pending_id", data.PendingID))
		return
	}

	s.setSessionWaitingForInput(ctx, data.TaskID, data.TaskSessionID)

	if s.messageCreator != nil {
		_, err := s.messageCreator.CreatePermissionRequestMessage(
			ctx,
			data.TaskID,
			data.TaskSessionID,
			data.PendingID,
			data.ToolCallID,
			data.Title,
			s.getActiveTurnID(data.TaskSessionID),
			data.Options,
			data.ActionType,
			data.ActionDetails,
		)
		if err != nil {
			s.logger.Error("failed to create permission request message",
				zap.String("task_id", data.TaskID),
				zap.String("pending_id", data.PendingID),
				zap.Error(err))
		} else {
			s.logger.Debug("created permission request message",
				zap.String("task_id", data.TaskID),
				zap.String("pending_id", data.PendingID))
		}
	}
}

// handleGitCommitCreated handles git commit events by forwarding them to the frontend.
// In the live model, commits are not persisted to DB - they're only captured at archive time.
func (s *Service) handleGitCommitCreated(ctx context.Context, data watcher.GitEventData) {
	if data.Commit == nil {
		s.logger.Debug("missing commit data for git commit event",
			zap.String("task_id", data.TaskID))
		return
	}

	s.logger.Debug("handling git commit created",
		zap.String("task_id", data.TaskID),
		zap.String("commit_sha", data.Commit.CommitSHA))

	// Forward commit_created event to WebSocket subject for frontend real-time updates
	if s.eventBus != nil {
		event := bus.NewEvent(events.GitEvent, "orchestrator", &lifecycle.GitEventPayload{
			Type:      lifecycle.GitEventTypeCommitCreated,
			SessionID: data.SessionID,
			TaskID:    data.TaskID,
			Timestamp: time.Now().Format("2006-01-02T15:04:05.000000000Z07:00"),
			Commit: &lifecycle.GitCommitData{
				CommitSHA:    data.Commit.CommitSHA,
				ParentSHA:    data.Commit.ParentSHA,
				Message:      data.Commit.Message,
				AuthorName:   data.Commit.AuthorName,
				AuthorEmail:  data.Commit.AuthorEmail,
				FilesChanged: data.Commit.FilesChanged,
				Insertions:   data.Commit.Insertions,
				Deletions:    data.Commit.Deletions,
				CommittedAt:  data.Commit.CommittedAt,
			},
		})
		_ = s.eventBus.Publish(ctx, events.BuildGitWSEventSubject(data.SessionID), event)
	}
}

// handleGitCommitsReset handles git reset events by forwarding them to the frontend.
// In the live model, no DB cleanup is needed - the frontend queries agentctl directly.
func (s *Service) handleGitCommitsReset(ctx context.Context, data watcher.GitEventData) {
	if data.Reset == nil {
		s.logger.Debug("missing reset data for git reset event",
			zap.String("task_id", data.TaskID))
		return
	}

	s.logger.Debug("handling git commits reset",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("previous_head", data.Reset.PreviousHead),
		zap.String("current_head", data.Reset.CurrentHead))

	// Forward commits_reset event to WebSocket subject for frontend real-time updates
	if s.eventBus != nil {
		event := bus.NewEvent(events.GitEvent, "orchestrator", &lifecycle.GitEventPayload{
			Type:      lifecycle.GitEventTypeCommitsReset,
			SessionID: data.SessionID,
			TaskID:    data.TaskID,
			Timestamp: time.Now().Format("2006-01-02T15:04:05.000000000Z07:00"),
			Reset: &lifecycle.GitResetData{
				PreviousHead: data.Reset.PreviousHead,
				CurrentHead:  data.Reset.CurrentHead,
			},
		})
		_ = s.eventBus.Publish(ctx, events.BuildGitWSEventSubject(data.SessionID), event)
	}
}
