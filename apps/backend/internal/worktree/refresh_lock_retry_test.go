package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/subproc"
)

const refreshLockFixtureOutput = "fatal: Unable to create '.git/refs/heads/main.lock': File exists.\n"

func writeRefreshLockFakeGit(t *testing.T, body string) (callCountPath string) {
	t.Helper()
	return writeRefreshLockFakeGitForCommand(t, "", body)
}

// writeRefreshLockFakeGitForCommand counts only invocations of command, so an
// operation-specific retry can include unrelated inspection commands.
func writeRefreshLockFakeGitForCommand(t *testing.T, command, body string) (callCountPath string) {
	t.Helper()

	callCountPath = filepath.Join(t.TempDir(), "calls")
	t.Setenv("KD_LOCK_CALLS", callCountPath)
	t.Setenv("KD_LOCK_COUNT_COMMAND", command)
	scriptDir := writeFakeGitScript(t, `
count_file="${KD_LOCK_CALLS:?}"
count=0
if [ -f "$count_file" ]; then
  count=$(cat "$count_file")
fi
if [ -z "${KD_LOCK_COUNT_COMMAND:-}" ] || [ "${1:-}" = "${KD_LOCK_COUNT_COMMAND}" ]; then
  count=$((count + 1))
  printf '%s' "$count" > "$count_file"
fi
`+body)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return callCountPath
}

func refreshLockCallCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0
		}
		t.Fatalf("read call count: %v", err)
	}
	var count int
	if _, err := fmt.Sscanf(string(data), "%d", &count); err != nil {
		t.Fatalf("parse call count %q: %v", data, err)
	}
	return count
}

func runRefreshLockHelper(
	t *testing.T, mgr *Manager, ctx context.Context, operation string, timeout time.Duration,
) refreshLockRetryResult {
	t.Helper()
	return mgr.runRefreshGitWithLockRetry(
		ctx, operation, t.TempDir(), "main", timeout, operation, "--dry-run",
	)
}

// onRefreshRetryCore runs a callback exactly when the helper enters backoff.
// The log write happens before the timer starts, so test state changes are
// deterministic and do not depend on subprocess or scheduler timing.
type onRefreshRetryCore struct {
	zapcore.Core
	onRetry func()
}

func (c *onRefreshRetryCore) With(fields []zapcore.Field) zapcore.Core {
	return &onRefreshRetryCore{Core: c.Core.With(fields), onRetry: c.onRetry}
}

func (c *onRefreshRetryCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

func (c *onRefreshRetryCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	if entry.Message == "retrying git refresh after lock contention" {
		c.onRetry()
	}
	return c.Core.Write(entry, fields)
}

func newOnRetryManager(t *testing.T, onRetry func()) *Manager {
	t.Helper()
	core, _ := observer.New(zapcore.DebugLevel)
	log, err := commonlogger.NewFromZap(zap.New(&onRefreshRetryCore{Core: core, onRetry: onRetry}))
	if err != nil {
		t.Fatalf("create observed logger: %v", err)
	}
	mgr, err := NewManager(newTestConfig(t), newMockStore(), log)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return mgr
}

func waitForRefreshLockWaiter(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if snapshot := subproc.AdmissionSnapshot(); snapshot.Classes[string(subproc.GitLifecycle)].Waiters > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("refresh helper did not queue for Git admission")
}

func receiveRefreshLockResult(t *testing.T, results <-chan refreshLockRetryResult) refreshLockRetryResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("refresh helper did not finish")
		return refreshLockRetryResult{}
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.29
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.32
func TestRefreshLockRetryTransientFailures(t *testing.T) {
	t.Run("fetch retries once", func(t *testing.T) {
		calls := writeRefreshLockFakeGit(t, `
if [ "${1:-}" = "fetch" ] && [ "$count" -eq 1 ]; then
  echo "`+refreshLockFixtureOutput+`" >&2
  exit 128
fi
exit 0
`)
		mgr := newRecreateTestManager(t)
		result := runRefreshLockHelper(t, mgr, context.Background(), "fetch", 5*time.Second)
		if result.runErr != nil {
			t.Fatalf("runRefreshGitWithLockRetry() error = %v", result.runErr)
		}
		if result.attempts != 2 || refreshLockCallCount(t, calls) != 2 {
			t.Fatalf("attempts = %d, calls = %d, want both 2", result.attempts, refreshLockCallCount(t, calls))
		}
	})

	t.Run("pull retries once", func(t *testing.T) {
		calls := writeRefreshLockFakeGitForCommand(t, "pull", `
if [ "${1:-}" = "rev-parse" ]; then
  echo "main"
  exit 0
fi
if [ "${1:-}" = "pull" ] && [ "$count" -eq 1 ]; then
  echo "`+refreshLockFixtureOutput+`" >&2
  exit 128
fi
exit 0
`)
		mgr := newRecreateTestManager(t)
		result := runRefreshLockHelper(t, mgr, context.Background(), "pull", 5*time.Second)
		if result.runErr != nil {
			t.Fatalf("runRefreshGitWithLockRetry() error = %v", result.runErr)
		}
		if result.attempts != 2 || refreshLockCallCount(t, calls) != 2 {
			t.Fatalf("attempts = %d, calls = %d, want both 2", result.attempts, refreshLockCallCount(t, calls))
		}
	})

	t.Run("wrapped ref lock retries once", func(t *testing.T) {
		calls := writeRefreshLockFakeGit(t, `
if [ "${1:-}" = "fetch" ] && [ "$count" -eq 1 ]; then
  echo "error: cannot lock ref 'refs/remotes/origin/main': Unable to create '.git/refs/remotes/origin/main.lock': File exists." >&2
  exit 128
fi
exit 0
`)
		mgr := newRecreateTestManager(t)
		result := runRefreshLockHelper(t, mgr, context.Background(), "fetch", 5*time.Second)
		if result.runErr != nil {
			t.Fatalf("runRefreshGitWithLockRetry() error = %v", result.runErr)
		}
		if result.attempts != 2 || refreshLockCallCount(t, calls) != 2 {
			t.Fatalf("attempts = %d, calls = %d, want both 2", result.attempts, refreshLockCallCount(t, calls))
		}
	})

	t.Run("ref compare-and-swap race retries once", func(t *testing.T) {
		calls := writeRefreshLockFakeGit(t, `
if [ "${1:-}" = "fetch" ] && [ "$count" -eq 1 ]; then
  echo "error: cannot lock ref 'refs/remotes/origin/main': is at 1111111 but expected 2222222" >&2
  exit 128
fi
exit 0
`)
		mgr := newRecreateTestManager(t)
		result := runRefreshLockHelper(t, mgr, context.Background(), "fetch", 5*time.Second)
		if result.runErr != nil {
			t.Fatalf("runRefreshGitWithLockRetry() error = %v", result.runErr)
		}
		if result.attempts != 2 || refreshLockCallCount(t, calls) != 2 {
			t.Fatalf("attempts = %d, calls = %d, want both 2", result.attempts, refreshLockCallCount(t, calls))
		}
	})

	t.Run("first attempt success has no retry", func(t *testing.T) {
		calls := writeRefreshLockFakeGit(t, "exit 0\n")
		mgr := newRecreateTestManager(t)
		result := runRefreshLockHelper(t, mgr, context.Background(), "fetch", 5*time.Second)
		if result.runErr != nil || result.attempts != 1 {
			t.Fatalf("result = %#v, want one successful attempt", result)
		}
		if got := refreshLockCallCount(t, calls); got != 1 {
			t.Fatalf("calls = %d, want 1", got)
		}
	})
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.29
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.32
func TestRefreshLockRetryNonLockFailures(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantCalls int
		wantText  string
	}{
		{
			name: "permission denied lock creation",
			body: `
echo "fatal: Unable to create '.git/index.lock': Permission denied" >&2
exit 128
`,
			wantCalls: 1,
		},
		{
			name: "wrapped ref lock permission denied",
			body: `
echo "error: cannot lock ref 'refs/remotes/origin/main': Unable to create '.git/refs/remotes/origin/main.lock': Permission denied" >&2
exit 128
`,
			wantCalls: 1,
		},
		{
			name: "read-only lock creation",
			body: `
echo "fatal: Unable to create '.git/index.lock': Read-only file system" >&2
exit 128
`,
			wantCalls: 1,
		},
		{
			name: "ref namespace conflict",
			body: `
echo "error: cannot lock ref 'refs/heads/feature': 'refs/heads/feature/child' exists; cannot create 'refs/heads/feature'" >&2
exit 128
`,
			wantCalls: 1,
		},
		{
			name: "authentication failure",
			body: `
echo "fatal: Authentication failed for 'https://user:secret@example.invalid/repo.git'" >&2
exit 128
`,
			wantCalls: 1,
		},
		{
			name: "lock and authentication failure in one output",
			body: `
echo "` + refreshLockFixtureOutput + `" >&2
echo "fatal: Authentication failed for 'https://user:secret@example.invalid/repo.git'" >&2
exit 128
`,
			wantCalls: 1,
		},
		{
			name: "invalid local ref",
			body: `
echo "fatal: bad ref refs/heads/main" >&2
exit 128
`,
			wantCalls: 1,
		},
		{
			name: "lock filename in remote URL is not contention",
			body: `
echo "fatal: unable to access 'https://example.invalid/index.lock/repo.git': The requested URL returned error: 403" >&2
exit 128
`,
			wantCalls: 1,
		},
		{
			name: "non-lock failure after a lock failure",
			body: `
if [ "$count" -eq 1 ]; then
  echo "` + refreshLockFixtureOutput + `" >&2
  exit 128
fi
echo "fatal: Authentication failed" >&2
exit 128
`,
			wantCalls: 2,
			wantText:  "Authentication failed",
		},
		{
			name: "wrapped ref permission failure after a lock failure",
			body: `
if [ "$count" -eq 1 ]; then
  echo "` + refreshLockFixtureOutput + `" >&2
  exit 128
fi
echo "error: cannot lock ref 'refs/remotes/origin/main': Unable to create '.git/refs/remotes/origin/main.lock': Permission denied" >&2
exit 128
`,
			wantCalls: 2,
			wantText:  "Permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := writeRefreshLockFakeGit(t, tt.body)
			mgr := newRecreateTestManager(t)
			result := runRefreshLockHelper(t, mgr, context.Background(), "fetch", 5*time.Second)
			if result.runErr == nil {
				t.Fatal("runRefreshGitWithLockRetry() error = nil, want a failure")
			}
			if got := refreshLockCallCount(t, calls); got != tt.wantCalls {
				t.Fatalf("calls = %d, want %d", got, tt.wantCalls)
			}
			if tt.wantText != "" && !strings.Contains(string(result.output), tt.wantText) {
				t.Fatalf("output = %q, want %q", result.output, tt.wantText)
			}
			var exhausted *refreshLockExhaustedError
			if errors.As(result.runErr, &exhausted) {
				t.Fatalf("non-lock failure was replaced by an exhausted lock error: %v", result.runErr)
			}
			var exitErr *exec.ExitError
			if !errors.As(result.runErr, &exitErr) {
				t.Fatalf("error = %v, want the original Git process error", result.runErr)
			}
		})
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.30
// @covers AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.3
// @covers AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.4
func TestRefreshLockRetryBudget(t *testing.T) {
	t.Run("queued admission does not consume the execution budget", func(t *testing.T) {
		calls := writeRefreshLockFakeGit(t, `
if [ "${1:-}" = "fetch" ] && [ "$count" -eq 1 ]; then
  echo "`+refreshLockFixtureOutput+`" >&2
  exit 128
fi
exit 0
`)
		restore := subproc.Git().SetCapForTest(1)
		defer restore()

		ctx := context.Background()
		release, err := subproc.Git().AcquireClass(ctx, subproc.GitLifecycle)
		if err != nil {
			t.Fatalf("hold Git admission slot: %v", err)
		}
		defer release()

		mgr := newRecreateTestManager(t)
		repoPath := t.TempDir()
		results := make(chan refreshLockRetryResult, 1)
		go func() {
			results <- mgr.runRefreshGitWithLockRetry(
				ctx, "fetch", repoPath, "main", 400*time.Millisecond, "fetch", "--dry-run",
			)
		}()
		waitForRefreshLockWaiter(t)
		if got := refreshLockCallCount(t, calls); got != 0 {
			t.Fatalf("calls while queued = %d, want the command to wait for admission", got)
		}

		// Hold the slot longer than the execution budget. The first command
		// must still receive its full post-admission budget.
		time.Sleep(700 * time.Millisecond)
		release()
		result := receiveRefreshLockResult(t, results)
		if result.runErr != nil {
			t.Fatalf("runRefreshGitWithLockRetry() error = %v, want recovery after queued admission", result.runErr)
		}
		if result.attempts != 2 || refreshLockCallCount(t, calls) != 2 {
			t.Fatalf("attempts = %d, calls = %d, want both 2",
				result.attempts, refreshLockCallCount(t, calls))
		}
		snapshot := subproc.AdmissionSnapshot()
		if snapshot.Inflight != 0 || snapshot.Waiters != 0 {
			t.Fatalf("admission inflight/waiters = %d/%d, want released", snapshot.Inflight, snapshot.Waiters)
		}
	})

	t.Run("retries share the first admitted deadline", func(t *testing.T) {
		calls := writeRefreshLockFakeGit(t, `
echo "`+refreshLockFixtureOutput+`" >&2
exit 128
`)
		restore := subproc.Git().SetCapForTest(1)
		defer restore()

		mgr := newRecreateTestManager(t)
		result := runRefreshLockHelper(t, mgr, context.Background(), "fetch", 300*time.Millisecond)
		if !errors.Is(result.runErr, context.DeadlineExceeded) {
			t.Fatalf("error = %v, want context.DeadlineExceeded", result.runErr)
		}
		if result.attempts >= maxRefreshLockAttempts {
			t.Fatalf("attempts = %d, want the first admitted deadline to stop retries", result.attempts)
		}
		if got := refreshLockCallCount(t, calls); got >= maxRefreshLockAttempts {
			t.Fatalf("calls = %d, want fewer than %d within one execution budget", got, maxRefreshLockAttempts)
		}
		if snapshot := subproc.AdmissionSnapshot(); snapshot.Inflight != 0 {
			t.Fatalf("admission inflight = %d, want the slot released", snapshot.Inflight)
		}
	})
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.30
// @covers AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.3
// @covers AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.4
func TestRefreshLockRetryCancellation(t *testing.T) {
	calls := writeRefreshLockFakeGit(t, `
echo "`+refreshLockFixtureOutput+`" >&2
exit 128
`)
	restore := subproc.Git().SetCapForTest(1)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The hook cancels when the helper logs its backoff entry, so the test
	// always observes cancellation in the intended state.
	mgr := newOnRetryManager(t, cancel)
	repoPath := t.TempDir()
	results := make(chan refreshLockRetryResult, 1)
	go func() {
		results <- mgr.runRefreshGitWithLockRetry(
			ctx, "fetch", repoPath, "main", 5*time.Second, "fetch", "--dry-run",
		)
	}()
	result := receiveRefreshLockResult(t, results)
	if !errors.Is(result.runErr, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", result.runErr)
	}
	if result.attempts != 1 {
		t.Fatalf("attempts = %d, want the cancelled first attempt", result.attempts)
	}
	if got := refreshLockCallCount(t, calls); got != 1 {
		t.Fatalf("calls = %d, want no command after cancellation", got)
	}
	if snapshot := subproc.AdmissionSnapshot(); snapshot.Inflight != 0 {
		t.Fatalf("admission inflight = %d, want the slot released", snapshot.Inflight)
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.30
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.32
func TestRefreshLockRetryAbortsOnCheckoutSwitch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX Git wrapper fixture is unavailable on Windows")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("resolve real git: %v", err)
	}
	repoPath, _ := initGitRepoWithRemoteAhead(t)
	runGit(t, repoPath, "branch", "other")

	calls := filepath.Join(t.TempDir(), "calls")
	t.Setenv("KD_REAL_GIT", realGit)
	t.Setenv("KD_LOCK_CALLS", calls)
	scriptDir := writeFakeGitScript(t, `
if [ "${1:-}" = "pull" ]; then
  count_file="${KD_LOCK_CALLS:?}"
  count=0
  if [ -f "$count_file" ]; then
    count=$(cat "$count_file")
  fi
  count=$((count + 1))
  printf '%s' "$count" > "$count_file"
  echo "`+refreshLockFixtureOutput+`" >&2
  exit 128
fi
exec "$KD_REAL_GIT" "$@"
`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var switchErr error
	mgr := newOnRetryManager(t, func() {
		cmd := exec.Command(realGit, "checkout", "--quiet", "other")
		cmd.Dir = repoPath
		switchErr = cmd.Run()
	})
	result := mgr.runRefreshGitWithLockRetry(
		context.Background(), "pull", repoPath, "main", 5*time.Second,
		"pull", "--ff-only", "origin", "main",
	)
	if switchErr != nil {
		t.Fatalf("switch checkout during backoff: %v", switchErr)
	}
	if result.runErr == nil || !strings.Contains(result.runErr.Error(), "checkout moved") {
		t.Fatalf("error = %v, want a checkout-switch abort", result.runErr)
	}
	if result.attempts != 1 {
		t.Fatalf("attempts = %d, want no pull attempt on another branch", result.attempts)
	}
	if got := refreshLockCallCount(t, calls); got != 1 {
		t.Fatalf("pull calls = %d, want 1", got)
	}
	if branch := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "--abbrev-ref", "HEAD")); branch != "other" {
		t.Fatalf("checkout branch = %q, want other", branch)
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.33
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.14
// @covers AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.5
func TestRefreshLockRetryExhaustion(t *testing.T) {
	calls := writeRefreshLockFakeGitForCommand(t, "pull", `
if [ "${1:-}" = "rev-parse" ]; then
  echo "main"
  exit 0
fi
echo "`+refreshLockFixtureOutput+`" >&2
exit 128
`)
	mgr := newRecreateTestManager(t)
	result := runRefreshLockHelper(t, mgr, context.Background(), "pull", 5*time.Second)
	if result.runErr == nil {
		t.Fatal("runRefreshGitWithLockRetry() error = nil, want lock exhaustion")
	}
	if result.attempts != maxRefreshLockAttempts {
		t.Fatalf("attempts = %d, want %d", result.attempts, maxRefreshLockAttempts)
	}
	if got := refreshLockCallCount(t, calls); got != maxRefreshLockAttempts {
		t.Fatalf("calls = %d, want %d", got, maxRefreshLockAttempts)
	}
	var exhausted *refreshLockExhaustedError
	if !errors.As(result.runErr, &exhausted) {
		t.Fatalf("error = %v, want *refreshLockExhaustedError", result.runErr)
	}
	if exhausted.operation != "pull" || exhausted.attempts != maxRefreshLockAttempts {
		t.Fatalf("exhausted error = %#v, want pull after %d attempts", exhausted, maxRefreshLockAttempts)
	}
	if !errors.Is(result.runErr, ErrGitCommandFailed) {
		t.Fatalf("error = %v, want it to preserve ErrGitCommandFailed", result.runErr)
	}
	var exitErr *exec.ExitError
	if !errors.As(result.runErr, &exitErr) || exitErr.ExitCode() != 128 {
		t.Fatalf("error = %v, want the final Git process error preserved", result.runErr)
	}
	text := result.runErr.Error()
	for _, want := range []string{"lock contention", "pull", "4 attempts"} {
		if !strings.Contains(text, want) {
			t.Fatalf("error = %q, want %q", text, want)
		}
	}
	for _, forbidden := range []string{"File exists", "another Git process", "Permission denied"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("error = %q, want fixed ownership-neutral wording without %q", text, forbidden)
		}
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.33
// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.14
// @covers AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.5
func TestRefreshLockRetrySafeDiagnostics(t *testing.T) {
	scriptDir := writeFakeGitScript(t, `
case "${1:-}" in
  rev-parse)
    exit 0
    ;;
  fetch)
    echo "error: cannot lock ref 'refs/remotes/origin/main': Unable to create '.git/refs/remotes/origin/main.lock': File exists." >&2
    echo "fatal: unable to access 'https://user:super-secret@example.invalid/repo.git'" >&2
    exit 128
    ;;
  *)
    exit 0
    ;;
esac
`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	log, logs := newObservedWorktreeLogger(t)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), log)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	var events []SyncProgressEvent
	_, _, err = mgr.pullBaseBranchWithPolicy(
		context.Background(), t.TempDir(), "main", "", true, captureSyncProgress(&events),
	)
	if err == nil {
		t.Fatal("pullBaseBranchWithPolicy() error = nil, want persistent lock failure")
	}
	if !errors.Is(err, ErrGitCommandFailed) {
		t.Fatalf("error = %v, want ErrGitCommandFailed", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want a lock failure not a timeout", err)
	}
	if text := err.Error(); !strings.Contains(text, "lock contention") || !strings.Contains(text, "4 attempts") {
		t.Fatalf("error = %q, want lock contention and attempt count", text)
	}
	for _, secret := range []string{"super-secret", "https://"} {
		if strings.Contains(err.Error(), secret) || strings.Contains(fmt.Sprint(events), secret) {
			t.Fatalf("error or progress exposed %q: error=%v events=%#v", secret, err, events)
		}
	}

	entries := logs.FilterMessage("git refresh failed").All()
	if len(entries) != 1 {
		t.Fatalf("observed %d refresh diagnostics, want 1; logs = %v", len(entries), logs.All())
	}
	fields := entries[0].ContextMap()
	if fields["operation"] != "fetch" || fields["branch"] != "main" {
		t.Fatalf("diagnostic fields = %v, want fetch/main", fields)
	}
	if fields["diagnostic_code"] != refreshDiagnosticLock {
		t.Fatalf("diagnostic_code = %v, want %q", fields["diagnostic_code"], refreshDiagnosticLock)
	}
	if fields["attempts"] != int64(maxRefreshLockAttempts) && fields["attempts"] != maxRefreshLockAttempts {
		t.Fatalf("attempts = %v, want %d", fields["attempts"], maxRefreshLockAttempts)
	}
	for _, entry := range logs.All() {
		rendered := fmt.Sprint(entry.ContextMap())
		if strings.Contains(rendered, "super-secret") || strings.Contains(entry.Message, "super-secret") {
			t.Fatalf("log exposed credentials: %#v", entry)
		}
	}
}
