package worktree

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/subproc"
)

const (
	// maxRefreshLockAttempts bounds one refresh command, including its first attempt.
	maxRefreshLockAttempts = 4
	// refreshLockInitialBackoff is doubled before each later attempt.
	refreshLockInitialBackoff = 200 * time.Millisecond
)

// refreshLockExhaustedError is the credential-safe result of persistent lock
// contention. It carries fixed wording and never includes raw Git output.
type refreshLockExhaustedError struct {
	operation string
	attempts  int
	cause     error
}

func (e *refreshLockExhaustedError) Error() string {
	return fmt.Sprintf(
		"git %s failed after %d attempts: repository lock contention persisted; retry the launch after the lock becomes available",
		e.operation, e.attempts,
	)
}

func (e *refreshLockExhaustedError) Unwrap() error { return e.cause }

type refreshLockRetryResult struct {
	output     []byte
	runErr     error
	execCtxErr error
	attempts   int
}

type refreshLockAttempt struct {
	output      []byte
	runErr      error
	execCtxErr  error
	deadline    time.Time
	hasDeadline bool
	admitted    bool
}

// runRefreshGitWithLockRetry re-executes one backend base-refresh command while
// Git reports recognized lock contention. The first admitted attempt starts the
// only execution budget; later attempts and backoff share its deadline.
func (m *Manager) runRefreshGitWithLockRetry(
	ctx context.Context,
	operation, repoPath, branch string,
	execTimeout time.Duration,
	args ...string,
) refreshLockRetryResult {
	var result refreshLockRetryResult
	retryCtx := ctx
	var retryCancel context.CancelFunc
	defer func() {
		if retryCancel != nil {
			retryCancel()
		}
	}()

	for attempt := 1; attempt <= maxRefreshLockAttempts; attempt++ {
		if attempt > 1 {
			delay := refreshLockBackoff(attempt)
			if waitErr := waitRefreshLockBackoff(retryCtx, delay); waitErr != nil {
				result.runErr = waitErr
				result.execCtxErr = waitErr
				return result
			}
		}
		attemptResult := m.runRefreshGitAttempt(retryCtx, repoPath, execTimeout, args...)
		if attemptResult.admitted {
			result.attempts++
		}
		result.output = attemptResult.output
		result.runErr = attemptResult.runErr
		result.execCtxErr = attemptResult.execCtxErr
		if result.runErr == nil {
			return result
		}
		if !isRetryableRefreshLockFailure(string(result.output), result.execCtxErr, result.runErr) ||
			attempt == maxRefreshLockAttempts {
			break
		}
		if attempt == 1 {
			retryCtx, retryCancel = refreshLockRetryContext(ctx, attemptResult)
			if ctxErr := retryCtx.Err(); ctxErr != nil {
				result.runErr = ctxErr
				result.execCtxErr = ctxErr
				return result
			}
		}
		m.logger.Debug("retrying git refresh after lock contention",
			zap.String("operation", operation),
			zap.String("repository_path", repoPath),
			zap.String("branch", branch),
			zap.Int("attempt", attempt),
			zap.Int("next_attempt", attempt+1),
			zap.Duration("backoff", refreshLockBackoff(attempt+1)))
	}
	if isRetryableRefreshLockFailure(string(result.output), result.execCtxErr, result.runErr) {
		result.runErr = &refreshLockExhaustedError{
			operation: operation,
			attempts:  result.attempts,
			cause:     errors.Join(ErrGitCommandFailed, result.runErr),
		}
	}
	return result
}

func (m *Manager) runRefreshGitAttempt(
	ctx context.Context, repoPath string, execTimeout time.Duration, args ...string,
) refreshLockAttempt {
	var attempt refreshLockAttempt
	attempt.output, attempt.runErr, attempt.execCtxErr = subproc.RunGitCombinedAfterAcquire(
		ctx,
		subproc.GitLifecycle,
		execTimeout,
		func(execCtx context.Context) *exec.Cmd {
			attempt.deadline, attempt.hasDeadline = execCtx.Deadline()
			return m.newNonInteractiveGitCmd(execCtx, repoPath, args...)
		},
	)
	attempt.admitted = !errors.Is(attempt.runErr, subproc.ErrAdmissionCanceled)
	return attempt
}

// refreshLockRetryContext derives retries from the caller lifetime and the
// first post-admission deadline, never from the completed attempt's context.
func refreshLockRetryContext(ctx context.Context, attempt refreshLockAttempt) (context.Context, context.CancelFunc) {
	if attempt.hasDeadline {
		return context.WithDeadline(ctx, attempt.deadline)
	}
	return context.WithCancel(ctx)
}

func refreshLockBackoff(attempt int) time.Duration {
	if attempt <= 2 {
		return refreshLockInitialBackoff
	}
	return refreshLockInitialBackoff * time.Duration(1<<(attempt-2))
}

func waitRefreshLockBackoff(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// isRetryableRefreshLockFailure separates real contention causes from
// permission, read-only, namespace, and incidental lock-filename failures.
func isRetryableRefreshLockFailure(output string, contextErr, runErr error) bool {
	if classifyRefreshDiagnostic(output, runErr, contextErr).code != refreshDiagnosticLock {
		return false
	}
	for line := range strings.SplitSeq(strings.ToLower(output), "\n") {
		if isRetryableRefreshLockLine(line) {
			return true
		}
	}
	return false
}

func isRetryableRefreshLockLine(line string) bool {
	if strings.Contains(line, "another git process seems to be running") {
		return true
	}
	if strings.Contains(line, "file exists") &&
		(strings.Contains(line, "unable to create") ||
			strings.Contains(line, "cannot lock ref") ||
			strings.Contains(line, "index.lock")) {
		return true
	}
	// A concurrent ref update can lose Git's compare-and-swap check without an
	// existing lock file. The changed ref is safe to re-read on another attempt.
	return strings.Contains(line, "cannot lock ref") &&
		strings.Contains(line, " is at ") &&
		strings.Contains(line, " but expected ")
}
