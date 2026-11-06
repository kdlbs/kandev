package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestExecutorOwnershipNotificationsFailClosedWithoutResolver(t *testing.T) {
	hub := newTestHub(t)
	hub.setAuthPolicy(AuthPolicy{Enforced: func() bool { return true }})
	broadcaster := &TaskEventBroadcaster{hub: hub, logger: testLogger()}
	event := bus.NewEvent(events.ExecutorProfileUpdated, "test", map[string]any{
		"id": "private", "executor_id": "private-parent", "config": map[string]string{"host": "private-host"},
	})
	require.NoError(t, broadcaster.broadcastEvent(context.Background(), event, ws.ActionExecutorProfileUpdated))
	message := <-hub.broadcast
	data, err := json.Marshal(message)
	require.NoError(t, err)
	require.NotContains(t, string(data), "private-host")
	require.NotContains(t, string(data), "private-parent")
}

func TestExecutorOwnershipNotificationsUseRecipientIdentity(t *testing.T) {
	hub := newTestHub(t)
	owner := &Client{ID: "owner", identity: authn.Identity{UserID: "alice"}, send: make(chan []byte, 4), logger: testLogger()}
	other := &Client{ID: "other", identity: authn.Identity{UserID: "bob"}, send: make(chan []byte, 4), logger: testLogger()}
	hub.clients[owner] = true
	hub.clients[other] = true
	hub.setAuthPolicy(AuthPolicy{ExecutorConfiguration: func(ctx context.Context, _, _ string) error {
		identity, _ := authn.IdentityFromContext(ctx)
		if actor, bound := models.ExecutorActorFromContext(ctx); bound {
			identity.UserID = actor
		}
		if identity.UserID == "alice" {
			return nil
		}
		return errors.New("private executor")
	}})
	msg, err := ws.NewNotification(ws.ActionExecutorProfileUpdated, map[string]any{"id": "private", "executor_id": "parent", "config": map[string]string{"host": "private-host"}})
	require.NoError(t, err)
	hub.sendExecutorConfiguration(models.WithExecutorActor(t.Context(), "alice"), "parent", "private", msg)
	require.Len(t, owner.send, 1)
	require.Empty(t, other.send)
}
