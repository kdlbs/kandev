package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// Turn operations

// StartTurn creates a new turn for a session and publishes the turn.started event.
// Returns the created turn.
func (s *Service) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	turn := &models.Turn{
		ID:            uuid.New().String(),
		TaskSessionID: sessionID,
		TaskID:        session.TaskID,
		StartedAt:     time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	if err := s.turns.CreateTurn(ctx, turn); err != nil {
		s.logger.Error("failed to create turn", zap.Error(err))
		return nil, err
	}

	s.publishTurnEvent(events.TurnStarted, turn)

	s.logger.Debug("started turn",
		zap.String("turn_id", turn.ID),
		zap.String("session_id", sessionID),
		zap.String("task_id", turn.TaskID))

	return turn, nil
}

// CompleteTurn marks a turn as completed and publishes the turn.completed event.
func (s *Service) CompleteTurn(ctx context.Context, turnID string) error {
	if turnID == "" {
		return nil // No active turn to complete
	}

	if err := s.turns.CompleteTurn(ctx, turnID); err != nil {
		s.logger.Error("failed to complete turn", zap.String("turn_id", turnID), zap.Error(err))
		return err
	}

	// Safety net: mark any tool calls still in "running" state as "complete"
	if affected, err := s.turns.CompleteRunningToolCallsForTurn(ctx, turnID); err != nil {
		s.logger.Warn("failed to complete running tool calls for turn", zap.String("turn_id", turnID), zap.Error(err))
	} else if affected > 0 {
		s.logger.Info("completed stale running tool calls on turn end",
			zap.String("turn_id", turnID),
			zap.Int64("affected", affected))
	}

	// Fetch the completed turn to get the completed_at timestamp
	turn, err := s.turns.GetTurn(ctx, turnID)
	if err != nil {
		s.logger.Warn("failed to refetch completed turn", zap.String("turn_id", turnID), zap.Error(err))
		// Turn was likely deleted (task deletion race), skip publishing
		return nil
	}

	s.publishTurnEvent(events.TurnCompleted, turn)

	s.logger.Debug("completed turn",
		zap.String("turn_id", turnID),
		zap.String("session_id", turn.TaskSessionID),
		zap.String("task_id", turn.TaskID))

	return nil
}

// GetActiveTurn returns the currently active (non-completed) turn for a session.
func (s *Service) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	return s.turns.GetActiveTurnBySessionID(ctx, sessionID)
}

// getOrStartTurn returns the active turn for a session, or starts a new one if none exists.
// This is used to ensure messages always have a valid turn ID.
func (s *Service) getOrStartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	// First try to get an active turn
	turn, err := s.turns.GetActiveTurnBySessionID(ctx, sessionID)
	if err == nil && turn != nil {
		return turn, nil
	}

	// No active turn, start a new one
	return s.StartTurn(ctx, sessionID)
}

// publishTurnEvent publishes a turn event to the event bus.
func (s *Service) publishTurnEvent(eventType string, turn *models.Turn) {
	if s.eventBus == nil {
		return
	}
	if turn == nil {
		s.logger.Warn("publishTurnEvent: turn is nil, skipping", zap.String("event_type", eventType))
		return
	}
	_ = s.eventBus.Publish(context.Background(), eventType, bus.NewEvent(eventType, "task-service", map[string]interface{}{
		"id":           turn.ID,
		"session_id":   turn.TaskSessionID,
		"task_id":      turn.TaskID,
		"started_at":   turn.StartedAt,
		"completed_at": turn.CompletedAt,
		"metadata":     turn.Metadata,
		"created_at":   turn.CreatedAt,
		"updated_at":   turn.UpdatedAt,
	}))
}

// GetGitSnapshots retrieves git snapshots for a session
func (s *Service) GetGitSnapshots(ctx context.Context, sessionID string, limit int) ([]*models.GitSnapshot, error) {
	return s.gitSnapshots.GetGitSnapshotsBySession(ctx, sessionID, limit)
}

// GetLatestGitSnapshot retrieves the latest git snapshot for a session
func (s *Service) GetLatestGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error) {
	return s.gitSnapshots.GetLatestGitSnapshot(ctx, sessionID)
}

// GetFirstGitSnapshot retrieves the first git snapshot for a session (oldest)
func (s *Service) GetFirstGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error) {
	return s.gitSnapshots.GetFirstGitSnapshot(ctx, sessionID)
}

// GetSessionCommits retrieves commits for a session
func (s *Service) GetSessionCommits(ctx context.Context, sessionID string) ([]*models.SessionCommit, error) {
	return s.gitSnapshots.GetSessionCommits(ctx, sessionID)
}

// GetLatestSessionCommit retrieves the latest commit for a session
func (s *Service) GetLatestSessionCommit(ctx context.Context, sessionID string) (*models.SessionCommit, error) {
	return s.gitSnapshots.GetLatestSessionCommit(ctx, sessionID)
}

// GetCumulativeDiff computes the cumulative diff from base commit to current HEAD
// by using the first snapshot's base_commit and the latest snapshot's files
func (s *Service) GetCumulativeDiff(ctx context.Context, sessionID string) (*models.CumulativeDiff, error) {
	// Get the first snapshot to find the base commit
	firstSnapshot, err := s.gitSnapshots.GetFirstGitSnapshot(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // No snapshots yet — valid state for fresh tasks
		}
		return nil, fmt.Errorf("failed to get first git snapshot: %w", err)
	}

	// Get the latest snapshot for current state
	latestSnapshot, err := s.gitSnapshots.GetLatestGitSnapshot(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest git snapshot: %w", err)
	}

	// Count total commits for this session
	commits, err := s.gitSnapshots.GetSessionCommits(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session commits: %w", err)
	}

	return &models.CumulativeDiff{
		SessionID:    sessionID,
		BaseCommit:   firstSnapshot.BaseCommit,
		HeadCommit:   latestSnapshot.HeadCommit,
		TotalCommits: len(commits),
		Files:        latestSnapshot.Files,
	}, nil
}

// GetWorkspaceInfoForSession returns workspace information for a task session.
// This implements the lifecycle.WorkspaceInfoProvider interface.
// The taskID parameter is optional - if empty, it will be looked up from the session.
func (s *Service) GetWorkspaceInfoForSession(ctx context.Context, taskID, sessionID string) (*lifecycle.WorkspaceInfo, error) {
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session %s: %w", sessionID, err)
	}
	if session == nil {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	// Use session's TaskID if not provided
	if taskID == "" {
		taskID = session.TaskID
	}

	// Get workspace path from the session's worktree
	var workspacePath string
	if len(session.Worktrees) > 0 {
		workspacePath = session.Worktrees[0].WorktreePath
	}

	// If no worktree, try to get from repository snapshot
	if workspacePath == "" && session.RepositorySnapshot != nil {
		if path, ok := session.RepositorySnapshot["path"].(string); ok {
			workspacePath = path
		}
	}

	// Get agent ID from profile snapshot
	var agentID string
	if session.AgentProfileSnapshot != nil {
		if id, ok := session.AgentProfileSnapshot["agent_id"].(string); ok {
			agentID = id
		}
	}

	// Get ACP session ID from metadata
	var acpSessionID string
	if session.Metadata != nil {
		if id, ok := session.Metadata["acp_session_id"].(string); ok {
			acpSessionID = id
		}
	}

	return &lifecycle.WorkspaceInfo{
		TaskID:         taskID,
		SessionID:      sessionID,
		WorkspacePath:  workspacePath,
		AgentProfileID: session.AgentProfileID,
		AgentID:        agentID,
		ACPSessionID:   acpSessionID,
	}, nil
}
