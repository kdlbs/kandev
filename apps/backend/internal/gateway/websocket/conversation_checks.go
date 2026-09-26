package websocket

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func (h *Hub) runConversationChecks(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.checkConversationSubscriptions(ctx)
		}
	}
}

func (h *Hub) checkConversationSubscriptions(ctx context.Context) {
	h.mu.RLock()
	reader := h.conversationSourceReader
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()
	if reader == nil {
		return
	}
	// Read each subscribed session once per pass, regardless of panel count.
	revisions := make(map[string]models.ConversationRevision)
	for _, client := range clients {
		client.mu.RLock()
		subscriptions := make([]conversationSubscription, 0, len(client.conversationSubscriptions))
		for _, subscription := range client.conversationSubscriptions {
			subscriptions = append(subscriptions, subscription)
		}
		client.mu.RUnlock()
		for _, subscription := range subscriptions {
			if ctx.Err() != nil {
				return
			}
			if !h.conversationAuthorized(client, subscription) {
				h.terminateConversation(client, subscription)
				continue
			}
			revision, ok := revisions[subscription.SessionID]
			if !ok {
				var err error
				revision, err = reader.ReadConversationRevision(ctx, subscription.SessionID)
				if err != nil {
					continue
				}
				revisions[subscription.SessionID] = revision
			}
			if !revision.Exists {
				h.terminateConversation(client, subscription)
				continue
			}
			h.sendConversationPayload(client, subscription, conversationChangedPayload{
				ProtocolVersion: 2, ScopeID: subscription.ScopeID, SessionID: subscription.SessionID,
				Epoch: subscription.Epoch, BaseRevision: formatRevision(revision.Revision), Revision: formatRevision(revision.Revision),
				Check: true, Operations: []conversationChangedOperation{},
			})
		}
	}
}

func (h *Hub) conversationAuthorized(client *Client, subscription conversationSubscription) bool {
	h.mu.RLock()
	policy, service := h.authPolicy, h.pluginConversationService
	h.mu.RUnlock()
	ctx := client.dispatchContext()
	// Synthetic and identity-less clients are the unscoped single-user mode.
	// They do not have an account to revalidate, so an account-backed active
	// user hook must not terminate their conversation subscriptions.
	if identityIsScoped(client.identity) && policy.ActiveUser != nil && !policy.ActiveUser(ctx, subscription.UserID) {
		return false
	}
	if policy.Subscriptions.Session != nil && policy.Subscriptions.Session(ctx, subscription.SessionID) != nil {
		return false
	}
	if subscription.ConsumerKind != conversationConsumerPlugin {
		return true
	}
	if service == nil {
		return false
	}
	if subscription.Managed != nil {
		return service.ValidateManagedConversationIdentity(ctx, *subscription.Managed)
	}
	record, err := service.Get(subscription.PluginID)
	return err == nil && record.Status == plugins.StatusActive && record.Capabilities.CanRead("messages") &&
		record.InstalledAt.UnixMicro() == subscription.Generation
}

func (h *Hub) terminateConversation(client *Client, subscription conversationSubscription) {
	client.mu.Lock()
	current, ok := client.conversationSubscriptions[subscription.ScopeID]
	if !ok || current.instance != subscription.instance {
		client.mu.Unlock()
		return
	}
	delete(client.conversationSubscriptions, subscription.ScopeID)
	client.mu.Unlock()
	payload := conversationChangedPayload{
		ProtocolVersion: 2, ScopeID: subscription.ScopeID, SessionID: subscription.SessionID, Epoch: subscription.Epoch,
		BaseRevision: "0", Revision: "0", Terminal: true, Operations: []conversationChangedOperation{},
	}
	message, _ := ws.NewNotification(ws.ActionSessionConversationChanged, payload)
	encoded, err := json.Marshal(message)
	if err == nil {
		client.sendNotification(encoded, ws.ActionSessionConversationChanged)
	}
}

func (h *Hub) sendConversationPayload(client *Client, subscription conversationSubscription, payload conversationChangedPayload) {
	if !h.conversationAuthorized(client, subscription) {
		h.terminateConversation(client, subscription)
		return
	}
	client.mu.RLock()
	current, ok := client.conversationSubscriptions[subscription.ScopeID]
	client.mu.RUnlock()
	if !ok || current.instance != subscription.instance {
		return
	}
	message, err := ws.NewNotification(ws.ActionSessionConversationChanged, payload)
	if err != nil {
		return
	}
	encoded, err := json.Marshal(message)
	if err == nil {
		client.sendNotification(encoded, ws.ActionSessionConversationChanged)
	}
}
