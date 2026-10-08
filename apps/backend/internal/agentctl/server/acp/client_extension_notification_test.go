package acp

import (
	"context"
	"encoding/json"
	"testing"
)

// TestHandleExtensionNotificationForwardsToHandler verifies inbound extension
// notifications reach the registered handler with their method and params.
func TestHandleExtensionNotificationForwardsToHandler(t *testing.T) {
	var gotMethod string
	var gotParams json.RawMessage
	c := NewClient(WithExtensionNotificationHandler(func(method string, params json.RawMessage) {
		gotMethod, gotParams = method, params
	}))

	params := json.RawMessage(`{"sessionId":"s-1"}`)
	if err := c.HandleExtensionNotification(context.Background(), "_claude/sdkMessage", params); err != nil {
		t.Fatalf("HandleExtensionNotification: %v", err)
	}
	if gotMethod != "_claude/sdkMessage" || string(gotParams) != string(params) {
		t.Fatalf("handler got (%q, %s), want (_claude/sdkMessage, %s)", gotMethod, gotParams, params)
	}
}

// TestHandleExtensionNotificationWithoutHandlerIsIgnored verifies a client with
// no handler accepts and ignores extension notifications.
func TestHandleExtensionNotificationWithoutHandlerIsIgnored(t *testing.T) {
	if err := NewClient().HandleExtensionNotification(context.Background(), "_x/y", nil); err != nil {
		t.Fatalf("HandleExtensionNotification: %v", err)
	}
}
