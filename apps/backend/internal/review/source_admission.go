package review

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
)

const gitCommonDirOutputLimit = 4096

// SourceAdmitted reports whether worktreePath may be reviewed for a task whose
// admitted roots are roots and whose registered local repositories are
// repositories. A path inside a root is admitted without running Git. Otherwise
// the path is admitted only when it is another worktree of a registered
// repository, that is, its Git common directory is the repository's. A path
// that is not a Git work tree, or whose probe fails, is rejected.
func SourceAdmitted(ctx context.Context, worktreePath string, roots, repositories []string) bool {
	if SourceWithinRoots(worktreePath, roots) {
		return true
	}
	if len(repositories) == 0 || !localPath(worktreePath) || hasTraversalParent(worktreePath) {
		return false
	}
	path, ok := canonicalLocalPath(worktreePath)
	if !ok {
		return false
	}
	common, ok := gitCommonDir(ctx, path)
	if !ok {
		return false
	}
	for _, repository := range repositories {
		if !localPath(repository) {
			continue
		}
		resolved, ok := canonicalLocalPath(repository)
		if !ok {
			continue
		}
		if repositoryCommon, ok := gitCommonDir(ctx, resolved); ok && repositoryCommon == common {
			return true
		}
	}
	return false
}

// gitCommonDir returns the canonical common Git directory of the work tree at
// path. Repository-binding variables such as GIT_DIR are removed from the
// environment so the probe can only describe path itself.
func gitCommonDir(ctx context.Context, path string) (string, bool) {
	var output localGitBuffer
	output.limit = gitCommonDirOutputLimit
	cmdErr, ctxErr := subproc.RunGitAfterAcquire(ctx, subproc.GitInteractive, 30*time.Second,
		func(execCtx context.Context) *exec.Cmd {
			cmd := subproc.NewGitCommand(execCtx, "-C", path, "rev-parse", "--path-format=absolute", "--git-common-dir")
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
