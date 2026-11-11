package hostutility

import (
	"context"
	"errors"
	"testing"
)

func TestProfileFlightRejectsObsoleteRegistration(t *testing.T) {
	m := &Manager{profileKey: []byte("test"), profileCache: newProfileCapabilityCache()}
	old, _ := m.profileContextGeneration("agent", "revision", 0, false)
	current, _ := m.profileContextGeneration("agent", "revision", 0, true)
	fresh, finishFresh := m.beginProfileDiscoveryFlight(context.Background(), "agent", "profile", "revision", 0, current)
	defer finishFresh()
	stale, finishStale := m.beginProfileDiscoveryFlight(context.Background(), "agent", "profile", "revision", 0, old)
	defer finishStale()
	if !errors.Is(context.Cause(stale), errProfileContextChanged) || fresh.Err() != nil {
		t.Fatalf("obsolete registration: old cause=%v, fresh cause=%v", context.Cause(stale), context.Cause(fresh))
	}
}

func TestProfileFlightRejectsPublicationAfterNewContextCompletes(t *testing.T) {
	m := &Manager{profileKey: []byte("test"), profileCache: newProfileCapabilityCache()}
	old, _ := m.profileContextGeneration("agent", "old-revision", 0, false)
	_, finishOld := m.beginProfileDiscoveryFlight(context.Background(), "agent", "profile", "old-revision", 0, old)
	defer finishOld()
	current, _ := m.profileContextGeneration("agent", "new-revision", 0, false)
	_, finishFresh := m.beginProfileDiscoveryFlight(context.Background(), "agent", "profile", "new-revision", 0, current)
	finishFresh()
	if m.setProfileCapabilityIfCurrent("agent", "profile", "old-revision", 0, old, "stale", ProfileCapabilityResult{}) {
		t.Fatal("superseded profile context published a capability")
	}
	if m.setProfileResolutionIfCurrent("agent", "profile", "old-revision", 0, old, "stale", ModelConfigResolution{}) {
		t.Fatal("superseded profile context published model options")
	}
}

func TestProfileFlightAllowsReturningToPreviousContext(t *testing.T) {
	m := &Manager{profileKey: []byte("test"), profileCache: newProfileCapabilityCache()}
	old, _ := m.profileContextGeneration("agent", "old", 0, false)
	_, doneOld := m.beginProfileDiscoveryFlight(context.Background(), "agent", "profile", "old", 0, old)
	doneOld()
	fresh, _ := m.profileContextGeneration("agent", "new", 0, false)
	_, doneFresh := m.beginProfileDiscoveryFlight(context.Background(), "agent", "profile", "new", 0, fresh)
	doneFresh()
	again, doneAgain := m.beginProfileDiscoveryFlight(context.Background(), "agent", "profile", "old", 0, old)
	defer doneAgain()
	if again.Err() != nil || !m.setProfileCapabilityIfCurrent("agent", "profile", "old", 0, old, "returned", ProfileCapabilityResult{}) {
		t.Fatal("a later request could not return to a valid previous context")
	}
}
