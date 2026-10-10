package acp

import (
	"context"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestAwaitModeSettleConfirmsReportBeforeRPCReturns(t *testing.T) {
	a := &Adapter{sessionID: "session-1"}
	baseline := a.currentModeSnapshot().generation

	// ACP can deliver current_mode_update before session/set_mode responds.
	a.noteCurrentMode("session-1", "default")
	result := a.awaitModeSettle(context.Background(), "session-1", "bypassPermissions", baseline)

	if !result.Confirmed || result.Effective != "default" {
		t.Fatalf("result = %+v, want the early clamp report", result)
	}
}

func TestAwaitModeSettleDoesNotConfirmPreRequestMode(t *testing.T) {
	a := &Adapter{sessionID: "session-1"}
	a.noteCurrentMode("session-1", "bypassPermissions")
	baseline := a.currentModeSnapshot().generation

	result := a.awaitModeSettle(context.Background(), "session-1", "bypassPermissions", baseline)

	if result.Confirmed || result.Effective != "" {
		t.Fatalf("result = %+v, want an unconfirmed result without an effective mode", result)
	}
}

func TestAwaitModeSettleRejectsAStaleSessionReport(t *testing.T) {
	a := &Adapter{sessionID: "session-2"}
	a.noteCurrentMode("session-2", "default")
	baseline := a.currentModeSnapshot()
	event := a.convertNotification(acpsdk.SessionNotification{
		SessionId: "session-1",
		Update: acpsdk.SessionUpdate{
			CurrentModeUpdate: &acpsdk.SessionCurrentModeUpdate{
				SessionUpdate: "current_mode_update",
				CurrentModeId: acpsdk.SessionModeId("bypassPermissions"),
			},
		},
	})
	if event != nil {
		t.Fatalf("stale session mode event = %+v, want dropped", event)
	}
	after := a.currentModeSnapshot()
	if after.sessionID != baseline.sessionID || after.mode != baseline.mode || after.generation != baseline.generation {
		t.Fatalf("stale report changed mode state: before=%+v after=%+v", baseline, after)
	}

	result := a.awaitModeSettle(context.Background(), "session-2", "bypassPermissions", after.generation)

	if result.Confirmed || result.Effective != "" {
		t.Fatalf("result = %+v, want stale report ignored", result)
	}
}

func TestAwaitModeSettleRespectsContextCancellation(t *testing.T) {
	a := &Adapter{sessionID: "session-1"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := a.awaitModeSettle(ctx, "session-1", "bypassPermissions", 0)
	if result.Confirmed || result.Effective != "" {
		t.Fatalf("result = %+v, want canceled request to stay unconfirmed", result)
	}
}
