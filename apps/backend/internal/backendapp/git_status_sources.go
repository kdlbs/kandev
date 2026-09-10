package backendapp

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type gitStatusSources struct {
	environmentID string
	workspacePath string
	sessionIDs    []string
}

func resolveGitStatusSources(
	ctx context.Context,
	taskRepo *sqliterepo.Repository,
	requested *models.TaskSession,
	log *logger.Logger,
) (*gitStatusSources, bool) {
	if requested == nil || requested.ID == "" {
		return nil, false
	}
	if requested.TaskEnvironmentID == "" {
		log.Debug("cannot resolve task environment for git status",
			zap.String("session_id", requested.ID),
			zap.String("reason", "session_environment_missing"))
		return nil, false
	}
	if taskRepo == nil {
		log.Debug("cannot resolve task environment for git status",
			zap.String("session_id", requested.ID),
			zap.String("reason", "task_repository_unavailable"))
		return nil, false
	}

	env, ok := loadGitStatusEnvironment(ctx, taskRepo, requested, log)
	if !ok {
		return nil, false
	}
	sessions, recordedPaths, ok := loadGitStatusCandidates(ctx, taskRepo, requested, env.ID, log)
	if !ok {
		return nil, false
	}
	sourceIDs := collectGitStatusSourceIDs(sessions, env, recordedPaths, log)
	if len(sourceIDs) == 0 {
		log.Debug("no eligible workspace source for git status",
			zap.String("session_id", requested.ID),
			zap.String("task_environment_id", env.ID),
			zap.String("reason", "no_matching_session"))
		return nil, false
	}

	// Prefer the requested session when it is canonical, then inspect sibling
	// sessions in the repository's stable order.
	ordered := orderGitStatusSourceIDs(sourceIDs, requested.ID)
	log.Debug("resolved task-environment git status sources",
		zap.String("session_id", requested.ID),
		zap.String("task_environment_id", env.ID),
		zap.Int("sources", len(ordered)))
	return &gitStatusSources{
		environmentID: env.ID,
		workspacePath: env.WorkspacePath,
		sessionIDs:    ordered,
	}, true
}

func loadGitStatusEnvironment(
	ctx context.Context,
	taskRepo *sqliterepo.Repository,
	requested *models.TaskSession,
	log *logger.Logger,
) (*models.TaskEnvironment, bool) {
	env, err := taskRepo.GetTaskEnvironment(ctx, requested.TaskEnvironmentID)
	if err != nil {
		log.Debug("cannot resolve task environment for git status",
			zap.String("session_id", requested.ID),
			zap.String("task_environment_id", requested.TaskEnvironmentID),
			zap.String("reason", "environment_lookup_failed"),
			zap.Error(err))
		return nil, false
	}
	if env == nil || env.WorkspacePath == "" {
		log.Debug("cannot resolve canonical workspace for git status",
			zap.String("session_id", requested.ID),
			zap.String("task_environment_id", requested.TaskEnvironmentID),
			zap.String("reason", "canonical_workspace_missing"))
		return nil, false
	}
	return env, true
}

func loadGitStatusCandidates(
	ctx context.Context,
	taskRepo *sqliterepo.Repository,
	requested *models.TaskSession,
	environmentID string,
	log *logger.Logger,
) ([]*models.TaskSession, map[string]string, bool) {
	sessions, err := taskRepo.ListTaskSessionsByTaskEnvironment(ctx, environmentID)
	if err != nil {
		log.Debug("cannot load sibling sessions for git status",
			zap.String("session_id", requested.ID),
			zap.String("task_environment_id", environmentID),
			zap.String("reason", "session_lookup_failed"),
			zap.Error(err))
		return nil, nil, false
	}
	recordedPaths, err := taskRepo.GetTaskSessionWorkspacePathsByTaskEnvironment(ctx, environmentID)
	if err != nil {
		log.Debug("cannot load recorded session workspaces for git status",
			zap.String("session_id", requested.ID),
			zap.String("task_environment_id", environmentID),
			zap.String("reason", "workspace_lookup_failed"),
			zap.Error(err))
		return nil, nil, false
	}
	return sessions, recordedPaths, true
}

func collectGitStatusSourceIDs(
	sessions []*models.TaskSession,
	env *models.TaskEnvironment,
	recordedPaths map[string]string,
	log *logger.Logger,
) []string {
	sourceIDs := make([]string, 0, len(sessions))
	seen := make(map[string]struct{}, len(sessions))
	for _, candidate := range sessions {
		if !eligibleGitStatusSession(candidate, env.ID, env.WorkspacePath, recordedPaths, log) {
			continue
		}
		if _, exists := seen[candidate.ID]; exists {
			continue
		}
		seen[candidate.ID] = struct{}{}
		sourceIDs = append(sourceIDs, candidate.ID)
	}
	return sourceIDs
}

func orderGitStatusSourceIDs(sourceIDs []string, requestedID string) []string {
	ordered := make([]string, 0, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		if sourceID == requestedID {
			ordered = append([]string{requestedID}, ordered...)
			continue
		}
		ordered = append(ordered, sourceID)
	}
	return ordered
}

func eligibleGitStatusSession(
	candidate *models.TaskSession,
	environmentID, canonicalWorkspacePath string,
	recordedPaths map[string]string,
	log *logger.Logger,
) bool {
	if candidate == nil {
		return false
	}
	if candidate.TaskEnvironmentID != environmentID {
		log.Debug("rejecting git status source",
			zap.String("source_session_id", candidate.ID),
			zap.String("task_environment_id", environmentID),
			zap.String("reason", "environment_mismatch"))
		return false
	}
	workspacePath, recorded := recordedPaths[candidate.ID]
	if !recorded || workspacePath == "" {
		log.Debug("rejecting git status source",
			zap.String("source_session_id", candidate.ID),
			zap.String("task_environment_id", environmentID),
			zap.String("reason", "workspace_unverified"))
		return false
	}
	if workspacePath != canonicalWorkspacePath {
		log.Debug("rejecting git status source",
			zap.String("source_session_id", candidate.ID),
			zap.String("task_environment_id", environmentID),
			zap.String("reason", "workspace_mismatch"))
		return false
	}
	return true
}

func newestGitStatusSnapshotsByRepository(snapshots []*models.GitSnapshot) map[string]*models.GitSnapshot {
	newest := make(map[string]*models.GitSnapshot)
	for _, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		repositoryName := gitStatusRepositoryName(snapshot)
		current := newest[repositoryName]
		if current == nil || newerGitStatusSnapshot(snapshot, current) {
			newest[repositoryName] = snapshot
		}
	}
	return newest
}

func gitStatusRepositoryName(snapshot *models.GitSnapshot) string {
	if snapshot != nil && snapshot.Metadata != nil {
		if repositoryName, ok := snapshot.Metadata["repository_name"].(string); ok {
			return repositoryName
		}
	}
	return ""
}

func newerGitStatusSnapshot(candidate, current *models.GitSnapshot) bool {
	candidateAt := gitStatusObservationTime(candidate)
	currentAt := gitStatusObservationTime(current)
	if !candidateAt.Equal(currentAt) {
		return candidateAt.After(currentAt)
	}
	if !candidate.CreatedAt.Equal(current.CreatedAt) {
		return candidate.CreatedAt.After(current.CreatedAt)
	}
	return candidate.ID > current.ID
}

func gitStatusObservationTime(snapshot *models.GitSnapshot) time.Time {
	if snapshot != nil && snapshot.Metadata != nil {
		if timestamp, ok := snapshot.Metadata["timestamp"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
				return parsed
			}
		}
	}
	if snapshot == nil {
		return time.Time{}
	}
	return snapshot.CreatedAt
}
