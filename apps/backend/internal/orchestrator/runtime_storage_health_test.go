package orchestrator

import (
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
)

type runtimeStorageObservationAgent struct {
	*mockAgentManager
	observation agentruntime.RuntimeStorageObservation
	ok          bool
}

func (a *runtimeStorageObservationAgent) RuntimeStorageObservation(string) (agentruntime.RuntimeStorageObservation, bool) {
	return a.observation, a.ok
}

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.16
func TestRuntimeStorageHealthProjectsIndependentPressureAge(t *testing.T) {
	receivedAt := time.Now().UTC().Add(-5 * time.Second)
	observedAt := receivedAt.Add(-time.Minute)
	pressureObservedAt := receivedAt
	agent := &runtimeStorageObservationAgent{
		mockAgentManager: &mockAgentManager{},
		ok:               true,
		observation: agentruntime.RuntimeStorageObservation{
			State:              "normal",
			Location:           "local_runtime",
			ObservedAt:         &observedAt,
			ReceivedAt:         receivedAt,
			PressureObservedAt: &pressureObservedAt,
			PressureReceivedAt: receivedAt,
			WritePressure:      true,
		},
	}
	service := &Service{agentManager: agent}
	health := service.RuntimeStorageHealth("session")
	if health == nil {
		t.Fatal("runtime storage health was omitted")
	}
	if health.PressureObservedAt == nil || !health.PressureObservedAt.Equal(pressureObservedAt) {
		t.Fatalf("pressure observed at = %v, want %v", health.PressureObservedAt, pressureObservedAt)
	}
	if health.PressureAgeSeconds == nil || *health.PressureAgeSeconds < 4 {
		t.Fatalf("pressure age = %v, want at least four seconds", health.PressureAgeSeconds)
	}
	if health.AgeSeconds < 4 {
		t.Fatalf("capacity age = %d, want at least four seconds", health.AgeSeconds)
	}
	if health.ObservedAt == nil || !health.ObservedAt.Equal(observedAt) {
		t.Fatalf("capacity observed at = %v, want %v", health.ObservedAt, observedAt)
	}
}

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.16
func TestRuntimeStorageHealthLeavesUnknownPressureAgeAbsent(t *testing.T) {
	agent := &runtimeStorageObservationAgent{
		mockAgentManager: &mockAgentManager{},
		ok:               true,
		observation:      agentruntime.RuntimeStorageObservation{State: "warning", Location: "local_runtime"},
	}
	health := (&Service{agentManager: agent}).RuntimeStorageHealth("session")
	if health == nil || health.PressureAgeSeconds != nil || health.PressureObservedAt != nil {
		t.Fatalf("unknown pressure freshness became a claim: %+v", health)
	}
}
