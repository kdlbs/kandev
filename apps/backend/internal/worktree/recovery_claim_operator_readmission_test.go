package worktree

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestRecoveryClaimOperatorReadmissionPassesActiveCleanupBarrier(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	store.seedSessionWithEnvironment(t, "session-readmit", "task-readmit")
	now := time.Now().UTC()
	if _, err := store.db.ExecContext(ctx, store.db.Rebind(`
		INSERT INTO task_resource_cleanup_jobs (id, operation_id, task_id, trigger, state, attempts, created_at, updated_at)
		VALUES ('job-readmit', 'operation-job-readmit', 'task-readmit', 'cascade_archive', ?, 1, ?, ?)
	`), models.TaskResourceCleanupStateRetryWait, now, now); err != nil {
		t.Fatalf("seed retrying cleanup job: %v", err)
	}
	request := models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: "env-session-readmit", OwnerTaskID: "task-readmit", OwnershipGeneration: 1,
		SessionID: "session-readmit", OperationID: "operation-readmit", ExecutorType: string(models.ExecutorTypeWorktree),
	}

	if _, err := store.AcquireTaskEnvironmentRecoveryClaim(ctx, request); !errors.Is(err, repoerrors.ErrTaskCleanupInProgress) {
		t.Fatalf("ordinary claim err = %v, want ErrTaskCleanupInProgress", err)
	}
	request.OperatorReadmission = true
	claim, err := store.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
	if err != nil {
		t.Fatalf("operator readmission claim: %v", err)
	}
	defer func() { _ = store.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) }()

	// A running cleanup attempt still excludes the readmission through the per-environment claim.
	other := request
	other.OperationID = "operation-cleanup-attempt"
	if _, err := store.AcquireTaskEnvironmentRecoveryClaim(ctx, other); !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("competing claim err = %v, want recoveryclaim.ErrBusy", err)
	}
}
