package process

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/kandev/kandev/internal/common/subproc"
)

type gitWorkClassContextKey struct{}

func withGitWorkClass(ctx context.Context, class subproc.GitWorkClass) context.Context {
	return context.WithValue(ctx, gitWorkClassContextKey{}, class)
}

func gitWorkClass(ctx context.Context) subproc.GitWorkClass {
	if class, ok := ctx.Value(gitWorkClassContextKey{}).(subproc.GitWorkClass); ok {
		return class
	}
	return subproc.GitInteractive
}

// gitOptionalLocksOff is the env var git reads to skip "optional" locks, i.e.
// the index refresh lock that `git status` and friends take to update stat
// info. The workspace tracker polls git read-only, but without this flag even
// those reads can briefly take `.git/index.lock`, racing with concurrent user
// operations (stage, commit) that need it for writes — in tight-loop polling
// this manifests as sporadic "Unable to create '.../index.lock': File exists"
// failures from user commands.
//
// See: https://git-scm.com/docs/git#Documentation/git.txt-codeGITOPTIONALLOCKSltbooleangtcode
const gitOptionalLocksOff = "GIT_OPTIONAL_LOCKS=0"

// pollingGitCommand builds an exec.Cmd with optional Git locks disabled. The
// lock policy is independent from the admission class: fresh interactive
// status also needs lockless reads while still using the interactive queue.
func (wt *WorkspaceTracker) pollingGitCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := subproc.NewGitCommand(ctx, args...)
	cmd.Dir = wt.workDir
	cmd.Env = gitCommandEnv(ctx, true)
	return cmd
}

func gitCommandEnv(ctx context.Context, lockless bool) []string {
	env := os.Environ()
	if lockless {
		env = replaceGitEnvAssignment(env, gitOptionalLocksOff)
	}
	if indexPath := gitIndexFile(ctx); indexPath != "" {
		env = replaceGitEnvAssignment(env, "GIT_INDEX_FILE="+indexPath)
	}
	return env
}

func replaceGitEnvAssignment(env []string, assignment string) []string {
	key, _, ok := strings.Cut(assignment, "=")
	if !ok {
		return env
	}
	prefix := key + "="
	filtered := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, assignment)
}

// gitCommand builds a git command for the already-admitted execution context.
// The timeout is deliberately owned by subproc.RunGit*AfterAcquire so queue
// time cannot consume the command's execution budget.
func (wt *WorkspaceTracker) gitCommand(ctx context.Context, lockless bool, args ...string) *exec.Cmd {
	if lockless {
		return wt.pollingGitCommand(ctx, args...)
	}
	cmd := subproc.NewGitCommand(ctx, args...)
	cmd.Dir = wt.workDir
	cmd.Env = gitCommandEnv(ctx, false)
	return cmd
}

// runGitOutput runs a class-selected git command with a per-command timeout
// and returns its stdout. Background contexts also disable optional Git locks.
func (wt *WorkspaceTracker) runGitOutput(ctx context.Context, args ...string) ([]byte, error) {
	class := gitWorkClass(ctx)
	return wt.runGitOutputClass(ctx, class, class == subproc.GitBackground, args...)
}

// runGitOutputClass runs a Git command under the supplied admission class and
// independently chooses whether Git's optional locks are disabled. Keeping
// these concerns separate prevents a fresh interactive status from being
// accidentally scheduled on the background FIFO just because it is read-only.
func (wt *WorkspaceTracker) runGitOutputClass(
	ctx context.Context,
	class subproc.GitWorkClass,
	lockless bool,
	args ...string,
) ([]byte, error) {
	out, runErr, execCtxErr := subproc.RunGitOutputAfterAcquire(
		ctx,
		class,
		gitCommandTimeout,
		func(execCtx context.Context) *exec.Cmd {
			return wt.gitCommand(execCtx, lockless, args...)
		},
	)
	return out, gitCommandError(runErr, execCtxErr)
}

// runGit is runGitOutput's no-stdout sibling for verify-style probes where
// only the exit code matters.
func (wt *WorkspaceTracker) runGit(ctx context.Context, args ...string) error {
	class := gitWorkClass(ctx)
	runErr, execCtxErr := subproc.RunGitAfterAcquire(
		ctx,
		class,
		gitCommandTimeout,
		func(execCtx context.Context) *exec.Cmd {
			return wt.gitCommand(execCtx, class == subproc.GitBackground, args...)
		},
	)
	return gitCommandError(runErr, execCtxErr)
}

// runPollingGitOutput is runGitOutput's polling sibling. It always uses the
// background class and builds the command with GIT_OPTIONAL_LOCKS=0.
func (wt *WorkspaceTracker) runPollingGitOutput(ctx context.Context, args ...string) ([]byte, error) {
	out, runErr, execCtxErr := subproc.RunGitOutputAfterAcquire(
		ctx,
		subproc.GitBackground,
		gitCommandTimeout,
		func(execCtx context.Context) *exec.Cmd {
			return wt.gitCommand(execCtx, true, args...)
		},
	)
	return out, gitCommandError(runErr, execCtxErr)
}

func (wt *WorkspaceTracker) runPollingGitOutputWithStderr(ctx context.Context, args ...string) ([]byte, string, error) {
	var stderr bytes.Buffer
	out, runErr, execCtxErr := subproc.RunGitOutputAfterAcquire(
		ctx,
		subproc.GitBackground,
		gitCommandTimeout,
		func(execCtx context.Context) *exec.Cmd {
			cmd := wt.gitCommand(execCtx, true, args...)
			cmd.Stderr = &stderr
			return cmd
		},
	)
	return out, strings.TrimSpace(stderr.String()), gitCommandError(runErr, execCtxErr)
}

func (wt *WorkspaceTracker) runPollingGit(ctx context.Context, args ...string) error {
	runErr, execCtxErr := subproc.RunGitAfterAcquire(
		ctx,
		subproc.GitBackground,
		gitCommandTimeout,
		func(execCtx context.Context) *exec.Cmd {
			return wt.gitCommand(execCtx, true, args...)
		},
	)
	return gitCommandError(runErr, execCtxErr)
}

func gitCommandError(runErr, execCtxErr error) error {
	if runErr != nil {
		return runErr
	}
	return execCtxErr
}
