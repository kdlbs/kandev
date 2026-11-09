package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/review"
	"github.com/kandev/kandev/internal/task/models"
)

type reviewExecutorGetter interface {
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
}

type localReviewTaskRepositories interface {
	ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error)
}

func (r reviewSessionReader) localReviewSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	if r.repo == nil {
		return nil, fmt.Errorf("local review session unavailable")
	}
	session, err := r.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return nil, fmt.Errorf("local review session unavailable")
	}
	executors, ok := r.repo.(reviewExecutorGetter)
	if !ok {
		return nil, fmt.Errorf("local executor identity unavailable")
	}
	executor, err := executors.GetExecutor(ctx, session.ExecutorID)
	if err != nil || executor == nil {
		return nil, fmt.Errorf("local executor identity unavailable")
	}
	switch executor.Type {
	case models.ExecutorTypeLocal, models.ExecutorTypeWorktree, models.ExecutorType("local_pc"):
	default:
		return nil, fmt.Errorf("executor %s is not local", executor.Type)
	}
	return session, nil
}

// LocalReviewFiles reads the persisted local workspace without restoring its runtime.
func (r reviewSessionReader) LocalReviewFiles(ctx context.Context, sessionID string, committed bool) (map[string]any, error) {
	session, err := r.localReviewSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	repos := session.Worktrees
	if len(repos) == 0 && session.WorkspacePath != "" {
		repos = []*models.TaskEnvironmentRepo{{WorktreePath: session.WorkspacePath, RepositoryID: session.RepositoryID}}
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("local review worktree unavailable")
	}
	files := make(map[string]any)
	for _, repo := range repos {
		if repo == nil {
			continue
		}
		changes, err := r.localReviewRepo(ctx, session, repo, committed, len(repos) > 1)
		if err != nil {
			return nil, err
		}
		for _, file := range changes {
			files[file.Key()] = map[string]any{
				"path": file.Path, "diff": file.Diff, "status": file.Status,
				"additions": file.Additions, "deletions": file.Deletions,
				"repository_id":   file.RepositoryID,
				"repository_name": file.RepositoryName,
			}
		}
	}
	return files, nil
}

func (r reviewSessionReader) localReviewRepo(ctx context.Context, session *models.TaskSession, repo *models.TaskEnvironmentRepo, committed, multi bool) ([]review.ChangedFile, error) {
	source := review.Source{WorktreePath: repo.WorktreePath, BaseRef: "HEAD"}
	if committed {
		base, ok, err := r.localReviewBase(ctx, session, repo, multi)
		if err != nil || !ok {
			return nil, err
		}
		source.BaseRef, source.HeadRef = base, "HEAD"
	}
	files, err := review.CollectLocalChanges(ctx, source)
	if err != nil {
		return nil, err
	}
	// Match the Review panel's scope identity: a multi-repository task scopes a
	// file by its workspace-relative repository path, while a single repository
	// keeps the legacy bare-path key (both fields empty).
	scopeName, scopeID := "", ""
	if multi {
		scopeName, scopeID = repo.WorkspaceRelativePath, repo.RepositoryID
	}
	for i := range files {
		files[i].RepositoryID, files[i].RepositoryName = scopeID, scopeName
	}
	return files, nil
}

// localReviewBase requires an explicit repository-qualified target when one is
// attached. Without one, it prefers the live branch and falls back to the
// stored session base when that branch cannot be compared.
func (r reviewSessionReader) localReviewBase(ctx context.Context, session *models.TaskSession, repo *models.TaskEnvironmentRepo, multi bool) (string, bool, error) {
	link, hasLink, err := r.localReviewTaskRepository(ctx, session, repo)
	if err != nil {
		return "", false, fmt.Errorf("%w: resolve local review repository target: %v", review.ErrWorkspaceUnavailable, err)
	}
	if hasLink {
		target, explicit, err := models.LoadComparisonTarget(link.Metadata)
		if err != nil {
			return "", false, fmt.Errorf("%w: local review comparison target invalid: %v", review.ErrWorkspaceUnavailable, err)
		}
		if explicit {
			base, ok := review.ResolveLocalComparisonRef(ctx, repo.WorktreePath, target.ComparisonRef())
			if !ok {
				return "", false, fmt.Errorf("%w: explicit comparison target unavailable", review.ErrWorkspaceUnavailable)
			}
			return base, true, nil
		}
	}

	target, stored := session.BaseBranch, session.BaseCommitSHA
	if multi && repo.RepositoryID != session.RepositoryID {
		if !hasLink {
			return "", false, fmt.Errorf("%w: local review comparison base unavailable", review.ErrWorkspaceUnavailable)
		}
		target, stored = link.BaseBranch, ""
	}
	base, ok := review.ResolveLocalComparisonBase(ctx, repo.WorktreePath, target, stored)
	return base, ok, nil
}

// localReviewTaskRepository resolves the attachment for one checkout.
func (r reviewSessionReader) localReviewTaskRepository(ctx context.Context, session *models.TaskSession, repo *models.TaskEnvironmentRepo) (*models.TaskRepository, bool, error) {
	getter, ok := r.repo.(localReviewTaskRepositories)
	if !ok {
		return nil, false, nil
	}
	links, err := getter.ListTaskRepositories(ctx, session.TaskID)
	if err != nil {
		return nil, false, err
	}
	var match *models.TaskRepository
	for _, link := range links {
		if link == nil || link.TaskID != session.TaskID || link.RepositoryID != repo.RepositoryID {
			continue
		}
		if match != nil {
			return nil, false, fmt.Errorf("local review repository attachment is ambiguous")
		}
		match = link
	}
	return match, match != nil, nil
}
