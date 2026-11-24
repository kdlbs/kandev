package acpv2

import "testing"

func TestForegroundOutcomeRequiresAcknowledgementAndTerminalState(t *testing.T) {
	for _, ackFirst := range []bool{true, false} {
		turn := &foregroundTurn{}
		if ackFirst {
			turn.acknowledge("prompt")
		}
		turn.observeState("running", "")
		turn.observeState("requires_action", "")
		if turn.settled() {
			t.Fatal("running or pending action does not settle the turn")
		}
		turn.observeState("idle", "cancelled")
		if !ackFirst && turn.settled() {
			t.Fatal("terminal state does not acknowledge insertion")
		}
		if !ackFirst {
			turn.acknowledge("prompt")
		}
		turn.observeState("idle", "end_turn")
		if !turn.settled() || turn.stopReason != "cancelled" {
			t.Fatalf("foreground turn did not retain its first terminal outcome: %+v", turn)
		}
	}
}

func TestForegroundIgnoresBaselineAndUnknownState(t *testing.T) {
	turn := &foregroundTurn{}
	turn.observeState("idle", "")
	turn.acknowledge("next-message")
	turn.observeState("unknown", "")
	if turn.settled() {
		t.Fatal("baseline idle or unknown state settled a foreground turn")
	}
	turn.observeState("running", "")
	turn.observeState("idle", "")
	if !turn.settled() || turn.stopReason != "" {
		t.Fatalf("idle transition without a stop reason lost its outcome: %+v", turn)
	}
}
