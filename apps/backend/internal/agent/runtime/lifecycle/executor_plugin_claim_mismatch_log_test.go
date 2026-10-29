package lifecycle

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// A checkpoint refused because its claim echo differs from the active claim is
// logged with both claim ids, so the refusal is visible in the backend log.
func TestClaimCheckpointMismatchLogsExpectedAndReceivedClaim(t *testing.T) {
	manager, _, request := claimCheckpointFixture(t, declaresDestroyCheckpoints)
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	manager.logger = log
	request.Claim = claimTestSecondAcquisition

	if err := manager.checkpointPluginExecutorClaim(context.Background(), request, "cleanup_progress"); !errors.Is(err, models.ErrExecutorClaimNotFound) {
		t.Fatalf("error = %v, want ErrExecutorClaimNotFound", err)
	}
	entries := logs.FilterMessage("plugin executor checkpoint refused: claim mismatch").All()
	if len(entries) != 1 {
		t.Fatalf("claim mismatch log entries = %d, want 1 (all: %v)", len(entries), logs.All())
	}
	fields := entries[0].ContextMap()
	if fields["expected_claim"] != claimTestFirstAcquisition || fields["received_claim"] != claimTestSecondAcquisition {
		t.Fatalf("log fields = %v", fields)
	}
}
