package lifecycle

import (
	"context"
	"sync"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.17
func TestDeliveryStorageRejectsStaleIdlePressureAfterSuccessorStarts(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*AgentExecution)
	}{
		{name: "changed submission", mutate: func(execution *AgentExecution) {
			execution.setDeliverySubmissionID("submission-2")
		}},
		{name: "changed prompt generation", mutate: func(execution *AgentExecution) {
			execution.promptGeneration++
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
			manager := pressureAgeTestManager(&now)
			manager.eventPublisher = &EventPublisher{}
			execution := pressureAgeTestExecution()
			execution.setDeliverySubmissionID("submission-1")
			client := agentctl.NewClient("127.0.0.1", 1, newTestLogger())
			t.Cleanup(client.Close)
			execution.replaceAgentctlClient(client)

			initial := RuntimeStoragePressure{
				WritePressure: true, ProducerPaused: true, DeliveryState: deliveryFailedHealthState,
				FailureStorage: &models.AgentErrorStorage{Reason: models.AgentStorageReasonDiskSpaceLow},
				SubmissionID:   "submission-1", PromptGeneration: execution.promptGenerationSnapshot(),
				SubmissionCaptured: true, Client: client,
			}
			manager.applyRuntimeStoragePressure(context.Background(), execution, initial)
			before, ok := manager.RuntimeStorageObservation("session")
			if !ok || !before.WritePressure {
				t.Fatalf("initial observation = %+v, ok = %v", before, ok)
			}

			entered := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			manager.runtimeStorageClock = func() time.Time {
				once.Do(func() { close(entered) })
				<-release
				return now
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				manager.applyRuntimeStoragePressure(context.Background(), execution, RuntimeStoragePressure{
					WritePressure: false, DeliveryState: deliveryHealthyHealthState,
					SubmissionID: "submission-1", PromptGeneration: initial.PromptGeneration,
					SubmissionCaptured: true, Client: client,
				})
			}()
			<-entered
			test.mutate(execution)
			close(release)
			<-done

			after, ok := manager.RuntimeStorageObservation("session")
			if !ok {
				t.Fatal("stale successor observation removed the pressure cache")
			}
			if !after.WritePressure || after.PressureReceivedAt != before.PressureReceivedAt ||
				after.Revision != before.Revision {
				t.Fatalf("stale idle observation changed successor pressure: before=%+v after=%+v", before, after)
			}
		})
	}
}

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.17
func TestDeliveryStorageRejectsPressureForReplacedClient(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	manager := pressureAgeTestManager(&now)
	manager.eventPublisher = &EventPublisher{}
	execution := pressureAgeTestExecution()
	execution.setDeliverySubmissionID("submission-1")
	captured := agentctl.NewClient("127.0.0.1", 1, newTestLogger())
	replacement := agentctl.NewClient("127.0.0.1", 2, newTestLogger())
	t.Cleanup(captured.Close)
	t.Cleanup(replacement.Close)
	execution.replaceAgentctlClient(captured)
	manager.applyRuntimeStoragePressure(context.Background(), execution, RuntimeStoragePressure{
		WritePressure: true, ProducerPaused: true, DeliveryState: deliveryFailedHealthState,
		SubmissionID: "submission-1", PromptGeneration: execution.promptGenerationSnapshot(),
		SubmissionCaptured: true, Client: captured,
	})
	before, ok := manager.RuntimeStorageObservation("session")
	if !ok || !before.WritePressure {
		t.Fatalf("initial observation = %+v, ok = %v", before, ok)
	}
	execution.replaceAgentctlClient(replacement)
	manager.applyRuntimeStoragePressure(context.Background(), execution, RuntimeStoragePressure{
		WritePressure: false, DeliveryState: deliveryHealthyHealthState,
		SubmissionID: "submission-1", PromptGeneration: execution.promptGenerationSnapshot(),
		SubmissionCaptured: true, Client: captured,
	})
	after, ok := manager.RuntimeStorageObservation("session")
	if !ok || !after.WritePressure || after.PressureReceivedAt != before.PressureReceivedAt ||
		after.Revision != before.Revision {
		t.Fatalf("replaced-client pressure changed observation: before=%+v after=%+v", before, after)
	}
}
