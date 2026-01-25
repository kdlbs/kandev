// Package orchestrator provides event handler methods for the orchestrator service.
package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Event handlers

func (s *Service) handleACPSessionCreated(ctx context.Context, data watcher.ACPSessionEventData) {
	if data.SessionID == "" || data.ACPSessionID == "" {
		return
	}
	s.storeResumeToken(ctx, data.TaskID, data.SessionID, data.ACPSessionID)
}

// storeResumeToken stores an agent's session ID as the resume token for session recovery.
// This is called both from handleACPSessionCreated (for ACP-based agents) and from
// handleAgentStreamEvent (for stream-based agents like Claude Code).
func (s *Service) storeResumeToken(ctx context.Context, taskID, sessionID, acpSessionID string) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to load task session for resume token storage",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	resumable := true
	if session.ExecutorID != "" {
		if executor, err := s.repo.GetExecutor(ctx, session.ExecutorID); err == nil && executor != nil {
			resumable = executor.Resumable
		}
	}

	running := &models.ExecutorRunning{
		ID:               session.ID,
		SessionID:        session.ID,
		TaskID:           session.TaskID,
		ExecutorID:       session.ExecutorID,
		Status:           "ready",
		Resumable:        resumable,
		ResumeToken:      acpSessionID,
		AgentExecutionID: session.AgentExecutionID,
		ContainerID:      session.ContainerID,
	}
	if len(session.Worktrees) > 0 {
		running.WorktreeID = session.Worktrees[0].WorktreeID
		running.WorktreePath = session.Worktrees[0].WorktreePath
		running.WorktreeBranch = session.Worktrees[0].WorktreeBranch
	}

	if err := s.repo.UpsertExecutorRunning(ctx, running); err != nil {
		s.logger.Warn("failed to persist resume token for session",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	s.logger.Debug("stored resume token for session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("resume_token", acpSessionID))
}

// handleAgentCompleted handles agent completion events
func (s *Service) handleAgentCompleted(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Info("handling agent completed",
		zap.String("task_id", data.TaskID),
		zap.String("agent_execution_id", data.AgentExecutionID))

	// Update scheduler and remove from queue
	s.scheduler.HandleTaskCompleted(data.TaskID, true)
	s.scheduler.RemoveTask(data.TaskID)

	// Move task to REVIEW state for user review
	// The user can then send a follow-up (moves back to IN_PROGRESS) or mark as COMPLETED
	if err := s.taskRepo.UpdateTaskState(ctx, data.TaskID, v1.TaskStateReview); err != nil {
		s.logger.Error("failed to update task state to REVIEW",
			zap.String("task_id", data.TaskID),
			zap.Error(err))
	} else {
		s.logger.Info("task moved to REVIEW state after agent completion",
			zap.String("task_id", data.TaskID))
	}
}

// handleAgentFailed handles agent failure events
func (s *Service) handleAgentFailed(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Warn("handling agent failed",
		zap.String("task_id", data.TaskID),
		zap.String("agent_execution_id", data.AgentExecutionID),
		zap.String("error_message", data.ErrorMessage))

	// Trigger retry logic
	s.scheduler.HandleTaskCompleted(data.TaskID, false)
	if !s.scheduler.RetryTask(data.TaskID) {
		s.logger.Error("task failed and retry limit exceeded",
			zap.String("task_id", data.TaskID))

		// Move task to REVIEW state even on failure - user can decide to retry or close
		// This maintains the review cycle: user reviews the failure and decides next steps
		if err := s.taskRepo.UpdateTaskState(ctx, data.TaskID, v1.TaskStateReview); err != nil {
			s.logger.Error("failed to update task state to REVIEW after failure",
				zap.String("task_id", data.TaskID),
				zap.Error(err))
		} else {
			s.logger.Info("task moved to REVIEW state after failure (for user review)",
				zap.String("task_id", data.TaskID))
		}
	}
}

// handleAgentStreamEvent handles agent stream events (tool calls, message chunks, etc.)
func (s *Service) handleAgentStreamEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload == nil || payload.Data == nil {
		return
	}

	taskID := payload.TaskID
	sessionID := payload.SessionID
	eventType := payload.Data.Type

	s.logger.Debug("handling agent stream event",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("event_type", eventType))

	// Handle different event types
	switch eventType {
	case "message_streaming":
		s.handleMessageStreamingEvent(ctx, payload)

	case "tool_call":
		s.saveAgentTextIfPresent(ctx, payload)
		s.handleToolCallEvent(ctx, payload)

	case "tool_update":
		s.handleToolUpdateEvent(ctx, payload)

	case "complete":
		s.saveAgentTextIfPresent(ctx, payload)
		s.completeTurnForSession(ctx, sessionID)
		s.setSessionWaitingForInput(ctx, taskID, sessionID)

	case "error":
		// Handle error events
		// Note: Agent-specific error parsing is done in the adapter layer.
		// The adapter provides a user-friendly message in the Text field,
		// with raw details in the Data field for debugging.
		if sessionID != "" && s.messageCreator != nil {
			// Build error message - adapters should provide parsed message in Text field
			errorMsg := payload.Data.Error
			if errorMsg == "" {
				errorMsg = payload.Data.Text
			}
			if errorMsg == "" {
				errorMsg = "An error occurred while processing your request"
			}

			// Build metadata with all available error context
			metadata := map[string]interface{}{
				"provider":       "agent",
				"provider_agent": payload.AgentID,
			}
			if payload.Data.Data != nil {
				metadata["error_data"] = payload.Data.Data
			}

			if err := s.messageCreator.CreateSessionMessage(
				ctx,
				taskID,
				errorMsg,
				sessionID,
				string(v1.MessageTypeError),
				s.getActiveTurnID(sessionID),
				metadata,
				false,
			); err != nil {
				s.logger.Error("failed to create error message",
					zap.String("task_id", taskID),
					zap.Error(err))
			}
		}
		// Complete the turn since the agent errored
		s.completeTurnForSession(ctx, sessionID)

	case "session_status":
		// Handle session status events (resumed vs new session)
		// Also store the agent's session ID as resume token for session recovery
		if sessionID != "" && payload.Data.ACPSessionID != "" {
			s.storeResumeToken(ctx, taskID, sessionID, payload.Data.ACPSessionID)
		}

		if sessionID != "" && s.messageCreator != nil {
			var statusMsg string
			if payload.Data.SessionStatus == "resumed" {
				statusMsg = "Session resumed"
			} else {
				statusMsg = "New session started"
			}
			if err := s.messageCreator.CreateSessionMessage(
				ctx,
				taskID,
				statusMsg,
				sessionID,
				string(v1.MessageTypeStatus),
				s.getActiveTurnID(sessionID),
				nil,
				false,
			); err != nil {
				s.logger.Error("failed to create session status message",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID),
					zap.Error(err))
			}
		}
	}
}

// handleToolCallEvent handles tool_call events and creates messages
func (s *Service) handleToolCallEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.SessionID == "" {
		s.logger.Warn("missing session_id for tool_call",
			zap.String("task_id", payload.TaskID),
			zap.String("tool_call_id", payload.Data.ToolCallID))
		return
	}

	if s.messageCreator != nil {
		if err := s.messageCreator.CreateToolCallMessage(
			ctx,
			payload.TaskID,
			payload.Data.ToolCallID,
			payload.Data.ToolTitle,
			payload.Data.ToolStatus,
			payload.SessionID,
			s.getActiveTurnID(payload.SessionID),
			payload.Data.ToolArgs,
		); err != nil {
			s.logger.Error("failed to create tool call message",
				zap.String("task_id", payload.TaskID),
				zap.String("tool_call_id", payload.Data.ToolCallID),
				zap.Error(err))
		} else {
			s.logger.Debug("created tool call message",
				zap.String("task_id", payload.TaskID),
				zap.String("tool_call_id", payload.Data.ToolCallID))
		}

		s.updateTaskSessionState(ctx, payload.TaskID, payload.SessionID, models.TaskSessionStateRunning, "", false)
	}
}

// saveAgentTextIfPresent saves any accumulated agent text as an agent message
func (s *Service) saveAgentTextIfPresent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.Data.Text == "" || payload.SessionID == "" {
		return
	}

	if s.messageCreator != nil {
		if err := s.messageCreator.CreateAgentMessage(ctx, payload.TaskID, payload.Data.Text, payload.SessionID, s.getActiveTurnID(payload.SessionID)); err != nil {
			s.logger.Error("failed to create agent message",
				zap.String("task_id", payload.TaskID),
				zap.Error(err))
		} else {
			s.logger.Debug("created agent message",
				zap.String("task_id", payload.TaskID),
				zap.Int("message_length", len(payload.Data.Text)))
		}
	}
}

// handleMessageStreamingEvent handles streaming message events for real-time text updates.
// It creates a new message on first chunk (IsAppend=false) or appends to existing (IsAppend=true).
func (s *Service) handleMessageStreamingEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.Data.Text == "" || payload.SessionID == "" {
		return
	}

	if s.messageCreator == nil {
		return
	}

	// MessageID is pre-generated by the manager to avoid race conditions
	messageID := payload.Data.MessageID
	if messageID == "" {
		s.logger.Warn("streaming message event missing message ID",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID))
		return
	}

	if payload.Data.IsAppend {
		// Append to existing message
		if err := s.messageCreator.AppendAgentMessage(ctx, messageID, payload.Data.Text); err != nil {
			s.logger.Error("failed to append to streaming message",
				zap.String("task_id", payload.TaskID),
				zap.String("message_id", messageID),
				zap.Error(err))
		} else {
			s.logger.Debug("appended to streaming message",
				zap.String("task_id", payload.TaskID),
				zap.String("message_id", messageID),
				zap.Int("content_length", len(payload.Data.Text)))
		}
	} else {
		// Create new streaming message with the pre-generated ID
		if err := s.messageCreator.CreateAgentMessageStreaming(ctx, messageID, payload.TaskID, payload.Data.Text, payload.SessionID, s.getActiveTurnID(payload.SessionID)); err != nil {
			s.logger.Error("failed to create streaming message",
				zap.String("task_id", payload.TaskID),
				zap.String("message_id", messageID),
				zap.Error(err))
		} else {
			s.logger.Debug("created streaming message",
				zap.String("task_id", payload.TaskID),
				zap.String("session_id", payload.SessionID),
				zap.String("message_id", messageID),
				zap.Int("content_length", len(payload.Data.Text)))
		}
	}
}

// handleToolUpdateEvent handles tool_update events and updates messages
func (s *Service) handleToolUpdateEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.SessionID == "" {
		s.logger.Warn("missing session_id for tool_update",
			zap.String("task_id", payload.TaskID),
			zap.String("tool_call_id", payload.Data.ToolCallID))
		return
	}

	if s.messageCreator == nil {
		return
	}

	// Handle all status updates (running, complete, error)
	switch payload.Data.ToolStatus {
	case "running", "complete", "completed", "success", "error", "failed":
		result := ""
		if payload.Data.ToolResult != nil {
			if str, ok := payload.Data.ToolResult.(string); ok {
				result = str
			}
		}
		if err := s.messageCreator.UpdateToolCallMessage(
			ctx,
			payload.TaskID,
			payload.Data.ToolCallID,
			payload.Data.ToolStatus,
			result,
			payload.SessionID,
			payload.Data.ToolTitle, // Include title from update event
			payload.Data.ToolArgs,  // Include args from update event
		); err != nil {
			s.logger.Warn("failed to update tool call message",
				zap.String("task_id", payload.TaskID),
				zap.String("tool_call_id", payload.Data.ToolCallID),
				zap.Error(err))
		}

		// Update session state for completion events
		if payload.Data.ToolStatus == "complete" || payload.Data.ToolStatus == "completed" ||
			payload.Data.ToolStatus == "success" || payload.Data.ToolStatus == "error" || payload.Data.ToolStatus == "failed" {
			s.updateTaskSessionState(ctx, payload.TaskID, payload.SessionID, models.TaskSessionStateRunning, "", false)
		}
	}
}

func (s *Service) updateTaskSessionState(ctx context.Context, taskID, sessionID string, nextState models.TaskSessionState, errorMessage string, allowWakeFromWaiting bool) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return
	}
	if session.State == models.TaskSessionStateWaitingForInput && nextState == models.TaskSessionStateRunning && !allowWakeFromWaiting {
		return
	}
	oldState := session.State
	switch session.State {
	case models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		return
	}
	if session.State == nextState {
		return
	}
	if err := s.repo.UpdateTaskSessionState(ctx, sessionID, nextState, errorMessage); err != nil {
		s.logger.Error("failed to update task session state",
			zap.String("session_id", sessionID),
			zap.String("state", string(nextState)),
			zap.Error(err))
	}
	s.logger.Debug("task session state updated",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("old_state", string(oldState)),
		zap.String("new_state", string(nextState)))
	if s.eventBus != nil {
		_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(events.TaskSessionStateChanged, "task-session", map[string]interface{}{
			"task_id":                taskID,
			"session_id":             sessionID,
			"old_state":              string(oldState),
			"new_state":              string(nextState),
			"agent_profile_id":       session.AgentProfileID,
			"agent_profile_snapshot": session.AgentProfileSnapshot,
		}))
	}
}

func (s *Service) setSessionWaitingForInput(ctx context.Context, taskID, sessionID string) {
	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateWaitingForInput, "", false)

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateReview); err != nil {
		s.logger.Error("failed to update task state to REVIEW",
			zap.String("task_id", taskID),
			zap.Error(err))
	} else {
		s.logger.Info("task moved to REVIEW state",
			zap.String("task_id", taskID))
	}
}

func (s *Service) setSessionRunning(ctx context.Context, taskID, sessionID string) {
	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateRunning, "", true)

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateInProgress); err != nil {
		s.logger.Error("failed to update task state to IN_PROGRESS",
			zap.String("task_id", taskID),
			zap.Error(err))
	} else {
		s.logger.Info("task moved to IN_PROGRESS state",
			zap.String("task_id", taskID))
	}
}

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

// handleGitStatusUpdate handles git status updates by creating git snapshots
func (s *Service) handleGitStatusUpdate(ctx context.Context, data watcher.GitEventData) {
	if data.Status == nil {
		s.logger.Debug("missing status data for git status update",
			zap.String("task_id", data.TaskID))
		return
	}

	// Forward status_update event to WebSocket subject for frontend
	// Since data is already lifecycle.GitEventPayload, we can forward it directly
	if s.eventBus != nil {
		event := bus.NewEvent(events.GitWSEvent, "orchestrator", &data)
		_ = s.eventBus.Publish(ctx, events.BuildGitWSEventSubject(data.SessionID), event)
	}

	// Convert Files from interface{} to map[string]interface{}
	var files map[string]interface{}
	if data.Status.Files != nil {
		if f, ok := data.Status.Files.(map[string]interface{}); ok {
			files = f
		}
	}

	// Create git snapshot instead of storing in session metadata
	snapshot := &models.GitSnapshot{
		SessionID:    data.SessionID,
		SnapshotType: models.SnapshotTypeStatusUpdate,
		Branch:       data.Status.Branch,
		RemoteBranch: data.Status.RemoteBranch,
		HeadCommit:   data.Status.HeadCommit,
		BaseCommit:   data.Status.BaseCommit,
		Ahead:        data.Status.Ahead,
		Behind:       data.Status.Behind,
		Files:        files,
		TriggeredBy:  "git_status_event",
		Metadata: map[string]interface{}{
			"modified":  data.Status.Modified,
			"added":     data.Status.Added,
			"deleted":   data.Status.Deleted,
			"untracked": data.Status.Untracked,
			"renamed":   data.Status.Renamed,
			"timestamp": data.Timestamp,
		},
	}

	sessionID := data.SessionID
	taskID := data.TaskID

	// Persist snapshot to database asynchronously, but skip if duplicate of latest
	go func() {
		bgCtx := context.Background()

		// Check if this is a duplicate of the latest snapshot
		latest, err := s.repo.GetLatestGitSnapshot(bgCtx, sessionID)
		if err == nil && latest != nil {
			// Compare key fields to detect duplicates
			if s.isSnapshotDuplicate(latest, snapshot) {
				s.logger.Debug("skipping duplicate git snapshot",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID))
				return
			}
		}

		if err := s.repo.CreateGitSnapshot(bgCtx, snapshot); err != nil {
			s.logger.Error("failed to create git snapshot",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else {
			s.logger.Debug("created git snapshot",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("snapshot_id", snapshot.ID))

			// Publish event to notify frontend
			if s.eventBus != nil {
				event := bus.NewEvent(events.GitEvent, "orchestrator", &lifecycle.GitEventPayload{
					Type:      lifecycle.GitEventTypeSnapshotCreated,
					SessionID: sessionID,
					TaskID:    taskID,
					Timestamp: snapshot.CreatedAt.Format("2006-01-02T15:04:05.000000000Z07:00"),
					Snapshot: &lifecycle.GitSnapshotData{
						ID:           snapshot.ID,
						SessionID:    snapshot.SessionID,
						SnapshotType: string(snapshot.SnapshotType),
						Branch:       snapshot.Branch,
						RemoteBranch: snapshot.RemoteBranch,
						HeadCommit:   snapshot.HeadCommit,
						BaseCommit:   snapshot.BaseCommit,
						Ahead:        snapshot.Ahead,
						Behind:       snapshot.Behind,
						Files:        snapshot.Files,
						TriggeredBy:  snapshot.TriggeredBy,
						CreatedAt:    snapshot.CreatedAt.Format("2006-01-02T15:04:05.000000000Z07:00"),
					},
				})
				_ = s.eventBus.Publish(bgCtx, events.BuildGitWSEventSubject(sessionID), event)
			}
		}
	}()
}

// isSnapshotDuplicate checks if two snapshots have the same content
func (s *Service) isSnapshotDuplicate(existing, new *models.GitSnapshot) bool {
	// Different snapshot types are never duplicates
	if existing.SnapshotType != new.SnapshotType {
		return false
	}

	// Compare branch and commit info
	if existing.Branch != new.Branch ||
		existing.HeadCommit != new.HeadCommit ||
		existing.Ahead != new.Ahead ||
		existing.Behind != new.Behind {
		return false
	}

	// Compare file counts first (quick check)
	existingFileCount := len(existing.Files)
	newFileCount := len(new.Files)
	if existingFileCount != newFileCount {
		return false
	}

	// Compare file paths (if same count, check if same files)
	for path := range new.Files {
		if _, exists := existing.Files[path]; !exists {
			return false
		}
	}

	return true
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

	session, err := s.repo.GetTaskSession(ctx, data.TaskSessionID)
	if err != nil {
		s.logger.Debug("no task session for context window update",
			zap.String("session_id", data.TaskSessionID),
			zap.Error(err))
		return
	}

	// Update session metadata with context window info
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	contextWindowData := map[string]interface{}{
		"size":       data.ContextWindowSize,
		"used":       data.ContextWindowUsed,
		"remaining":  data.ContextWindowRemaining,
		"efficiency": data.ContextEfficiency,
	}
	session.Metadata["context_window"] = contextWindowData

	// Persist to database asynchronously
	go func() {
		if err := s.repo.UpdateTaskSession(context.Background(), session); err != nil {
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

	// Broadcast session state change with metadata so frontend can update
	// This uses the existing session.state_changed event with metadata included
	if s.eventBus != nil {
		_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
			events.TaskSessionStateChanged,
			"orchestrator",
			map[string]interface{}{
				"task_id":                data.TaskID,
				"session_id":             session.ID,
				"new_state":              string(session.State),
				"agent_profile_id":       session.AgentProfileID,
				"agent_profile_snapshot": session.AgentProfileSnapshot,
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

// handleGitCommitCreated handles git commit events by creating session commit records
func (s *Service) handleGitCommitCreated(ctx context.Context, data watcher.GitEventData) {
	if data.Commit == nil {
		s.logger.Debug("missing commit data for git commit event",
			zap.String("task_id", data.TaskID))
		return
	}

	s.logger.Debug("handling git commit created",
		zap.String("task_id", data.TaskID),
		zap.String("commit_sha", data.Commit.CommitSHA))

	// Parse committed_at timestamp
	var committedAt time.Time
	if data.Commit.CommittedAt != "" {
		if t, err := time.Parse(time.RFC3339, data.Commit.CommittedAt); err == nil {
			committedAt = t
		} else {
			committedAt = time.Now().UTC()
		}
	} else {
		committedAt = time.Now().UTC()
	}

	sessionID := data.SessionID
	taskID := data.TaskID
	commitSHA := data.Commit.CommitSHA

	// Create session commit record
	commit := &models.SessionCommit{
		SessionID:     sessionID,
		CommitSHA:     data.Commit.CommitSHA,
		ParentSHA:     data.Commit.ParentSHA,
		AuthorName:    data.Commit.AuthorName,
		AuthorEmail:   data.Commit.AuthorEmail,
		CommitMessage: data.Commit.Message,
		CommittedAt:   committedAt,
		FilesChanged:  data.Commit.FilesChanged,
		Insertions:    data.Commit.Insertions,
		Deletions:     data.Commit.Deletions,
	}

	// Persist commit record to database asynchronously
	go func() {
		bgCtx := context.Background()
		if err := s.repo.CreateSessionCommit(bgCtx, commit); err != nil {
			s.logger.Error("failed to create session commit record",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("commit_sha", commitSHA),
				zap.Error(err))
		} else {
			s.logger.Debug("created session commit record",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("commit_sha", commitSHA))

			// Publish event to notify frontend using unified format
			if s.eventBus != nil {
				event := bus.NewEvent(events.GitEvent, "orchestrator", &lifecycle.GitEventPayload{
					Type:      lifecycle.GitEventTypeCommitCreated,
					SessionID: sessionID,
					TaskID:    taskID,
					Timestamp: time.Now().Format("2006-01-02T15:04:05.000000000Z07:00"),
					Commit: &lifecycle.GitCommitData{
						ID:           commit.ID,
						CommitSHA:    commit.CommitSHA,
						ParentSHA:    commit.ParentSHA,
						Message:      commit.CommitMessage,
						AuthorName:   commit.AuthorName,
						AuthorEmail:  commit.AuthorEmail,
						FilesChanged: commit.FilesChanged,
						Insertions:   commit.Insertions,
						Deletions:    commit.Deletions,
						CommittedAt:  commit.CommittedAt.Format(time.RFC3339),
						CreatedAt:    commit.CreatedAt.Format("2006-01-02T15:04:05.000000000Z07:00"),
					},
				})
				_ = s.eventBus.Publish(bgCtx, events.BuildGitWSEventSubject(sessionID), event)
			}
		}
	}()
}

// handleGitCommitsReset handles git reset events by removing orphaned commits
func (s *Service) handleGitCommitsReset(ctx context.Context, data watcher.GitEventData) {
	if data.Reset == nil {
		s.logger.Debug("missing reset data for git reset event",
			zap.String("task_id", data.TaskID))
		return
	}

	sessionID := data.SessionID
	taskID := data.TaskID
	previousHead := data.Reset.PreviousHead
	currentHead := data.Reset.CurrentHead

	s.logger.Info("handling git commits reset",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("previous_head", previousHead),
		zap.String("current_head", currentHead))

	// Remove orphaned commits asynchronously
	go func() {
		bgCtx := context.Background()

		// Get all stored commits for this session
		commits, err := s.repo.GetSessionCommits(bgCtx, sessionID)
		if err != nil {
			s.logger.Error("failed to get session commits for reset handling",
				zap.String("session_id", sessionID),
				zap.Error(err))
			return
		}

		if len(commits) == 0 {
			return
		}

		// Build a map for quick lookup
		commitBySHA := make(map[string]*models.SessionCommit)
		for _, c := range commits {
			commitBySHA[c.CommitSHA] = c
		}

		// Walk the parent chain from currentHead to find reachable commits
		reachable := make(map[string]bool)
		current := currentHead
		for current != "" {
			reachable[current] = true
			if c, exists := commitBySHA[current]; exists {
				current = c.ParentSHA
			} else {
				// Parent not in our stored commits, stop walking
				break
			}
		}

		// Delete commits that are not reachable from currentHead
		var deleted int
		for _, c := range commits {
			if !reachable[c.CommitSHA] {
				if err := s.repo.DeleteSessionCommit(bgCtx, c.ID); err != nil {
					s.logger.Error("failed to delete orphaned commit",
						zap.String("session_id", sessionID),
						zap.String("commit_sha", c.CommitSHA),
						zap.Error(err))
				} else {
					deleted++
					s.logger.Debug("deleted orphaned commit after reset",
						zap.String("session_id", sessionID),
						zap.String("commit_sha", c.CommitSHA))
				}
			}
		}

		if deleted > 0 {
			s.logger.Info("removed orphaned commits after git reset",
				zap.String("session_id", sessionID),
				zap.Int("deleted_count", deleted),
				zap.String("new_head", currentHead))

			// Publish event to notify frontend that commits changed
			if s.eventBus != nil {
				event := bus.NewEvent(events.GitEvent, "orchestrator", &lifecycle.GitEventPayload{
					Type:      lifecycle.GitEventTypeCommitsReset,
					SessionID: sessionID,
					TaskID:    taskID,
					Timestamp: time.Now().Format("2006-01-02T15:04:05.000000000Z07:00"),
					Reset: &lifecycle.GitResetData{
						PreviousHead: previousHead,
						CurrentHead:  currentHead,
						DeletedCount: deleted,
					},
				})
				_ = s.eventBus.Publish(bgCtx, events.BuildGitWSEventSubject(sessionID), event)
			}
		}
	}()
}
