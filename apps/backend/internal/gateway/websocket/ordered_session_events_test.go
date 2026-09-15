package websocket

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestSanitizedOrderedSessionPayloadStripsSystemContent(t *testing.T) {
	payload := sanitizedOrderedSessionPayload("message.added", map[string]any{
		"session_id":             "session-1",
		"task_id":                "task-1",
		"message_id":             "message-1",
		orderedContentPayloadKey: "visible <kandev-system>secret</kandev-system>",
	})

	require.Equal(t, "visible", payload["content"])
}
func TestTurnRemovedUsesOrderedSessionEventMapping(t *testing.T) {
	require.Equal(t, "session.turn.removed", orderedEventTypeByAction[ws.ActionSessionTurnRemoved])
	payload := sanitizedOrderedSessionPayload(orderedTurnRemovedEvent, map[string]any{
		"id":         "turn-1",
		"session_id": "session-1",
		"task_id":    "task-1",
	})
	require.Equal(t, "turn-1", payload["id"])
}

// TestOrderedRecipientsRevocationReleasesDurableCursor pins the round-9 fix:
// when authorization denies a previously-subscribed client at fanout time, the
// denied client is removed and its durable cursor is released so retention can
// reclaim the partition.
func TestOrderedRecipientsRevocationReleasesDurableCursor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h := NewHub(ws.NewDispatcher(), testLogger())
	go h.Run(ctx)
	svc := plugins.NewService(nil, plugins.NewRegistry(), nil, testLogger())
	h.SetPluginConversationService(svc)
	key := plugins.SessionDeliveryCursorKey{
		SessionID: "sess-revoked", ConsumerKind: "core", WireID: "wire-revoked", UserID: "user-1",
	}
	require.NoError(t, svc.SessionEvents().RegisterCursor(key, 0))

	c := NewClient("c-revoked", authn.Identity{}, nil, h, testLogger())
	registerTestClient(h, c)
	c.orderedSessionSubscriptions = map[string]map[string]plugins.SessionDeliveryCursorKey{
		"sess-revoked": {"wire-revoked": key},
	}
	deny := func(context.Context, string) error { return errors.New("membership revoked") }
	h.authPolicy.Subscriptions.Session = deny

	recipients := h.orderedSessionRecipients("sess-revoked")
	require.Len(t, recipients, 0)
	require.False(t, c.hasOrderedSessionSubscription("sess-revoked"))
	require.False(t, svc.SessionEvents().HasCursor(key))
}
