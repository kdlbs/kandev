package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/processidentity"
)

func TestRecoveryPayloadPreservesOriginalRuntimeProof(t *testing.T) {
	process, err := processidentity.Capture(os.Getpid())
	if err != nil {
		t.Skip(err)
	}
	owner := agentctl.NewRuntimeOwner(nil, newTestLogger(), "boot")
	t.Cleanup(owner.Stop)
	candidate, err := owner.PrepareBinding()
	if err != nil {
		t.Fatal(err)
	}
	if err = candidate.SetProcessIdentity(process); err != nil {
		t.Fatal(err)
	}
	if err = candidate.Configure("127.0.0.1", 1234, "secret", process.PID, nil); err != nil {
		t.Fatal(err)
	}
	if err = candidate.Commit(); err != nil {
		t.Fatal(err)
	}
	lease, err := owner.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	manager, eventBus := createTestManagerWithTracking()
	t.Cleanup(func() { _ = manager.Stop() })
	execution := createTestExecution("execution", "task", "session")
	execution.agentctl = lease.NewBoundInstanceClient(1235, newTestLogger())
	execution.DeliveryMode = DurableDeliveryV1
	execution.DeliveryStreamID = "stream"
	execution.DeliveryIncarnationID = "session"
	execution.DeliveryHarnessGeneration = 1
	execution.promptGeneration = 1
	execution.runtimeEpoch = lease.Epoch()
	execution.setDeliverySubmissionID("prompt")
	identity, err := captureDeliveryReconciliationIdentity(execution)
	if err != nil {
		t.Fatal(err)
	}
	manager.eventPublisher.PublishAgentctlDeliveryRecovery(context.Background(), execution, identity, DeliveryReconciliationPhaseUncertain)
	raw, err := json.Marshal(eventBus.PublishedEvents[0].Event.Data)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		OriginalRuntime processidentity.Identity `json:"original_runtime"`
	}
	if err = json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.OriginalRuntime != process {
		t.Fatalf("original runtime proof=%+v, want %+v", payload.OriginalRuntime, process)
	}
}
