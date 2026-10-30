package lifecycle

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
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

	if err := manager.CheckpointExecutorResource(context.Background(), request); !errors.Is(err, models.ErrExecutorClaimNotFound) {
		t.Fatalf("error = %v, want ErrExecutorClaimNotFound", err)
	}
	entries := logs.FilterMessage("plugin executor checkpoint refused").All()
	if len(entries) != 1 {
		t.Fatalf("claim mismatch log entries = %d, want 1 (all: %v)", len(entries), logs.All())
	}
	fields := entries[0].ContextMap()
	if fields["expected_claim"] != claimTestFirstAcquisition || fields["received_claim"] != claimTestSecondAcquisition {
		t.Fatalf("log fields = %v", fields)
	}
}

// A launch-phase checkpoint refusal is logged with its cause as well.
func TestLaunchCheckpointRefusalIsLogged(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{logger: log}
	request := &pluginsdk.CheckpointExecutorResourceRequest{Phase: "provisioned", Context: &pluginsdk.ExecutorProviderRequestContext{ExecutionId: "execution-x", EnvironmentId: "environment-x"}}

	if err := manager.CheckpointExecutorResource(context.Background(), request); err == nil {
		t.Fatal("a checkpoint without a lease must be refused")
	}
	entries := logs.FilterMessage("plugin executor checkpoint refused").All()
	if len(entries) != 1 || entries[0].ContextMap()["execution_id"] != "execution-x" || entries[0].ContextMap()["error"] == nil {
		t.Fatalf("log entries = %v", logs.All())
	}
}
