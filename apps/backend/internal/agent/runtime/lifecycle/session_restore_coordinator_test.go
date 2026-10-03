package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestRestoreCoordinatorPreservesRuntimeConfiguration(t *testing.T) {
	var applied string
	var committed string
	var snapshotSaved bool
	coordinator := &RestoreCoordinator{Now: func() time.Time { return time.Unix(10, 0).UTC() }}
	result, err := coordinator.Restore(context.Background(), RestoreCoordinatorRequest{
		Identity: RestoreIdentity{
			SessionID:         "session-1",
			IncarnationID:     "incarnation-1",
			HarnessGeneration: 2,
			NativeSessionID:   "native-old",
		},
		Action:                RestoreActionContinueFromHistory,
		ExplicitAuthorization: true,
		TargetWorkspace:       "/workspace/new",
		Snapshot: &models.ContinuationSnapshot{
			Content:     "saved context",
			ContentHash: "hash",
		},
	}, RestoreCoordinatorHooks{
		LoadNative:   func(context.Context, string) error { return errors.New("Resource not found") },
		CreateNative: func(context.Context, string) (string, error) { return "native-new", nil },
		ApplyConfiguration: func(_ context.Context, id string) error {
			applied = id
			return nil
		},
		PersistAttempt: func(context.Context, *models.RestoreAttempt) error { return nil },
		PersistSnapshot: func(_ context.Context, snapshot *models.ContinuationSnapshot) error {
			snapshotSaved = snapshot.Status == models.ContinuitySnapshotPrepared
			return nil
		},
		CommitGeneration: func(_ context.Context, generation *models.HarnessSessionGeneration, expected int64) (bool, error) {
			committed = generation.NativeSessionID
			if expected != 2 || generation.Generation != 3 {
				t.Fatalf("generation commit = %+v, expected %d", generation, expected)
			}
			return true, nil
		},
		CompleteAttempt: func(context.Context, string, string, time.Time) error { return nil },
	})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if result.Decision.Outcome != RestoreOutcomeContextContinued || !result.DispatchAllowed {
		t.Fatalf("result = %+v, want authorized continuation", result)
	}
	if applied != "native-new" || committed != "native-new" || !snapshotSaved {
		t.Fatalf("configuration/commit order evidence = applied %q, committed %q, snapshot %v", applied, committed, snapshotSaved)
	}
}

func TestRestoreCoordinatorPartialFailureBlocksDispatch(t *testing.T) {
	committed := false
	result, err := (&RestoreCoordinator{}).Restore(context.Background(), RestoreCoordinatorRequest{
		Identity: RestoreIdentity{
			SessionID:         "session-2",
			IncarnationID:     "incarnation-1",
			HarnessGeneration: 0,
			NativeSessionID:   "native-old",
		},
		Action:                RestoreActionContinueFromHistory,
		ExplicitAuthorization: true,
		TargetWorkspace:       "/workspace/new",
		Snapshot:              &models.ContinuationSnapshot{Content: "saved context"},
	}, RestoreCoordinatorHooks{
		LoadNative:         func(context.Context, string) error { return errors.New("Resource not found") },
		CreateNative:       func(context.Context, string) (string, error) { return "native-new", nil },
		ApplyConfiguration: func(context.Context, string) error { return errors.New("permission denied") },
		PersistAttempt:     func(context.Context, *models.RestoreAttempt) error { return nil },
		PersistSnapshot:    func(context.Context, *models.ContinuationSnapshot) error { return nil },
		CommitGeneration: func(context.Context, *models.HarnessSessionGeneration, int64) (bool, error) {
			committed = true
			return true, nil
		},
		CompleteAttempt: func(context.Context, string, string, time.Time) error { return nil },
	})
	if err == nil {
		t.Fatal("Restore succeeded after configuration failure")
	}
	if result.DispatchAllowed || committed {
		t.Fatalf("partial restore admitted work: result=%+v committed=%v", result, committed)
	}
	if result.NativeSessionID != "native-old" {
		t.Fatalf("native identity changed after partial restore: %q", result.NativeSessionID)
	}
}
