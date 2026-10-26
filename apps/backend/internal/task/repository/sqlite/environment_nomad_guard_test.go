package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
)

func setNomadPhase(t *testing.T, fx *publishFixture, phase string) {
	t.Helper()
	fx.rewriteInventory(t, func(s *nomad.EnvironmentState) {
		s.Phase = phase
		if phase == nomadPhaseAbsent {
			s.DrainReceiptGeneration = s.Generation
			s.DrainReceiptDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		}
	})
}

func TestNomadCleanupEvidenceDeleteEnvironmentGuard(t *testing.T) {
	ctx := context.Background()
	for _, phase := range []string{"ready", nomadPhaseCleanupPending, nomadPhaseQuarantined} {
		t.Run(phase+" retains", func(t *testing.T) {
			fx := newPublishFixture(t)
			setNomadPhase(t, fx, phase)
			if err := fx.repo.DeleteTaskEnvironment(ctx, publishEnvironmentID); !errors.Is(err, ErrNomadInventoryRetained) {
				t.Fatalf("DeleteTaskEnvironment error = %v, want ErrNomadInventoryRetained", err)
			}
			if err := fx.repo.DeleteTaskEnvironmentsByTask(ctx, publishTaskID); !errors.Is(err, ErrNomadInventoryRetained) {
				t.Fatalf("DeleteTaskEnvironmentsByTask error = %v, want ErrNomadInventoryRetained", err)
			}
			if _, err := fx.repo.GetTaskEnvironment(ctx, publishEnvironmentID); err != nil {
				t.Fatalf("environment row must be retained: %v", err)
			}
			if has, err := fx.repo.HasNomadEnvironmentInventory(ctx, publishEnvironmentID); err != nil || !has {
				t.Fatalf("HasNomadEnvironmentInventory = %v, %v", has, err)
			}
		})
	}
}

func TestNomadCleanupEvidenceAbsentTombstoneDoesNotBlockDelete(t *testing.T) {
	ctx := context.Background()
	fx := newPublishFixture(t)
	setNomadPhase(t, fx, nomadPhaseCleanupPending)
	setNomadPhase(t, fx, nomadPhaseAbsent)
	if has, err := fx.repo.HasNomadEnvironmentInventory(ctx, publishEnvironmentID); err != nil || has {
		t.Fatalf("HasNomadEnvironmentInventory = %v, %v; want false", has, err)
	}
	if err := fx.repo.DeleteTaskEnvironment(ctx, publishEnvironmentID); err != nil {
		t.Fatalf("absent tombstone must not block delete: %v", err)
	}
}

func TestNomadCleanupEvidenceTaskDeleteCascadeRetainsOwner(t *testing.T) {
	ctx := context.Background()
	fx := newPublishFixture(t)
	if err := fx.repo.DeleteTask(ctx, publishTaskID); !errors.Is(err, ErrNomadInventoryRetained) {
		t.Fatalf("DeleteTask error = %v, want ErrNomadInventoryRetained", err)
	}
	if _, err := fx.repo.GetTaskSession(ctx, publishSessionID); err != nil {
		t.Fatalf("owner session must be retained: %v", err)
	}
	if _, err := fx.repo.GetTaskEnvironment(ctx, publishEnvironmentID); err != nil {
		t.Fatalf("owner environment must be retained: %v", err)
	}
	setNomadPhase(t, fx, nomadPhaseCleanupPending)
	setNomadPhase(t, fx, nomadPhaseAbsent)
	if err := fx.repo.DeleteTask(ctx, publishTaskID); err != nil {
		t.Fatalf("absent inventory must allow task delete: %v", err)
	}
}

func TestNomadCleanupEvidenceRunningRowDeleteGuard(t *testing.T) {
	ctx := context.Background()
	for _, phase := range []string{"ready", nomadPhaseCleanupPending, nomadPhaseQuarantined} {
		t.Run(phase+" retains", func(t *testing.T) {
			fx := newPublishFixture(t)
			setNomadPhase(t, fx, phase)
			if err := fx.repo.DeleteExecutorRunningBySessionID(ctx, publishSessionID); !errors.Is(err, ErrNomadInventoryRetained) {
				t.Fatalf("DeleteExecutorRunningBySessionID error = %v, want ErrNomadInventoryRetained", err)
			}
			running, err := fx.repo.GetExecutorRunningBySessionID(ctx, publishSessionID)
			if err != nil {
				t.Fatalf("retained row must survive: %v", err)
			}
			err = fx.repo.DeleteExecutorRunningIfCurrent(ctx, publishSessionID, running.AgentExecutionID, running.UpdatedAt)
			if !errors.Is(err, ErrNomadInventoryRetained) {
				t.Fatalf("DeleteExecutorRunningIfCurrent error = %v, want ErrNomadInventoryRetained", err)
			}
		})
	}
	t.Run("absent tombstone is deletable", func(t *testing.T) {
		fx := newPublishFixture(t)
		setNomadPhase(t, fx, nomadPhaseCleanupPending)
		setNomadPhase(t, fx, nomadPhaseAbsent)
		if err := fx.repo.DeleteExecutorRunningBySessionID(ctx, publishSessionID); err != nil {
			t.Fatalf("absent tombstone must be deletable: %v", err)
		}
	})
}
