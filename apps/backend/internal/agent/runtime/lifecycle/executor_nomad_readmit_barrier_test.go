package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// cleanupBarrierStore models an archive cleanup job parked in retry_wait: every
// claim except an operator readmission is refused while the job is active.
type cleanupBarrierStore struct {
	*readmitStoreFake
	requests []models.TaskEnvironmentRecoveryClaimRequest
}

func (f *cleanupBarrierStore) AcquireTaskEnvironmentRecoveryClaim(
	ctx context.Context, req models.TaskEnvironmentRecoveryClaimRequest,
) (*models.TaskEnvironmentRecoveryClaim, error) {
	f.requests = append(f.requests, req)
	if !req.OperatorReadmission && req.CleanupJobID == "" {
		return nil, fmt.Errorf("%w: task-1", repoerrors.ErrTaskCleanupInProgress)
	}
	return f.readmitStoreFake.AcquireTaskEnvironmentRecoveryClaim(ctx, req)
}

func TestNomadWorkerQuarantineReadmitProceedsWhileArchiveCleanupJobIsRetrying(t *testing.T) {
	manager, base := newReadmitHarness(t, "quarantined")
	store := &cleanupBarrierStore{readmitStoreFake: base}
	manager.runningWriter = store

	if err := manager.ReadmitNomadEnvironment(context.Background(), readmitRequest()); err != nil {
		t.Fatalf("readmit while the cleanup job retries: %v", err)
	}
	if len(store.requests) != 1 || !store.requests[0].OperatorReadmission {
		t.Fatalf("readmit claim requests = %+v, want one operator readmission", store.requests)
	}
	state, err := nomad.EnvironmentStateFromMetadata(base.row.Metadata)
	if err != nil || state.Phase != "operator_released" || base.deletes != 0 {
		t.Fatalf("row = %+v err=%v deletes=%d, want the retained row operator_released", state, err, base.deletes)
	}
	// The retrying job finds nothing left to destroy and finishes without touching Nomad.
	records, err := manager.nomadRecordsForEnvironment(context.Background(), "environment-1")
	if err != nil || len(records) != 0 {
		t.Fatalf("cleanup job would still destroy the released row: %d records, %v", len(records), err)
	}
	if store.claimsTaken != 1 || store.claimsClosed != 1 {
		t.Fatalf("claims taken/released = %d/%d, want 1/1", store.claimsTaken, store.claimsClosed)
	}
}

func TestNomadWorkerQuarantineReadmitReportsABusyClaimAsRetryable(t *testing.T) {
	manager, store := newReadmitHarness(t, "quarantined")
	store.claimErr = fmt.Errorf("%w: environment claimed by operation x", recoveryclaim.ErrBusy)
	err := manager.ReadmitNomadEnvironment(context.Background(), readmitRequest())
	if !errors.Is(err, nomad.ErrNomadReadmitBusy) {
		t.Fatalf("err = %v, want nomad.ErrNomadReadmitBusy", err)
	}
	if store.checkpoints != 0 {
		t.Fatal("a refused claim checkpointed the row")
	}
}
