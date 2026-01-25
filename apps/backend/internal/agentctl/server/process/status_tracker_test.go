package process

import (
	"sync"
	"testing"
	"time"

	"github.com/tuzig/vt10x"
)

// mockDetector is a test detector that returns configurable states
type mockDetector struct {
	state           AgentState
	acceptTransition bool
}

func (d *mockDetector) DetectState(lines []string, glyphs [][]vt10x.Glyph) AgentState {
	return d.state
}

func (d *mockDetector) ShouldAcceptStateChange(currentState, newState AgentState) bool {
	return d.acceptTransition
}

func TestStatusTracker_Write(t *testing.T) {
	log := newTestLogger(t)
	detector := &mockDetector{state: StateUnknown, acceptTransition: true}
	config := DefaultStatusTrackerConfig()

	tracker := NewStatusTracker("test-session", detector, nil, config, log)

	// Write some terminal data
	tracker.Write([]byte("Hello, World!"))

	// The data should be fed to the terminal - we can't easily verify this
	// without exposing internal state, but at least verify no panics
}

func TestStatusTracker_Resize(t *testing.T) {
	log := newTestLogger(t)
	detector := &mockDetector{state: StateUnknown, acceptTransition: true}
	config := StatusTrackerConfig{
		Rows:          24,
		Cols:          80,
		CheckInterval: 100 * time.Millisecond,
	}

	tracker := NewStatusTracker("test-session", detector, nil, config, log)

	// Resize the terminal
	tracker.Resize(120, 40)

	// Verify config was updated
	tracker.mu.Lock()
	cols := tracker.config.Cols
	rows := tracker.config.Rows
	tracker.mu.Unlock()

	if cols != 120 {
		t.Errorf("Resize() cols = %d, want 120", cols)
	}
	if rows != 40 {
		t.Errorf("Resize() rows = %d, want 40", rows)
	}
}

func TestStatusTracker_ShouldCheck(t *testing.T) {
	log := newTestLogger(t)
	detector := &mockDetector{state: StateUnknown, acceptTransition: true}
	config := StatusTrackerConfig{
		Rows:          24,
		Cols:          80,
		CheckInterval: 50 * time.Millisecond,
	}

	tracker := NewStatusTracker("test-session", detector, nil, config, log)

	// Should check immediately after creation
	if !tracker.ShouldCheck() {
		t.Error("ShouldCheck() should return true initially")
	}

	// After checking, should not check again immediately
	tracker.CheckAndUpdate()

	if tracker.ShouldCheck() {
		t.Error("ShouldCheck() should return false right after check")
	}

	// After waiting, should check again
	time.Sleep(60 * time.Millisecond)

	if !tracker.ShouldCheck() {
		t.Error("ShouldCheck() should return true after check interval")
	}
}

func TestStatusTracker_CheckAndUpdate_StateChange(t *testing.T) {
	log := newTestLogger(t)
	detector := &mockDetector{state: StateWorking, acceptTransition: true}

	var callbackCalled bool
	var callbackState AgentState
	var callbackMu sync.Mutex

	callback := func(sessionID string, state AgentState) {
		callbackMu.Lock()
		callbackCalled = true
		callbackState = state
		callbackMu.Unlock()
	}

	config := DefaultStatusTrackerConfig()
	tracker := NewStatusTracker("test-session", detector, callback, config, log)

	// First check should emit the initial state change
	state := tracker.CheckAndUpdate()

	if state != StateWorking {
		t.Errorf("CheckAndUpdate() = %v, want %v", state, StateWorking)
	}

	callbackMu.Lock()
	if !callbackCalled {
		t.Error("Callback should have been called on state change")
	}
	if callbackState != StateWorking {
		t.Errorf("Callback received state %v, want %v", callbackState, StateWorking)
	}
	callbackMu.Unlock()
}

func TestStatusTracker_CheckAndUpdate_NoChangeNoCallback(t *testing.T) {
	log := newTestLogger(t)
	detector := &mockDetector{state: StateWorking, acceptTransition: true}

	callCount := 0
	callback := func(sessionID string, state AgentState) {
		callCount++
	}

	config := DefaultStatusTrackerConfig()
	tracker := NewStatusTracker("test-session", detector, callback, config, log)

	// First check triggers callback
	tracker.CheckAndUpdate()

	// Second check with same state should not trigger callback
	tracker.CheckAndUpdate()

	if callCount != 1 {
		t.Errorf("Callback called %d times, want 1", callCount)
	}
}

func TestStatusTracker_CheckAndUpdate_TransitionRejected(t *testing.T) {
	log := newTestLogger(t)
	detector := &mockDetector{state: StateWorking, acceptTransition: false}

	callCount := 0
	callback := func(sessionID string, state AgentState) {
		callCount++
	}

	config := DefaultStatusTrackerConfig()
	tracker := NewStatusTracker("test-session", detector, callback, config, log)

	// Check - detector rejects the transition
	state := tracker.CheckAndUpdate()

	// State should remain unknown since transition was rejected
	if state != StateUnknown {
		t.Errorf("CheckAndUpdate() = %v, want %v (transition rejected)", state, StateUnknown)
	}

	if callCount != 0 {
		t.Errorf("Callback called %d times, want 0 (transition rejected)", callCount)
	}
}

func TestStatusTracker_StabilityWindow(t *testing.T) {
	log := newTestLogger(t)
	detector := &mockDetector{state: StateWorking, acceptTransition: true}

	var lastState AgentState
	callback := func(sessionID string, state AgentState) {
		lastState = state
	}

	config := StatusTrackerConfig{
		Rows:            24,
		Cols:            80,
		CheckInterval:   10 * time.Millisecond,
		StabilityWindow: 50 * time.Millisecond,
	}

	tracker := NewStatusTracker("test-session", detector, callback, config, log)

	// First check - state becomes pending
	tracker.CheckAndUpdate()

	// State should still be unknown (waiting for stability)
	if lastState == StateWorking {
		t.Error("State should not change immediately with stability window")
	}

	// Wait for stability window
	time.Sleep(60 * time.Millisecond)

	// Now check again - state should be stable
	tracker.CheckAndUpdate()

	if lastState != StateWorking {
		t.Errorf("State should be %v after stability window, got %v", StateWorking, lastState)
	}
}

func TestStatusTracker_CurrentState(t *testing.T) {
	log := newTestLogger(t)
	detector := &mockDetector{state: StateWaitingInput, acceptTransition: true}

	config := DefaultStatusTrackerConfig()
	tracker := NewStatusTracker("test-session", detector, nil, config, log)

	// Initial state
	if tracker.CurrentState() != StateUnknown {
		t.Errorf("CurrentState() = %v, want %v", tracker.CurrentState(), StateUnknown)
	}

	// After check
	tracker.CheckAndUpdate()

	if tracker.CurrentState() != StateWaitingInput {
		t.Errorf("CurrentState() = %v, want %v", tracker.CurrentState(), StateWaitingInput)
	}
}

func TestDefaultStatusTrackerConfig(t *testing.T) {
	config := DefaultStatusTrackerConfig()

	if config.Rows != 24 {
		t.Errorf("DefaultStatusTrackerConfig().Rows = %d, want 24", config.Rows)
	}
	if config.Cols != 80 {
		t.Errorf("DefaultStatusTrackerConfig().Cols = %d, want 80", config.Cols)
	}
	if config.CheckInterval != 100*time.Millisecond {
		t.Errorf("DefaultStatusTrackerConfig().CheckInterval = %v, want 100ms", config.CheckInterval)
	}
	if config.StabilityWindow != 0 {
		t.Errorf("DefaultStatusTrackerConfig().StabilityWindow = %v, want 0", config.StabilityWindow)
	}
}

func TestNoOpDetector(t *testing.T) {
	detector := &NoOpDetector{}

	// Always returns unknown
	state := detector.DetectState(nil, nil)
	if state != StateUnknown {
		t.Errorf("NoOpDetector.DetectState() = %v, want %v", state, StateUnknown)
	}

	// Always accepts transitions
	if !detector.ShouldAcceptStateChange(StateWorking, StateUnknown) {
		t.Error("NoOpDetector.ShouldAcceptStateChange() should always return true")
	}
}
