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

	go s.persistGitSnapshot(data.SessionID, data.TaskID, snapshot)

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

func (s *Service) persistGitSnapshot(sessionID, taskID string, snapshot *models.GitSnapshot) {
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
		return
	}

	s.logger.Debug("created git snapshot",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("snapshot_id", snapshot.ID))

	if s.eventBus == nil {
		return
	}

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

	// Compare file paths, staged status, line counts, and diff content
	for path, newFileData := range new.Files {
		existingFileData, exists := existing.Files[path]
		if !exists {
			return false
		}

		// Compare file details - extract from interface{}
		newInfo := extractFileInfo(newFileData)
		existingInfo := extractFileInfo(existingFileData)

		if newInfo.staged != existingInfo.staged ||
			newInfo.additions != existingInfo.additions ||
			newInfo.deletions != existingInfo.deletions ||
			newInfo.diff != existingInfo.diff {
			return false
		}
	}

	return true
}

// fileInfoCompare holds extracted file info fields for comparison
type fileInfoCompare struct {
	staged    bool
	additions int
	deletions int
	diff      string
}

// extractFileInfo extracts comparable fields from a file info interface
func extractFileInfo(fileData interface{}) fileInfoCompare {
	if fileData == nil {
		return fileInfoCompare{}
	}
	fileMap, ok := fileData.(map[string]interface{})
	if !ok {
		return fileInfoCompare{}
	}
	return extractFileInfoFromMap(fileMap)
}

// extractFileInfoFromMap populates a fileInfoCompare from a string-keyed map.
func extractFileInfoFromMap(fileMap map[string]interface{}) fileInfoCompare {
	info := fileInfoCompare{}
	if staged, ok := fileMap["staged"].(bool); ok {
		info.staged = staged
	}
	// Handle both int and float64 (JSON numbers are float64)
	if additions, ok := fileMap["additions"].(float64); ok {
		info.additions = int(additions)
	} else if additions, ok := fileMap["additions"].(int); ok {
		info.additions = additions
	}
	if deletions, ok := fileMap["deletions"].(float64); ok {
		info.deletions = int(deletions)
	} else if deletions, ok := fileMap["deletions"].(int); ok {
		info.deletions = deletions
	}
	if diff, ok := fileMap["diff"].(string); ok {
		info.diff = diff
	}
	return info
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
	go s.pruneOrphanedCommits(sessionID, taskID, previousHead, currentHead)
}

func (s *Service) pruneOrphanedCommits(sessionID, taskID, previousHead, currentHead string) {
	bgCtx := context.Background()

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
	for cur := currentHead; cur != ""; {
		reachable[cur] = true
		if c, exists := commitBySHA[cur]; exists {
			cur = c.ParentSHA
		} else {
			break
		}
	}

	// Delete commits that are not reachable from currentHead
	var deleted int
	for _, c := range commits {
		if reachable[c.CommitSHA] {
			continue
		}
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

	if deleted == 0 || s.eventBus == nil {
		return
	}

	s.logger.Info("removed orphaned commits after git reset",
		zap.String("session_id", sessionID),
		zap.Int("deleted_count", deleted),
		zap.String("new_head", currentHead))

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
