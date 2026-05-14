package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
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

	// Safety net: mark any tool calls still in a non-terminal state as "complete"
	if affected, err := s.turns.CompletePendingToolCallsForTurn(ctx, turnID); err != nil {
		s.logger.Warn("failed to complete pending tool calls for turn", zap.String("turn_id", turnID), zap.Error(err))
	} else if affected > 0 {
		s.logger.Info("completed stale pending tool calls on turn end",
			zap.String("turn_id", turnID),
			zap.Int64("affected", affected))
	}

	// Fetch the completed turn to get the completed_at timestamp
	turn, err := s.turns.GetTurn(ctx, turnID)
	if err != nil {
		s.logger.Debug("failed to refetch completed turn", zap.String("turn_id", turnID), zap.Error(err))
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
// Returns (nil, nil) when no turn is active. Other errors (DB failures) are returned as-is.
func (s *Service) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	turn, err := s.turns.GetActiveTurnBySessionID(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return turn, err
}

// AbandonOpenTurns closes any open turns for a session by setting their
// completed_at = started_at. Used on session resume to bury orphan turns left
// behind by a previous crash/restart so a fresh prompt starts a fresh turn
// (preventing the UI from showing the orphan's stale started_at as the running
// timer, and preventing the orphan from poisoning analytics with hours of dead
// time).
//
// Mirrors the iteration shape of orchestrator.completeTurnForSession: the DB is
// authoritative, so we loop GetActiveTurn → AbandonTurn until none remain, with
// a sanity cap to break runaway loops.
func (s *Service) AbandonOpenTurns(ctx context.Context, sessionID string) error {
	const maxIterations = 16
	closed := 0
	for closed < maxIterations {
		turn, err := s.GetActiveTurn(ctx, sessionID)
		if err != nil {
			return err
		}
		if turn == nil {
			return nil
		}
		if err := s.turns.AbandonTurn(ctx, turn.ID); err != nil {
			s.logger.Error("failed to abandon turn",
				zap.String("turn_id", turn.ID),
				zap.String("session_id", sessionID),
				zap.Error(err))
			return err
		}
		// Same safety net as CompleteTurn: any tool calls left in a non-terminal
		// state belong to a turn that is no longer running, so flip them to
		// "complete" instead of leaving them to spin forever in the UI.
		if affected, err := s.turns.CompletePendingToolCallsForTurn(ctx, turn.ID); err != nil {
			s.logger.Warn("failed to complete pending tool calls for abandoned turn",
				zap.String("turn_id", turn.ID),
				zap.Error(err))
		} else if affected > 0 {
			s.logger.Info("completed pending tool calls on abandoned turn",
				zap.String("turn_id", turn.ID),
				zap.Int64("affected", affected))
		}
		// Re-fetch so the published event carries the persisted completed_at
		// (= started_at) rather than a synthesized one.
		refreshed, err := s.turns.GetTurn(ctx, turn.ID)
		if err != nil {
			s.logger.Debug("failed to refetch abandoned turn",
				zap.String("turn_id", turn.ID),
				zap.Error(err))
		} else {
			s.publishTurnEvent(events.TurnCompleted, refreshed)
		}
		s.logger.Info("abandoned orphan turn on session resume",
			zap.String("turn_id", turn.ID),
			zap.String("session_id", sessionID))
		closed++
	}
	// Only warn if turns are *still* accumulating after the cap. Closing
	// exactly maxIterations turns and then finding the session clean is not a
	// runaway — same shape as completeTurnForSession.
	if turn, err := s.GetActiveTurn(ctx, sessionID); err == nil && turn != nil {
		s.logger.Warn("AbandonOpenTurns hit iteration cap; some orphan turns may remain",
			zap.String("session_id", sessionID),
			zap.Int("closed", closed),
			zap.Int("max_iterations", maxIterations))
	}
	return nil
}

// getOrStartTurn returns the active turn for a session, or starts a new one if none exists.
// This is used to ensure messages always have a valid turn ID.
func (s *Service) getOrStartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	// Route through GetActiveTurn so the ErrNoRows → (nil, nil) normalization
	// applies consistently. A real DB read failure is logged before falling
	// through to StartTurn — same observability contract as the orchestrator's
	// startTurnForSession adoption path.
	turn, err := s.GetActiveTurn(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to look up active turn; will create a new one",
			zap.String("session_id", sessionID),
			zap.Error(err))
	} else if turn != nil {
		return turn, nil
	}

	// No active turn (or read failed), start a new one
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

	// Get workspace path from the session's worktree(s).
	// Multi-repo: every per-repo worktree sits as a sibling under the task root
	// (~/.kandev/tasks/{taskDirName}/{repoName}/), so the workspace path agentctl
	// needs is the parent of any one of them. Picking the first repo's path here
	// would point agentctl at a single repo subdir and disable the per-repo
	// tracker fan-out that scanRepositorySubdirs relies on.
	var workspacePath string
	if len(session.Worktrees) > 1 {
		workspacePath = filepath.Dir(session.Worktrees[0].WorktreePath)
	} else if len(session.Worktrees) == 1 {
		workspacePath = session.Worktrees[0].WorktreePath
	}

	// If no worktree, try to get from repository snapshot
	if workspacePath == "" && session.RepositorySnapshot != nil {
		if path, ok := session.RepositorySnapshot["path"].(string); ok {
			workspacePath = path
		}
	}

	// Get agent name (registry slug) from profile snapshot.
	// Prefer "agent_name" (the slug used as registry key) over "agent_id" (the database UUID).
	var agentID string
	if session.AgentProfileSnapshot != nil {
		if name, ok := session.AgentProfileSnapshot["agent_name"].(string); ok {
			agentID = name
		} else if id, ok := session.AgentProfileSnapshot["agent_id"].(string); ok {
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

	info := &lifecycle.WorkspaceInfo{
		TaskID:            taskID,
		SessionID:         sessionID,
		TaskEnvironmentID: session.TaskEnvironmentID,
		WorkspacePath:     workspacePath,
		AgentProfileID:    session.AgentProfileID,
		AgentID:           agentID,
		ACPSessionID:      acpSessionID,
	}

	var taskEnv *models.TaskEnvironment
	if session.TaskEnvironmentID != "" {
		env, envErr := s.taskEnvironments.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
		if envErr != nil {
			s.logger.Warn("failed to get task environment for session",
				zap.String("session_id", sessionID),
				zap.String("task_environment_id", session.TaskEnvironmentID),
				zap.Error(envErr))
		} else {
			taskEnv = env
		}
	}
	if taskEnv == nil && taskID != "" {
		env, envErr := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
		if envErr != nil {
			s.logger.Warn("failed to get task environment by task",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(envErr))
		} else {
			taskEnv = env
		}
	}
	if taskEnv != nil {
		applyTaskEnvironmentToWorkspaceInfo(info, taskEnv)
	}

	// Populate executor info for correct runtime selection and remote reconnection
	running, err := s.executors.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		if isExecutorRunningNotFoundError(err) {
			s.logger.Debug("executor running not ready for session",
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else {
			s.logger.Warn("failed to get executor running for session",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	} else if running != nil {
		info.RuntimeName = running.Runtime
		info.AgentExecutionID = running.AgentExecutionID
		mergePersistentWorkspaceMetadata(info, running.Metadata)
		if running.ContainerID != "" {
			ensureWorkspaceMetadata(info)[lifecycle.MetadataKeyContainerID] = running.ContainerID
		}
	}
	if session.ExecutorID != "" {
		exec, err := s.executors.GetExecutor(ctx, session.ExecutorID)
		if err != nil {
			s.logger.Warn("failed to get executor for session",
				zap.String("session_id", sessionID),
				zap.String("executor_id", session.ExecutorID),
				zap.Error(err))
		} else if exec != nil {
			info.ExecutorType = string(exec.Type)
		}
	}

	return info, nil
}

func applyTaskEnvironmentToWorkspaceInfo(info *lifecycle.WorkspaceInfo, env *models.TaskEnvironment) {
	// Always align info.TaskEnvironmentID with the env we resolved against.
	// When session.TaskEnvironmentID points to a stale/missing env, the
	// caller falls back to GetTaskEnvironmentByTaskID and ends up with a
	// different env.ID. Previously we only updated info.TaskEnvironmentID
	// when it was empty, so the metadata + path here would come from env
	// while the ID still pointed at the stale row — a mismatch downstream
	// reconcilers and progress events would key off the wrong env.
	info.TaskEnvironmentID = env.ID
	if info.WorkspacePath == "" {
		info.WorkspacePath = env.WorkspacePath
	}
	if env.ContainerID != "" {
		ensureWorkspaceMetadata(info)[lifecycle.MetadataKeyContainerID] = env.ContainerID
	}
	if env.SandboxID != "" {
		ensureWorkspaceMetadata(info)["sprite_name"] = env.SandboxID
	}
}

func mergePersistentWorkspaceMetadata(info *lifecycle.WorkspaceInfo, metadata map[string]interface{}) {
	filtered := lifecycle.FilterPersistentMetadata(metadata)
	if filtered == nil {
		return
	}
	dst := ensureWorkspaceMetadata(info)
	for k, v := range filtered {
		dst[k] = v
	}
}

func ensureWorkspaceMetadata(info *lifecycle.WorkspaceInfo) map[string]interface{} {
	if info.Metadata == nil {
		info.Metadata = make(map[string]interface{})
	}
	return info.Metadata
}

// GetWorkspaceInfoForEnvironment returns workspace information for a task environment.
func (s *Service) GetWorkspaceInfoForEnvironment(ctx context.Context, taskEnvironmentID string) (*lifecycle.WorkspaceInfo, error) {
	if taskEnvironmentID == "" {
		return nil, fmt.Errorf("task_environment_id is required")
	}
	env, err := s.taskEnvironments.GetTaskEnvironment(ctx, taskEnvironmentID)
	if err != nil {
		return nil, err
	}
	if env == nil {
		return nil, fmt.Errorf("task environment %s not found", taskEnvironmentID)
	}
	sessions, err := s.sessions.ListTaskSessions(ctx, env.TaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions for task environment %s: %w", taskEnvironmentID, err)
	}
	matching := make([]*models.TaskSession, 0, len(sessions))
	for _, session := range sessions {
		if session.TaskEnvironmentID == taskEnvironmentID {
			matching = append(matching, session)
		}
	}
	if len(matching) > 0 {
		sort.SliceStable(matching, func(i, j int) bool {
			if !matching[i].StartedAt.Equal(matching[j].StartedAt) {
				return matching[i].StartedAt.After(matching[j].StartedAt)
			}
			if !matching[i].UpdatedAt.Equal(matching[j].UpdatedAt) {
				return matching[i].UpdatedAt.After(matching[j].UpdatedAt)
			}
			return matching[i].ID > matching[j].ID
		})
		return s.GetWorkspaceInfoForSession(ctx, env.TaskID, matching[0].ID)
	}
	return nil, fmt.Errorf("task environment %s has no linked task session", taskEnvironmentID)
}

func isExecutorRunningNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, models.ErrExecutorRunningNotFound)
}
