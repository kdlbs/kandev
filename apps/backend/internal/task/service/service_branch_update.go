package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

// UpdateRepositoryBaseBranchRequest carries the parameters for the
// changes-panel "Compare against" picker. Mutates exactly one
// task_repositories row.
type UpdateRepositoryBaseBranchRequest struct {
	TaskID           string
	TaskRepositoryID string
	BaseBranch       string
}

// ErrTaskRepositoryNotFound is returned when the supplied task_repository_id
// has no row, or it belongs to a different task than the caller claimed.
var ErrTaskRepositoryNotFound = errors.New("task repository not found")

// UpdateRepositoryBaseBranch changes the base_branch on a single
// task_repositories row, publishes task.updated so connected clients refresh,
// and pushes the new per-repo map to the live agentctl instance (if any) so
// the changes panel updates its BaseCommit / Ahead / Behind without waiting
// for a session restart.
//
// The DB write is the source of truth; a failed push is logged at warn but
// does NOT roll the DB back — at next session launch the persisted map
// rebuilds trackers correctly. Callers that need stronger guarantees can
// re-issue the request.
//
// Validation:
//   - TaskID, TaskRepositoryID, BaseBranch all required.
//   - BaseBranch is trimmed; whitespace-only is rejected.
//   - The TaskRepository row must belong to the supplied TaskID — guards
//     against a caller pointing at someone else's task_repository_id.
//
// Returns the updated TaskRepository on success.
func (s *Service) UpdateRepositoryBaseBranch(ctx context.Context, req UpdateRepositoryBaseBranchRequest) (*models.TaskRepository, error) {
	baseBranch, err := validateUpdateRepositoryBaseBranchRequest(req)
	if err != nil {
		return nil, err
	}
	taskRepo, err := s.loadTaskRepositoryForUpdate(ctx, req.TaskID, req.TaskRepositoryID)
	if err != nil {
		return nil, err
	}
	if taskRepo.BaseBranch == baseBranch {
		return taskRepo, nil
	}

	taskRepo.BaseBranch = baseBranch
	if err := s.taskRepos.UpdateTaskRepository(ctx, taskRepo); err != nil {
		return nil, fmt.Errorf("update task repository: %w", err)
	}

	// Detach from the caller's ctx for post-commit fan-out: the DB row is
	// already persisted, so if the HTTP / WS request gets cancelled mid-
	// response we must still clear session.base_commit_sha and push to
	// agentctl — otherwise the persisted task_repositories row would
	// disagree with the cached session SHA + live tracker map until the
	// next session launch.
	s.applyBaseBranchSideEffects(context.WithoutCancel(ctx), req.TaskID, taskRepo.RepositoryID, baseBranch)
	return taskRepo, nil
}

// validateUpdateRepositoryBaseBranchRequest checks required fields and
// returns the trimmed + sanitized base_branch. Pulled out so the main
// service method stays under the cyclomatic-complexity lint cap.
func validateUpdateRepositoryBaseBranchRequest(req UpdateRepositoryBaseBranchRequest) (string, error) {
	if req.TaskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	if req.TaskRepositoryID == "" {
		return "", fmt.Errorf("task_repository_id is required")
	}
	baseBranch := strings.TrimSpace(req.BaseBranch)
	if baseBranch == "" {
		return "", fmt.Errorf("base_branch is required")
	}
	// Reject values that would be unsafe to splice into a `git` argument
	// list downstream (the picker payload is user-controlled and reaches
	// `exec.Command("git", …, baseBranch)` via the agentctl workspace
	// tracker). Mirrors process.IsSafeGitRef in the agentctl side; kept
	// independent here so the service stays self-contained.
	if !isSafeBaseBranchRef(baseBranch) {
		return "", fmt.Errorf("base_branch contains characters not allowed in a git ref name")
	}
	return baseBranch, nil
}

// loadTaskRepositoryForUpdate fetches the row and validates it belongs to
// the supplied task. Folds the repo-tier "not found" string error into
// ErrTaskRepositoryNotFound for stable caller-side classification.
func (s *Service) loadTaskRepositoryForUpdate(ctx context.Context, taskID, taskRepositoryID string) (*models.TaskRepository, error) {
	taskRepo, err := s.taskRepos.GetTaskRepository(ctx, taskRepositoryID)
	if err != nil {
		if strings.Contains(err.Error(), "task repository not found") {
			return nil, ErrTaskRepositoryNotFound
		}
		return nil, fmt.Errorf("get task repository: %w", err)
	}
	if taskRepo == nil || taskRepo.TaskID != taskID {
		return nil, ErrTaskRepositoryNotFound
	}
	return taskRepo, nil
}

// applyBaseBranchSideEffects runs the post-write fan-out that keeps the
// commits panel, cumulative diff, WS-driven UIs, and the running agentctl
// instance aligned with the new base. Each step is best-effort: failures
// here don't roll back the DB because the persisted task_repositories
// row is the authoritative source for the next session launch.
func (s *Service) applyBaseBranchSideEffects(ctx context.Context, taskID, repositoryID, baseBranch string) {
	if _, err := s.sessions.ResetTaskSessionBasesForRepository(ctx, taskID, repositoryID, baseBranch); err != nil {
		s.logger.Warn("UpdateRepositoryBaseBranch: failed to reset session bases",
			zap.String("task_id", taskID),
			zap.String("repository_id", repositoryID),
			zap.Error(err))
	}
	if task, err := s.tasks.GetTask(ctx, taskID); err == nil && task != nil {
		s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	}
	if s.baseBranchPusher == nil {
		return
	}
	branches, mapErr := s.collectTaskBaseBranches(ctx, taskID)
	if mapErr != nil {
		s.logger.Warn("UpdateRepositoryBaseBranch: failed to collect base branches for live push",
			zap.String("task_id", taskID),
			zap.Error(mapErr))
		return
	}
	// Empty map = task currently has no recorded base_branches. Pushing
	// nil to agentctl would call Manager.UpdateBaseBranches(nil) and wipe
	// every tracker's override, including ones the caller didn't touch.
	// Skip the push instead — the DB row we just updated is the source of
	// truth for the next session launch.
	if len(branches) == 0 {
		return
	}
	s.baseBranchPusher.PushBaseBranchesForTask(ctx, taskID, branches)
}

// isSafeBaseBranchRef delegates to the shared
// `securityutil.IsValidBranchName` allowlist so the service-tier rejection
// rules track agentctl's exactly. Without this, a value like
// `feature/@2024` would pass here, persist to task_repositories, then get
// dropped by the agentctl-side sanitiser at push time — silently wiping
// every tracker's override. Sharing one allowlist keeps the DB write and
// the live push agreeing on what counts as a valid ref name.
//
// `origin/<name>` refs are split before validation because the underlying
// regex disallows "/" as the first character.
func isSafeBaseBranchRef(ref string) bool {
	if rest, ok := strings.CutPrefix(ref, "origin/"); ok {
		return securityutil.IsValidBranchName(rest)
	}
	return securityutil.IsValidBranchName(ref)
}

// collectTaskBaseBranches builds the per-repo {RepositoryName → base_branch}
// map the agentctl WorkspaceTracker reads. Mirrors lifecycle.collectBaseBranches
// but at update time the LaunchRequest is gone, so we hydrate from the DB:
// list task_repositories, resolve each Repository to recover its Name (which
// matches the worktree dir basename and therefore the tracker's
// repositoryName), and key the map with both Name and the empty fallback for
// single-repo tasks.
func (s *Service) collectTaskBaseBranches(ctx context.Context, taskID string) (map[string]string, error) {
	taskRepos, err := s.taskRepos.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list task repositories: %w", err)
	}
	out := make(map[string]string, len(taskRepos)+1)
	for _, tr := range taskRepos {
		if tr.BaseBranch == "" {
			continue
		}
		repo, err := s.repoEntities.GetRepository(ctx, tr.RepositoryID)
		if err != nil || repo == nil {
			continue
		}
		if repo.Name != "" {
			out[repo.Name] = tr.BaseBranch
		}
	}
	// Single-repo legacy fallback: when only one row, duplicate under the
	// empty key so the root WorkspaceTracker (repositoryName == "") picks it
	// up too — matches the synthesis lifecycle.collectBaseBranches performs
	// from req.RepoSpecs().
	if len(taskRepos) == 1 && taskRepos[0].BaseBranch != "" {
		if _, ok := out[""]; !ok {
			out[""] = taskRepos[0].BaseBranch
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
