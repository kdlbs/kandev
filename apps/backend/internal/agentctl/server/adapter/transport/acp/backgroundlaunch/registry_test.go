package backgroundlaunch_test

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp/backgroundlaunch"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type fakeRecognizer struct {
	agentID  string
	classify func(*streams.NormalizedPayload) bool
	panics   bool
}

func (f fakeRecognizer) AgentID() string { return f.agentID }

func (f fakeRecognizer) RecognizesDetachedLaunch(payload *streams.NormalizedPayload) bool {
	if f.panics {
		panic("fakeRecognizer: intentional panic for test")
	}
	if f.classify == nil {
		return false
	}
	return f.classify(payload)
}

func detachedShellPayload() *streams.NormalizedPayload {
	p := streams.NewShellExec("sleep 300", "", "", 0, true)
	return p
}

// D7: lookup miss is the default for every agent — no attestation, no probe,
// no parking.
func TestLookup_MissForUnregisteredAgent(t *testing.T) {
	_, ok := backgroundlaunch.Lookup("an-agent-nobody-registered")
	if ok {
		t.Fatalf("expected no recognizer registered for an arbitrary agent ID")
	}
}

func TestRegister_NilRecognizerPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected Register(nil) to panic")
		}
	}()
	backgroundlaunch.Register(nil)
}

func TestRegister_EmptyAgentIDPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected Register with an empty AgentID() to panic")
		}
	}()
	backgroundlaunch.Register(fakeRecognizer{agentID: ""})
}

func TestRegister_DuplicateAgentIDPanics(t *testing.T) {
	backgroundlaunch.Register(fakeRecognizer{agentID: "dup-agent-test"})

	defer func() {
		if recover() == nil {
			t.Fatalf("expected a second Register for the same agent ID to panic")
		}
	}()
	backgroundlaunch.Register(fakeRecognizer{agentID: "dup-agent-test"})
}

func TestRegisterAndLookup_RoundTrip(t *testing.T) {
	backgroundlaunch.Register(fakeRecognizer{
		agentID: "round-trip-agent-test",
		classify: func(p *streams.NormalizedPayload) bool {
			return p != nil && p.ShellExec() != nil && p.ShellExec().Background
		},
	})

	got, ok := backgroundlaunch.Lookup("round-trip-agent-test")
	if !ok {
		t.Fatalf("expected the registered recognizer to be found")
	}
	if got.AgentID() != "round-trip-agent-test" {
		t.Errorf("AgentID() = %q, want round-trip-agent-test", got.AgentID())
	}
}

// D7: a recogniser that panics is treated as "did not recognise" — fail
// closed to today's behaviour.
func TestRecognizesDetachedLaunch_PanicIsTreatedAsNotRecognized(t *testing.T) {
	backgroundlaunch.Register(fakeRecognizer{agentID: "panicking-agent-test", panics: true})

	if backgroundlaunch.RecognizesDetachedLaunch("panicking-agent-test", detachedShellPayload()) {
		t.Errorf("expected a panicking recognizer to be treated as false")
	}
}

func TestRecognizesDetachedLaunch_UnregisteredAgentIsFalse(t *testing.T) {
	if backgroundlaunch.RecognizesDetachedLaunch("an-agent-nobody-registered", detachedShellPayload()) {
		t.Errorf("expected an unregistered agent to never be recognised")
	}
}

func TestRecognizesDetachedLaunch_DelegatesToTheRegisteredRecognizer(t *testing.T) {
	backgroundlaunch.Register(fakeRecognizer{
		agentID: "delegating-agent-test",
		classify: func(p *streams.NormalizedPayload) bool {
			return p != nil && p.ShellExec() != nil && p.ShellExec().Background
		},
	})

	if !backgroundlaunch.RecognizesDetachedLaunch("delegating-agent-test", detachedShellPayload()) {
		t.Errorf("expected the registered recognizer's classification to be used")
	}

	foreground := streams.NewShellExec("ls", "", "", 0, false)
	if backgroundlaunch.RecognizesDetachedLaunch("delegating-agent-test", foreground) {
		t.Errorf("expected a foreground shell payload to not be recognised as detached")
	}
}
