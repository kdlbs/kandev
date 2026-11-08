package lifecycle

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func storageProbe(total, available int64) RuntimeStorageProbeResult {
	return RuntimeStorageProbeResult{
		Available:      true,
		StorageKind:    "journal",
		TotalBytes:     &total,
		AvailableBytes: &available,
		ObservedAt:     time.Now().UTC(),
	}
}

func TestDeliveryStorageAdvisoryThresholdsUseHysteresis(t *testing.T) {
	cases := []struct {
		name      string
		available int64
		previous  string
		want      string
	}{
		{name: "89.9% without warning stays normal", available: 101, want: RuntimeStorageStateNormal},
		{name: "90% warns", available: 100, want: RuntimeStorageStateWarning},
		{name: "88% retains warning", available: 120, previous: RuntimeStorageStateWarning, want: RuntimeStorageStateWarning},
		{name: "88% without warning stays normal", available: 120, want: RuntimeStorageStateNormal},
		{name: "85% retains warning before clearance", available: 150, previous: RuntimeStorageStateWarning, want: RuntimeStorageStateWarning},
		{name: "84.9% clears warning", available: 151, previous: RuntimeStorageStateWarning, want: RuntimeStorageStateNormal},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			observation := buildRuntimeStorageObservation(
				&AgentExecution{TaskID: "task", SessionID: "session"},
				storageProbe(1000, test.available),
				RuntimeStorageObservation{State: test.previous},
				test.previous != "",
				newRuntimeStorageProbeState(),
			)
			if observation.State != test.want {
				t.Fatalf("state = %q, want %q", observation.State, test.want)
			}
			if observation.TotalBytes == nil || *observation.TotalBytes != 1000 {
				t.Fatalf("measurements = %+v", observation)
			}
		})
	}
}

func TestDeliveryStorageFailedSampleKeepsPriorCauseAndMeasurements(t *testing.T) {
	observedAt := time.Now().UTC().Add(-time.Minute)
	total, available := int64(1000), int64(50)
	observation := buildRuntimeStorageObservation(
		&AgentExecution{TaskID: "task", SessionID: "session", ExecutorType: "ssh"},
		RuntimeStorageProbeResult{Available: false},
		RuntimeStorageObservation{
			State: RuntimeStorageStateWarning, TotalBytes: &total, AvailableBytes: &available,
			ObservedAt: &observedAt, StorageKind: "journal",
		},
		true,
		newRuntimeStorageProbeState(),
	)
	if observation.State != RuntimeStorageStateWarning {
		t.Fatalf("failed sample invented a state: %+v", observation)
	}
	if observation.AvailableBytes == nil || *observation.AvailableBytes != available {
		t.Fatalf("failed sample dropped measurements: %+v", observation)
	}
	if observation.Location != "executor_runtime" {
		t.Fatalf("location = %q", observation.Location)
	}
}

func TestDeliveryStorageUnknownMeasurementsStayUnknown(t *testing.T) {
	observation := buildRuntimeStorageObservation(
		&AgentExecution{TaskID: "task", SessionID: "session"},
		RuntimeStorageProbeResult{Available: true, StorageKind: "workspace", ObservedAt: time.Now().UTC()},
		RuntimeStorageObservation{},
		false,
		newRuntimeStorageProbeState(),
	)
	if observation.State != RuntimeStorageStateUnavailable {
		t.Fatalf("state = %q", observation.State)
	}
	if observation.TotalBytes != nil || observation.AvailableBytes != nil {
		t.Fatalf("unknown measurements became known: %+v", observation)
	}
}

func TestDeliveryStorageReplacementExecutionDoesNotInheritWarning(t *testing.T) {
	manager := &Manager{runtimeStorage: newRuntimeStorageProbeState()}
	first := &AgentExecution{ID: "execution-1", TaskID: "task", SessionID: "session"}
	manager.applyRuntimeStorageProbe(context.Background(), first, storageProbe(1000, 100))
	replacement := &AgentExecution{ID: "execution-2", TaskID: "task", SessionID: "session"}
	manager.applyRuntimeStorageProbe(context.Background(), replacement, storageProbe(1000, 120))
	observation, ok := manager.RuntimeStorageObservation("session")
	if !ok || observation.ExecutionID != "execution-2" {
		t.Fatalf("observation = %+v, ok = %v", observation, ok)
	}
	if observation.State != RuntimeStorageStateNormal {
		t.Fatalf("replacement inherited prior warning hysteresis: %q", observation.State)
	}
}

func TestDeliveryStorageObservationRevisionAndLocalLocation(t *testing.T) {
	manager := &Manager{
		logger:         nil,
		runtimeStorage: newRuntimeStorageProbeState(),
	}
	execution := &AgentExecution{TaskID: "task", SessionID: "session", ExecutorType: "worktree"}
	manager.applyRuntimeStorageProbe(context.Background(), execution, storageProbe(1000, 100))
	first, ok := manager.RuntimeStorageObservation("session")
	if !ok || first.State != RuntimeStorageStateWarning || first.Revision == 0 {
		t.Fatalf("first observation = %+v, ok = %v", first, ok)
	}
	if first.Location != "local_runtime" {
		t.Fatalf("location = %q", first.Location)
	}
	manager.applyRuntimeStorageProbe(context.Background(), execution, storageProbe(1000, 100))
	second, _ := manager.RuntimeStorageObservation("session")
	if second.Revision <= first.Revision {
		t.Fatalf("warning refresh revision did not advance: %d -> %d", first.Revision, second.Revision)
	}
	manager.applyRuntimeStorageProbe(context.Background(), execution, storageProbe(1000, 100))
	third, _ := manager.RuntimeStorageObservation("session")
	if third.Revision <= second.Revision {
		t.Fatalf("revision not monotonic: %d -> %d", second.Revision, third.Revision)
	}
}

func TestDeliveryStorageProductionEpochIsBrowserSafe(t *testing.T) {
	state := newRuntimeStorageProbeState()
	if state.runtimeEpoch == 0 || state.runtimeEpoch > runtimeStorageMaximumWireEpoch {
		t.Fatalf("epoch = %d, want a positive JavaScript-safe integer", state.runtimeEpoch)
	}
	manager := &Manager{runtimeStorage: state}
	manager.applyRuntimeStorageProbe(context.Background(),
		&AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", ExecutorType: "worktree"},
		storageProbe(1000, 100),
	)
	observation, ok := manager.RuntimeStorageObservation("session")
	if !ok {
		t.Fatal("observation missing")
	}
	encoded, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	epoch, ok := wire["epoch"].(float64)
	if !ok || epoch <= 0 || epoch > float64(1<<53-1) {
		t.Fatalf("serialized epoch = %v", wire["epoch"])
	}
}

func TestDeliveryStoragePressureIsIndependentOfUsageThreshold(t *testing.T) {
	manager := &Manager{runtimeStorage: newRuntimeStorageProbeState()}
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", ExecutorType: "worktree"}
	manager.applyRuntimeStorageProbe(context.Background(), execution, storageProbe(1000, 500))
	storage := &models.AgentErrorStorage{Reason: models.AgentStorageReasonDiskQuotaExceeded}
	manager.applyRuntimeStoragePressure(context.Background(), execution, RuntimeStoragePressure{
		WritePressure: true, ProducerPaused: true, DeliveryState: "delivery_failed", FailureStorage: storage,
	})
	observation, ok := manager.RuntimeStorageObservation("session")
	if !ok || observation.State != RuntimeStorageStateNormal {
		t.Fatalf("observation = %+v, ok = %v", observation, ok)
	}
	if !observation.WritePressure || !observation.ProducerPaused || observation.DeliveryState != "delivery_failed" {
		t.Fatalf("pressure not projected below the usage threshold: %+v", observation)
	}
	if observation.FailureStorage == nil || observation.FailureStorage.Reason != models.AgentStorageReasonDiskQuotaExceeded {
		t.Fatalf("failure storage = %+v", observation.FailureStorage)
	}
	beforeRefresh, _ := manager.RuntimeStorageObservation("session")
	manager.applyRuntimeStorageProbe(context.Background(), execution, storageProbe(1000, 500))
	observation, _ = manager.RuntimeStorageObservation("session")
	if !observation.WritePressure || observation.FailureStorage == nil {
		t.Fatal("a healthy capacity sample cleared a blocked writer")
	}
	if observation.Revision <= beforeRefresh.Revision {
		t.Fatalf("sustained blocked writer did not refresh: %d -> %d",
			beforeRefresh.Revision, observation.Revision)
	}
}

func TestDeliveryStorageFailedSamplePreservesReceiptAge(t *testing.T) {
	manager := &Manager{runtimeStorage: newRuntimeStorageProbeState()}
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session"}
	manager.applyRuntimeStorageProbe(context.Background(), execution, storageProbe(1000, 100))
	first, _ := manager.RuntimeStorageObservation("session")
	manager.applyRuntimeStorageProbe(context.Background(), execution, RuntimeStorageProbeResult{Available: false})
	second, _ := manager.RuntimeStorageObservation("session")
	if second.ReceivedAt.IsZero() || !second.ReceivedAt.Equal(first.ReceivedAt) {
		t.Fatalf("failed sample renewed receipt age: %v -> %v", first.ReceivedAt, second.ReceivedAt)
	}
}

func TestDeliveryStorageLateOwnerResultCannotReplaceSuccessor(t *testing.T) {
	store := NewExecutionStore()
	predecessor := &AgentExecution{ID: "execution-old", TaskID: "task", SessionID: "session"}
	if err := store.Add(predecessor); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{executionStore: store, runtimeStorage: newRuntimeStorageProbeState()}
	manager.applyRuntimeStorageProbe(context.Background(), predecessor, storageProbe(1000, 100))
	store.Remove(predecessor.ID)
	successor := &AgentExecution{ID: "execution-new", TaskID: "task", SessionID: "session"}
	if err := store.Add(successor); err != nil {
		t.Fatal(err)
	}
	manager.applyRuntimeStorageProbe(context.Background(), successor, storageProbe(1000, 500))
	manager.applyRuntimeStorageProbe(context.Background(), predecessor, storageProbe(1000, 100))
	observation, ok := manager.RuntimeStorageObservation("session")
	if !ok || observation.ExecutionID != successor.ID || observation.State != RuntimeStorageStateNormal {
		t.Fatalf("late predecessor replaced successor: %+v, ok = %v", observation, ok)
	}
	store.Remove(successor.ID)
	if _, ok := manager.RuntimeStorageObservation("session"); ok {
		t.Fatal("removed execution still exposed a current observation")
	}
}

func TestDeliveryStorageLateAttachmentResultCannotReplaceSuccessor(t *testing.T) {
	streamsManager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	t.Cleanup(streamsManager.Wait)
	oldAttachment := &deliveryStreamAttachment{}
	newAttachment := &deliveryStreamAttachment{}
	streamsManager.deliveryAttachments.Store("execution", oldAttachment)
	manager := &Manager{
		streamManager:  streamsManager,
		runtimeStorage: newRuntimeStorageProbeState(),
	}
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session"}
	if !manager.runtimeStorageAttachmentCurrent(execution, oldAttachment) {
		t.Fatal("test setup: old attachment is not current")
	}
	first := storageProbe(1000, 500)
	first.attachment = oldAttachment
	manager.applyRuntimeStorageProbe(context.Background(), execution, first)
	streamsManager.deliveryAttachments.Store("execution", newAttachment)
	newSample := storageProbe(1000, 100)
	newSample.attachment = newAttachment
	manager.applyRuntimeStorageProbe(context.Background(), execution, newSample)
	late := storageProbe(1000, 500)
	late.attachment = oldAttachment
	manager.applyRuntimeStorageProbe(context.Background(), execution, late)
	observation, ok := manager.RuntimeStorageObservation("session")
	if !ok || observation.State != RuntimeStorageStateWarning {
		t.Fatalf("late attachment replaced successor: %+v, ok = %v", observation, ok)
	}
}

func TestDeliveryStorageReceiptAgeUsesBackendTime(t *testing.T) {
	now := time.Now().UTC()
	if age := runtimeStorageAgeSeconds(now.Add(-2*time.Minute), now); age != 120 {
		t.Fatalf("receipt age = %d, want 120", age)
	}
	if age := runtimeStorageAgeSeconds(now.Add(time.Minute), now); age != 0 {
		t.Fatalf("future receipt age = %d, want 0", age)
	}
}

func TestDeliveryStorageAdvisoryStartsWithoutUpdatesStream(t *testing.T) {
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	execution := &AgentExecution{ID: "passthrough-execution", TaskID: "task", SessionID: "session"}
	manager.startRuntimeStorageAdvisory(execution)
	if _, loaded := manager.runtimeStorageAdvisories.Load(execution.ID); !loaded {
		t.Fatal("workspace-started advisory did not register without an updates stream")
	}
	manager.Wait()
	if _, loaded := manager.runtimeStorageAdvisories.Load(execution.ID); loaded {
		t.Fatal("advisory was not released on shutdown")
	}
}

func TestDeliveryStorageObservationIsRetiredOnExecutionRemoval(t *testing.T) {
	manager := &Manager{runtimeStorage: newRuntimeStorageProbeState()}
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session"}
	manager.applyRuntimeStorageProbe(context.Background(), execution, storageProbe(1000, 100))
	manager.retireRuntimeStorageObservation(execution, nil)
	if _, ok := manager.RuntimeStorageObservation("session"); ok {
		t.Fatal("removed execution retained a cached observation")
	}
}
