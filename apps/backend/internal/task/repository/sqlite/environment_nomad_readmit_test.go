package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
)

func TestNomadWorkerQuarantineOperatorReleasedTransitions(t *testing.T) {
	for _, from := range []string{nomadPhaseCleanupPending, nomadPhaseQuarantined} {
		if !validNomadStateTransition(from, nomadPhaseOperatorReleased) {
			t.Fatalf("%s -> operator_released must be allowed", from)
		}
	}
	for _, from := range []string{"intent", "registered", "allocated", "ready", nomadPhaseAbsent} {
		if validNomadStateTransition(from, nomadPhaseOperatorReleased) {
			t.Fatalf("%s -> operator_released must be refused", from)
		}
	}
	for _, to := range []string{"intent", "ready", nomadPhaseCleanupPending, nomadPhaseQuarantined, nomadPhaseAbsent} {
		if validNomadStateTransition(nomadPhaseOperatorReleased, to) {
			t.Fatalf("operator_released -> %s must be refused", to)
		}
	}
	if !validNomadStateTransition(nomadPhaseOperatorReleased, nomadPhaseOperatorReleased) {
		t.Fatal("operator_released must be terminal and idempotent")
	}
}

func TestNomadWorkerQuarantineOperatorReleasedDoesNotBlockDelete(t *testing.T) {
	ctx := context.Background()
	fx := newPublishFixture(t)
	setNomadPhase(t, fx, nomadPhaseCleanupPending)
	fx.rewriteInventory(t, func(s *nomad.EnvironmentState) {
		s.Phase = nomadPhaseOperatorReleased
		s.OperatorAttestationDigest = strings.Repeat("c", 64)
	})
	if has, err := fx.repo.HasNomadEnvironmentInventory(ctx, publishEnvironmentID); err != nil || has {
		t.Fatalf("HasNomadEnvironmentInventory = %v, %v; want false", has, err)
	}
	if err := fx.repo.DeleteTaskEnvironment(ctx, publishEnvironmentID); err != nil {
		t.Fatalf("operator_released row must not block delete: %v", err)
	}
}
