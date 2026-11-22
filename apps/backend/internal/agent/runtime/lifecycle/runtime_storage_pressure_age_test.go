package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func pressureAgeTestManager(now *time.Time) *Manager {
	manager := &Manager{runtimeStorage: newRuntimeStorageProbeState()}
	manager.runtimeStorageClock = func() time.Time { return *now }
	return manager
}

func pressureAgeTestExecution() *AgentExecution {
	return &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", ExecutorType: "worktree"}
}

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.16
func TestDeliveryStorageCapacityDoesNotRefreshPressureAge(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	manager := pressureAgeTestManager(&now)
	execution := pressureAgeTestExecution()
	manager.applyRuntimeStoragePressure(context.Background(), execution, RuntimeStoragePressure{
		WritePressure: true, ProducerPaused: true, DeliveryState: deliveryStorageErrorHealthState,
		FailureStorage: &models.AgentErrorStorage{Reason: models.AgentStorageReasonDiskQuotaExceeded},
	})
	first, ok := manager.RuntimeStorageObservation("session")
	if !ok || first.PressureReceivedAt.IsZero() {
		t.Fatalf("first observation = %+v, ok = %v", first, ok)
	}
	pressureReceived := first.PressureReceivedAt
	now = now.Add(2 * time.Minute)
	manager.applyRuntimeStorageProbe(context.Background(), execution, storageProbe(1000, 500))
	second, ok := manager.RuntimeStorageObservation("session")
	if !ok {
		t.Fatal("capacity sample removed the observation")
	}
	if !second.PressureReceivedAt.Equal(pressureReceived) {
		t.Fatalf("capacity renewed pressure receipt: %v -> %v", pressureReceived, second.PressureReceivedAt)
	}
	if second.ReceivedAt.IsZero() || !second.ReceivedAt.Equal(now) {
		t.Fatalf("capacity receipt = %v, want %v", second.ReceivedAt, now)
	}
	if !second.WritePressure || second.DeliveryState != deliveryStorageErrorHealthState {
		t.Fatalf("capacity sample cleared pressure: %+v", second)
	}
}

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.16
func TestDeliveryStorageUnchangedPressureRefreshesReceiptButBoundsEvents(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	manager := pressureAgeTestManager(&now)
	execution := pressureAgeTestExecution()
	pressure := RuntimeStoragePressure{
		WritePressure: true, ProducerPaused: true, DeliveryState: deliveryFailedHealthState,
		FailureStorage: &models.AgentErrorStorage{Reason: models.AgentStorageReasonDiskSpaceLow},
	}
	manager.applyRuntimeStoragePressure(context.Background(), execution, pressure)
	first, _ := manager.RuntimeStorageObservation("session")
	now = now.Add(10 * time.Second)
	manager.applyRuntimeStoragePressure(context.Background(), execution, pressure)
	second, _ := manager.RuntimeStorageObservation("session")
	if second.Revision != first.Revision {
		t.Fatalf("unchanged nonhealthy pressure advanced revision before the refresh bound: %d -> %d",
			first.Revision, second.Revision)
	}
	if !second.PressureReceivedAt.Equal(now) {
		t.Fatalf("unchanged pressure receipt = %v, want %v", second.PressureReceivedAt, now)
	}
	now = now.Add(21 * time.Second)
	manager.applyRuntimeStoragePressure(context.Background(), execution, pressure)
	third, _ := manager.RuntimeStorageObservation("session")
	if third.Revision <= second.Revision {
		t.Fatalf("sustained nonhealthy pressure did not refresh after 30s: %d -> %d",
			second.Revision, third.Revision)
	}
	if !third.PressureReceivedAt.Equal(now) {
		t.Fatalf("refreshed pressure receipt = %v, want %v", third.PressureReceivedAt, now)
	}
}

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.16
func TestDeliveryStorageHealthyPressureReceiptStaysSilent(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	manager := pressureAgeTestManager(&now)
	execution := pressureAgeTestExecution()
	healthy := RuntimeStoragePressure{WritePressure: false, DeliveryState: deliveryHealthyHealthState}
	manager.applyRuntimeStoragePressure(context.Background(), execution, healthy)
	first, _ := manager.RuntimeStorageObservation("session")
	now = now.Add(10 * time.Second)
	manager.applyRuntimeStoragePressure(context.Background(), execution, healthy)
	second, _ := manager.RuntimeStorageObservation("session")
	if second.Revision != first.Revision {
		t.Fatalf("healthy silence advanced revision: %d -> %d", first.Revision, second.Revision)
	}
	if !second.PressureReceivedAt.Equal(now) {
		t.Fatalf("healthy receipt = %v, want %v", second.PressureReceivedAt, now)
	}
}

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.16
func TestDeliveryStoragePressureAgeSerializesKnownZeroAndUnknown(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	manager := pressureAgeTestManager(&now)
	execution := pressureAgeTestExecution()
	manager.applyRuntimeStoragePressure(context.Background(), execution, RuntimeStoragePressure{
		WritePressure: true, DeliveryState: deliveryStorageErrorHealthState,
	})
	observation, _ := manager.RuntimeStorageObservation("session")
	if observation.PressureObservedAt == nil || !observation.PressureObservedAt.Equal(now) {
		t.Fatalf("pressure observed at = %v, want %v", observation.PressureObservedAt, now)
	}
	if observation.PressureAgeSeconds != nil && *observation.PressureAgeSeconds != 0 {
		t.Fatalf("cached pressure age = %v, want unknown until publication", observation.PressureAgeSeconds)
	}
	copied := copyRuntimeStorageObservation(observation)
	if copied.PressureObservedAt == observation.PressureObservedAt {
		t.Fatal("copy shared the pressure observed-at pointer")
	}
}
