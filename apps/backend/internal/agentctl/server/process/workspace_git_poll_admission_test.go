package process

import (
	"context"
	"fmt"
	"testing"

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
