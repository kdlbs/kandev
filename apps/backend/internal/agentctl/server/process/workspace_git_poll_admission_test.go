package process

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
)

func TestHandleGitPollFailureAdmissionCancellationDoesNotStopTracker(t *testing.T) {
	wt := &WorkspaceTracker{logger: newTestLogger(t), workDir: t.TempDir()}
	consecutiveFailures := 4
	cause := fmt.Errorf("queued poll: %w: %w", subproc.ErrAdmissionCanceled, context.DeadlineExceeded)

	if stop := wt.handleGitPollFailure(context.Background(), &consecutiveFailures, cause); stop {
		t.Fatal("admission cancellation stopped a healthy tracker")
	}
	if consecutiveFailures != 0 {
		t.Fatalf("consecutiveFailures = %d, want reset to 0 for admission cancellation", consecutiveFailures)
	}
}

func TestHandleGitPollFailureProbeAdmissionCancellationDoesNotStopTracker(t *testing.T) {
	restore := subproc.Git().SetCapForTest(1)
	t.Cleanup(restore)
	hold, err := subproc.AcquireGit(context.Background(), subproc.GitInteractive)
	if err != nil {
		t.Fatalf("hold Git slot: %v", err)
	}

	wt := &WorkspaceTracker{logger: newTestLogger(t), workDir: t.TempDir()}
	consecutiveFailures := 4
	cause := fmt.Errorf("snapshot failed")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan bool, 1)
	go func() {
		done <- wt.handleGitPollFailure(ctx, &consecutiveFailures, cause)
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := subproc.AdmissionSnapshot().Classes[string(subproc.GitBackground)].Waiters; got == 1 {
			break
		}
		runtime.Gosched()
	}
	if got := subproc.AdmissionSnapshot().Classes[string(subproc.GitBackground)].Waiters; got != 1 {
		t.Fatalf("background waiters = %d, want 1", got)
	}
	stop := <-done
	hold()
	if stop {
		t.Fatal("probe admission cancellation stopped a healthy tracker")
	}
	if consecutiveFailures != 0 {
		t.Fatalf("consecutiveFailures = %d, want reset to 0 for probe admission cancellation", consecutiveFailures)
	}
}
