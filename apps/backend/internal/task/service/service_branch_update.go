package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

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
	if req.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if req.TaskRepositoryID == "" {
		return nil, fmt.Errorf("task_repository_id is required")
	}
	baseBranch := strings.TrimSpace(req.BaseBranch)
	if baseBranch == "" {
		return nil, fmt.Errorf("base_branch is required")
	}
	// Reject values that would be unsafe to splice into a `git` argument
	// list downstream (the picker payload is user-controlled and reaches
	// `exec.Command("git", …, baseBranch)` via the agentctl workspace
	// tracker). Mirrors process.IsSafeGitRef in the agentctl side; kept
	// independent here so the service stays self-contained.
	if !isSafeBaseBranchRef(baseBranch) {
		return nil, fmt.Errorf("base_branch contains characters not allowed in a git ref name")
	}

	taskRepo, err := s.taskRepos.GetTaskRepository(ctx, req.TaskRepositoryID)
	if err != nil {
		// Repo-tier "task repository not found" is a formatted string, not
		// a typed error today; fold any missing-row outcome into
		// ErrTaskRepositoryNotFound for the caller's error class.
		if strings.Contains(err.Error(), "task repository not found") {
			return nil, ErrTaskRepositoryNotFound
		}
		return nil, fmt.Errorf("get task repository: %w", err)
	}
	if taskRepo == nil || taskRepo.TaskID != req.TaskID {
		return nil, ErrTaskRepositoryNotFound
	}

	if taskRepo.BaseBranch == baseBranch {
		return taskRepo, nil
	}

	taskRepo.BaseBranch = baseBranch
	if err := s.taskRepos.UpdateTaskRepository(ctx, taskRepo); err != nil {
		return nil, fmt.Errorf("update task repository: %w", err)
	}

	// Rewrite per-session base_branch + clear the cached base_commit_sha so
	// session.git.commits and session.cumulative_diff stop filtering against
	// the SHA captured at the OLD base. Without this the task-card stats
	// (live BaseCommit, Phase 1) and the commits panel (cached SHA) drift
	// out of sync — visible as "commits disappear after switching base".
	if _, err := s.sessions.ResetTaskSessionBasesForRepository(ctx, req.TaskID, taskRepo.RepositoryID, baseBranch); err != nil {
		s.logger.Warn("UpdateRepositoryBaseBranch: failed to reset session bases",
			zap.String("task_id", req.TaskID),
			zap.String("repository_id", taskRepo.RepositoryID),
			zap.Error(err))
	}

	task, err := s.tasks.GetTask(ctx, req.TaskID)
	if err == nil && task != nil {
		s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	}

	if s.baseBranchPusher != nil {
		branches, mapErr := s.collectTaskBaseBranches(ctx, req.TaskID)
		if mapErr != nil {
			s.logger.Warn("UpdateRepositoryBaseBranch: failed to collect base branches for live push",
				zap.String("task_id", req.TaskID),
				zap.Error(mapErr))
		} else {
			s.baseBranchPusher.PushBaseBranchesForTask(ctx, req.TaskID, branches)
		}
	}

	return taskRepo, nil
}

// isSafeBaseBranchRef applies the same subset-of-`git check-ref-format`
// rules that process.IsSafeGitRef enforces on the agentctl side. Kept here
// so the service layer can reject a malicious picker payload before it
// reaches the DB or the live agentctl push — and so static analysis sees a
// sanitizer between the HTTP/WS handler input and any downstream `git`
// invocation. Empty input returns false (callers gate on len > 0 first).
func isSafeBaseBranchRef(ref string) bool {
	if ref == "" || len(ref) > 255 {
		return false
	}
	if ref[0] == '-' || ref[0] == '/' || ref[len(ref)-1] == '/' {
		return false
	}
	if strings.Contains(ref, "..") || strings.Contains(ref, "@{") {
		return false
	}
	for i := 0; i < len(ref); i++ {
		c := ref[i]
		if c < 0x20 || c == 0x7f {
			return false
		}
		switch c {
		case ' ', '~', '^', ':', '?', '*', '[', '\\', '`', ';', '|', '&', '$', '(', ')', '\'', '"':
			return false
		}
	}
	return true
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
