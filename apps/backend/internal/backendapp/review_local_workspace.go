package backendapp

import (
	"context"
	"fmt"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

type reviewWorkspaceInspector interface {
	InspectSessionWorkspaceInfo(context.Context, string, string) (*agentruntime.WorkspaceInfo, error)
}

func (r reviewSessionReader) localReviewWorkspace(ctx context.Context, session *models.TaskSession) (*models.TaskSession, []*models.TaskEnvironmentRepo, error) {
	if r.workspaces == nil {
		if session.TaskEnvironmentID != "" {
			return nil, nil, fmt.Errorf("canonical review workspace is unavailable")
		}
		return session, legacyLocalReviewRepositories(session), nil
	}
	info, err := r.workspaces.InspectSessionWorkspaceInfo(ctx, session.TaskID, session.ID)
	if err != nil {
		return nil, nil, err
	}
	if info == nil || info.TaskID != session.TaskID || info.SessionID != session.ID {
		return nil, nil, fmt.Errorf("canonical review workspace identity changed")
	}
	// LocalReviewFiles has already checked the session's executor. Empty is the
	// legacy default local type, also accepted by workspace validation.
	switch info.ExecutorType {
	case "", string(models.ExecutorTypeLocal), string(models.ExecutorTypeWorktree), legacyLocalPCExecutor:
	default:
		return nil, nil, fmt.Errorf("canonical review workspace is not local")
	}
	copy := *session
	repos := make([]*models.TaskEnvironmentRepo, 0, len(info.WorkspaceRepositories))
	for _, repository := range info.WorkspaceRepositories {
		repos = append(repos, &models.TaskEnvironmentRepo{RepositoryID: repository.RepositoryID, WorktreePath: repository.WorktreePath, WorkspaceRelativePath: repository.WorkspaceRelativePath})
	}
	if len(repos) == 0 && info.WorkspacePath != "" {
		// The canonical inherited-local view can deliberately have no repository:
		// its child's old deleted source must not become the comparison identity.
		if session.RepositoryID != "" {
			copy.RepositoryID, copy.BaseCommitSHA, copy.BaseBranch = "", "", ""
		}
		repos = append(repos, &models.TaskEnvironmentRepo{WorktreePath: info.WorkspacePath})
	}
	return &copy, repos, nil
}

func legacyLocalReviewRepositories(session *models.TaskSession) []*models.TaskEnvironmentRepo {
	if len(session.Worktrees) > 0 {
		return session.Worktrees
	}
	if session.WorkspacePath != "" {
		return []*models.TaskEnvironmentRepo{{WorktreePath: session.WorkspacePath, RepositoryID: session.RepositoryID}}
	}
	return nil
}
