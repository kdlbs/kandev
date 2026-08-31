package process

import (
	"bytes"
	"context"
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

const defaultGitSSHCommand = "ssh -oBatchMode=yes"

// pollingGitCommand builds an exec.Cmd with optional Git locks disabled. The
// lock policy is independent from the admission class: fresh interactive
// status also needs lockless reads while still using the interactive queue.
func (wt *WorkspaceTracker) pollingGitCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := subproc.NewGitCommand(ctx, args...)
	cmd.Dir = wt.workDir
	cmd.Env = wt.gitCommandEnv(ctx, true)
	return cmd
}

func (wt *WorkspaceTracker) gitCommandEnv(ctx context.Context, lockless bool) []string {
	env := wt.gitEnvironmentSnapshot()
	if lockless {
		env = replaceGitEnvAssignment(env, gitOptionalLocksOff)
	}
	if indexPath := gitIndexFile(ctx); indexPath != "" {
		env = replaceGitEnvAssignment(env, "GIT_INDEX_FILE="+indexPath)
	}
	for _, assignment := range []string{
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=Never",
		"GIT_ASKPASS=echo",
		"SSH_ASKPASS=/bin/false",
	} {
		env = replaceGitEnvAssignment(env, assignment)
	}
	if command, ok := gitEnvValue(env, "GIT_SSH_COMMAND"); ok && strings.TrimSpace(command) != "" {
		env = replaceGitEnvAssignment(env, "GIT_SSH_COMMAND="+forceGitSSHBatchMode(command))
	} else {
		env = replaceGitEnvAssignment(env, "GIT_SSH_COMMAND="+defaultGitSSHCommand)
	}
	return env
}

func gitEnvValue(env []string, key string) (string, bool) {
	prefix := key + "="
	var value string
	found := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			value = strings.TrimPrefix(entry, prefix)
			found = true
		}
	}
	return value, found
}

// forceGitSSHBatchMode places BatchMode=yes immediately after a direct
// OpenSSH executable. OpenSSH uses the first command-line value for an option,
// so this prevents a later BatchMode=no in the inherited command from
// restoring terminal prompting while preserving the command's other options.
// Shell prefixes and custom wrappers are not parsed here. They use the safe
// default instead of receiving an option in the wrong position.
func forceGitSSHBatchMode(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return defaultGitSSHCommand
	}
	commandEnd := shellWordEnd(command)
	if commandEnd == 0 {
		return defaultGitSSHCommand
	}
	executable, ok := shellWordValue(command[:commandEnd])
	if !ok || !isOpenSSHExecutable(executable) {
		return defaultGitSSHCommand
	}
	return command[:commandEnd] + " -oBatchMode=yes" + command[commandEnd:]
}

func shellWordValue(word string) (string, bool) {
	if word == "" {
		return "", false
	}
	if word[0] == '\'' || word[0] == '"' {
		if len(word) < 2 || word[len(word)-1] != word[0] {
			return "", false
		}
		return word[1 : len(word)-1], true
	}
	if strings.ContainsAny(word, "'\"") {
		return "", false
	}
	return word, true
}

func isOpenSSHExecutable(executable string) bool {
	lastSeparator := strings.LastIndexAny(executable, `/\`)
	base := executable[lastSeparator+1:]
	return base == "ssh" || base == "ssh.exe"
}

func shellWordEnd(command string) int {
	var quote byte
	escaped := false
	for index := 0; index < len(command); index++ {
		character := command[index]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if quote == '"' && character == '\\' {
				escaped = true
				continue
			}
			if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\\':
			escaped = true
		case '\'', '"':
			quote = character
		case ' ', '\t', '\n', '\r':
			return index
		}
	}
	return len(command)
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
	cmd.Env = wt.gitCommandEnv(ctx, false)
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
