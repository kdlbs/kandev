package orchestrator

import "testing"

func TestLaunchReceiptRejectsStaleAndDuplicateFacts(t *testing.T) {
	var history LaunchReceiptHistory
	first := LaunchAttemptIdentity{SessionID: "session", Incarnation: "exec-1", Generation: 1}
	second := LaunchAttemptIdentity{SessionID: "session", Incarnation: "exec-2", Generation: 2}
	history.Start(first)
	history.Apply(LaunchReceiptFact{Identity: first, Kind: LaunchFactProcessStarted})
	history.Start(second)
	if history.Apply(LaunchReceiptFact{Identity: first, Kind: LaunchFactInferenceStarted}) {
		t.Fatal("stale fact applied to current receipt")
	}
	processStarted := LaunchReceiptFact{Identity: second, Kind: LaunchFactProcessStarted}
	if !history.Apply(processStarted) {
		t.Fatal("current fact was rejected")
	}
	if !history.Apply(processStarted) {
		t.Fatal("duplicate delivery was rejected")
	}
	if len(history.Current.Facts) != 1 {
		t.Fatalf("facts = %#v, want one deduplicated fact", history.Current.Facts)
	}
}

func TestLaunchReceiptTriStateOnlyTurnsFalseOnTypedPreflightFailure(t *testing.T) {
	var history LaunchReceiptHistory
	id := LaunchAttemptIdentity{SessionID: "session", Incarnation: "exec", Generation: 1}
	history.Start(id)
	if history.Current.ProcessCreated != LaunchTriStateUnknown || history.Current.InferenceStarted != LaunchTriStateUnknown {
		t.Fatalf("initial states = %+v", history.Current)
	}
	history.Apply(LaunchReceiptFact{Identity: id, Kind: LaunchFactTerminalPreflightFailure})
	if history.Current.ProcessCreated != LaunchTriStateFalse || history.Current.InferenceStarted != LaunchTriStateFalse {
		t.Fatalf("preflight states = %+v", history.Current)
	}
}
