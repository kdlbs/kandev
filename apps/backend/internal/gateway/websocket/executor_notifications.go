package websocket

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/kandev/kandev/internal/auth/authn"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const executorCatalogueInvalidated = "executor.catalogue.invalidated"

func isExecutorConfigurationAction(action string) bool {
	switch action {
	case ws.ActionExecutorCreated, ws.ActionExecutorUpdated, ws.ActionExecutorDeleted,
		ws.ActionExecutorProfileCreated, ws.ActionExecutorProfileUpdated, ws.ActionExecutorProfileDeleted,
		ws.ActionExecutorReachabilityChanged:
		return true
	}
	return false
}

func (h *Hub) broadcastExecutorConfiguration(ctx context.Context, data any, msg *ws.Message) error {
	executorID := extractStringField(data, "id")
	profileID := ""
	if strings.HasPrefix(msg.Action, "executor.profile.") {
		profileID = executorID
		executorID = extractStringField(data, "executor_id")
	} else if msg.Action == ws.ActionExecutorReachabilityChanged {
		executorID = extractStringField(data, "executor_id")
	}
	h.sendExecutorConfiguration(ctx, executorID, profileID, msg)
	// Every recipient refetches its own catalogue, including clients that lost
	// access and therefore did not receive the resource-bearing notification.
	invalidation, err := ws.NewNotification(executorCatalogueInvalidated, map[string]any{})
	if err != nil {
		return err
	}
	//ws:global All clients need this payload-free notice to remove revoked configuration.
	h.Broadcast(invalidation)
	return nil
}

func (h *Hub) sendExecutorConfiguration(ctx context.Context, executorID, profileID string, msg *ws.Message) {
	check := h.authPolicy.ExecutorConfiguration
	if check == nil {
		return
	}
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()
	encoded, err := json.Marshal(msg)
	if err != nil {
		return
	}
	for _, client := range clients {
		caller := authn.WithIdentity(context.Background(), client.identity)
		if check(caller, executorID, profileID) == nil {
			h.sendToClients(encoded, []*Client{client}, msg.Action)
		}
	}
}
