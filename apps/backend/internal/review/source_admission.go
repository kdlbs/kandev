package review

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/task/models"
)

const gitCommonDirOutputLimit = 4096

// PinSource uses the resolved checkout as the actual collection input.
func PinSource(ctx context.Context, source Source) (*Source, error) {
	common, ok := gitCommonDir(ctx, source.WorktreePath)
	root, known := gitCheckoutPath(ctx, source.WorktreePath, "--show-toplevel")
	if !ok || !known || !pathWithin(source.WorktreePath, root) || !hasOwnedGitObjects(common) {
		return nil, ErrWorkspaceUnavailable
	}
	source.admittedCommonDir = common
	source.admittedWorktreeRoot = root
	return &source, nil
}

func ResolveSourcePath(source Source) (Source, error) {
	path, ok := canonicalLocalPath(source.WorktreePath)
	if !ok || hasTraversalParent(source.WorktreePath) {
		return Source{}, ErrWorkspaceUnavailable
	}
	source.WorktreePath = path
	return source, nil
}

func sourceRepositoryStillMatches(ctx context.Context, source Source, path string) bool {
	path, ok := canonicalLocalPath(path)
	if !ok || path != source.WorktreePath {
		return false
	}
	common, commonOK := gitCommonDir(ctx, path)
	top, topOK := gitCheckoutPath(ctx, path, "--show-toplevel")
	return commonOK && topOK && common == source.admittedCommonDir && top == source.admittedWorktreeRoot &&
		hasOwnedGitObjects(common)
}

func hasOwnedGitObjects(common string) bool {
	objects, ok := canonicalLocalPath(filepath.Join(common, "objects"))
	if !ok || !pathWithin(objects, common) {
		return false
	}
	// Alternates can make a source read commits from a separate host store.
	// Native linked worktrees share the common directory without alternates.
	_, err := os.Lstat(filepath.Join(objects, "info", "alternates"))
	return errors.Is(err, os.ErrNotExist)
}

func ApproveSource(ctx context.Context, userID string, source Source) (*models.ReviewSourceApproval, error) {
	resolved, err := ResolveSourcePath(source)
	if err != nil {
		return nil, err
	}
	pinned, err := PinSource(ctx, resolved)
	if err != nil {
		return nil, err
	}
	return &models.ReviewSourceApproval{
		UserID: userID, Path: pinned.WorktreePath, CommonDir: pinned.admittedCommonDir, WorktreeRoot: pinned.admittedWorktreeRoot,
		BaseRef: source.BaseRef, HeadRef: source.HeadRef,
	}, nil
}

func ApprovedSource(ctx context.Context, source Source, approval *models.ReviewSourceApproval) (*Source, error) {
	if approval == nil || approval.UserID == "" || source.BaseRef != approval.BaseRef || source.HeadRef != approval.HeadRef {
		return nil, ErrWorkspaceUnavailable
	}
	if source.WorktreePath != approval.Path {
		return nil, ErrWorkspaceUnavailable
	}
	source.admittedCommonDir = approval.CommonDir
	source.admittedWorktreeRoot = approval.WorktreeRoot
	if !sourceRepositoryStillMatches(ctx, source, source.WorktreePath) {
		return nil, ErrWorkspaceUnavailable
	}
	return &source, nil
}

// SourceAdmitted reports whether worktreePath may be reviewed for a task whose
// admitted roots are roots and whose registered local repositories are
// repositories. A checkout and its Git common directory must be in admitted
// roots, or it must be a worktree of a registered repository. A gitfile inside
// an admitted root does not authorize the external repository it names.
func SourceAdmitted(ctx context.Context, worktreePath string, roots, repositories []string) bool {
	_, ok := admitSource(ctx, Source{WorktreePath: worktreePath}, roots, repositories)
	return ok
}

func admitSource(ctx context.Context, source Source, roots, repositories []string) (*Source, bool) {
	resolved, err := ResolveSourcePath(source)
	if err != nil {
		return nil, false
	}
	pathInside := SourceWithinRoots(resolved.WorktreePath, roots)
	if !pathInside && len(repositories) == 0 {
		return nil, false
	}
	pinned, err := PinSource(ctx, resolved)
	if err != nil {
		return nil, false
	}
	if pathInside && SourceWithinRoots(pinned.admittedWorktreeRoot, roots) &&
		SourceWithinRoots(pinned.admittedCommonDir, roots) {
		return pinned, true
	}
	for _, repository := range repositories {
		if !localPath(repository) {
			continue
		}
		canonical, ok := canonicalLocalPath(repository)
		if !ok {
			continue
		}
		if common, ok := gitCommonDir(ctx, canonical); ok && common == pinned.admittedCommonDir &&
			isRegisteredWorktree(ctx, pinned, canonical) {
			return pinned, true
		}
	}
	return nil, false
}

func isRegisteredWorktree(ctx context.Context, source *Source, registered string) bool {
	root, ok := gitCheckoutPath(ctx, registered, "--show-toplevel")
	if !ok {
		return false
	}
	if root == source.admittedWorktreeRoot {
		return true
	}
	// Git's linked-worktree admin directory names the gitfile of the actual
	// checkout. Match that file to the effective worktree Git would read.
	gitDir, ok := gitCheckoutPath(ctx, source.WorktreePath, "--absolute-git-dir")
	if !ok || gitDir == source.admittedCommonDir || !pathWithin(gitDir, source.admittedCommonDir) {
		return false
	}
	backlink, err := os.ReadFile(filepath.Join(gitDir, "gitdir"))
	if err != nil {
		return false
	}
	target, ok := canonicalLocalPath(strings.TrimSpace(string(backlink)))
	return ok && target == filepath.Join(source.admittedWorktreeRoot, ".git")
}

// gitCommonDir returns the canonical common Git directory of the work tree at
// path. Repository-binding variables such as GIT_DIR are removed from the
// environment so the probe can only describe path itself.
func gitCommonDir(ctx context.Context, path string) (string, bool) {
	return gitCheckoutPath(ctx, path, "--git-common-dir")
}

func gitCheckoutPath(ctx context.Context, path, argument string) (string, bool) {
	var output localGitBuffer
	output.limit = gitCommonDirOutputLimit
	cmdErr, ctxErr := subproc.RunGitAfterAcquire(ctx, subproc.GitInteractive, 30*time.Second,
		func(execCtx context.Context) *exec.Cmd {
			cmd := subproc.NewGitCommand(execCtx, "-C", path, "rev-parse", "--path-format=absolute", argument)
			cmd.Env = repositoryNeutralGitEnv(os.Environ())
			cmd.Stdout = &output
			return cmd
		})
	if cmdErr != nil || ctxErr != nil || output.exceeded {
		return "", false
	}
	common := strings.TrimSpace(string(output.data))
	if !localPath(common) {
		return "", false
	}
	return canonicalLocalPath(common)
}

func repositoryNeutralGitEnv(environ []string) []string {
	filtered := make([]string, 0, len(environ))
	for _, entry := range environ {
		key, _, _ := strings.Cut(entry, "=")
		if repositoryBindingGitVar(strings.ToUpper(key)) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func repositoryBindingGitVar(key string) bool {
	for _, prefix := range []string{
		"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_IMPLICIT_WORK_TREE", "GIT_PREFIX",
		"GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM", "GIT_NAMESPACE",
		"GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_", "GIT_CONFIG_VALUE_", "GIT_CONFIG_PARAMETERS",
	} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// TaskSourceLookup resolves the existing task-owned repository and session paths.
type TaskSourceLookup interface {
	ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error)
	GetRepository(context.Context, string) (*models.Repository, error)
	ListTaskSessions(context.Context, string) ([]*models.TaskSession, error)
	ReadExecutorType(context.Context, string) (models.ExecutorType, error)
}

// TaskSourceAdmitted applies the same checkout authority at request and collection time.
func TaskSourceAdmitted(ctx context.Context, lookup TaskSourceLookup, taskID, path string) (bool, error) {
	source, err := TaskReviewSource(ctx, lookup, taskID, Source{WorktreePath: path})
	return source != nil, err
}

// TaskReviewSource returns the same checkout checked for the task.
func TaskReviewSource(ctx context.Context, lookup TaskSourceLookup, taskID string, source Source) (*Source, error) {
	roots, repositories, err := taskSourceRoots(ctx, lookup, taskID)
	if err != nil {
		return nil, err
	}
	admitted, _ := admitSource(ctx, source, roots, repositories)
	return admitted, nil
}

// taskSourceRoots lists the admitted roots, which are the task's registered
// local repository checkouts and the workspace and worktree paths of its
// sessions on local executors, and separately the registered checkouts whose
// other worktrees are also admitted. A remote session's paths name another
// machine, so they never admit a backend path.
func taskSourceRoots(ctx context.Context, lookup TaskSourceLookup, taskID string) ([]string, []string, error) {
	links, err := lookup.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	var repositories []string
	for _, link := range links {
		if link == nil || link.RepositoryID == "" {
			continue
		}
		repository, err := lookup.GetRepository(ctx, link.RepositoryID)
		if err != nil {
			return nil, nil, err
		}
		if repository != nil && repository.LocalPath != "" {
			repositories = append(repositories, repository.LocalPath)
		}
	}
	roots := append([]string(nil), repositories...)
	sessions, err := lookup.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	for _, session := range sessions {
		if session == nil || !localReviewExecutor(ctx, lookup, session.ExecutorID) {
			continue
		}
		roots = append(roots, session.WorkspacePath)
		for _, worktree := range session.Worktrees {
			if worktree != nil {
				roots = append(roots, worktree.WorktreePath)
			}
		}
	}
	return roots, repositories, nil
}

func localReviewExecutor(ctx context.Context, lookup TaskSourceLookup, executorID string) bool {
	if executorID == "" {
		return false
	}
	executorType, err := lookup.ReadExecutorType(ctx, executorID)
	if err != nil {
		return false
	}
	switch executorType {
	case models.ExecutorTypeLocal, models.ExecutorTypeWorktree, models.ExecutorType("local_pc"):
		return true
	}
	return false
}
