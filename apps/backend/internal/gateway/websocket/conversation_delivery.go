package websocket

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	conversationConsumerPlugin = "plugin"
	conversationConsumerCore   = "core"
)

type conversationSubscription struct {
	instance     string
	ScopeID      string
	SessionID    string
	ConsumerKind string
	PluginID     string
	Generation   int64
	UserID       string
	TaskID       *string
	Authors      []string
	Sort         string
	Epoch        string
}

type conversationSubscribeRequest struct {
	SessionID    string   `json:"session_id"`
	ScopeID      string   `json:"scope_id"`
	ConsumerKind string   `json:"consumer_kind"`
	PluginID     string   `json:"plugin_id,omitempty"`
	Generation   int64    `json:"generation,omitempty"`
	BindingToken string   `json:"binding_token,omitempty"`
	TaskID       *string  `json:"task_id,omitempty"`
	Authors      []string `json:"authors,omitempty"`
	Sort         string   `json:"sort,omitempty"`
}

type conversationChangedPayload struct {
	ProtocolVersion int                            `json:"protocol_version"`
	ScopeID         string                         `json:"scope_id"`
	SessionID       string                         `json:"session_id"`
	Epoch           string                         `json:"epoch"`
	BaseRevision    string                         `json:"base_revision"`
	Revision        string                         `json:"revision"`
	Check           bool                           `json:"check,omitempty"`
	Terminal        bool                           `json:"terminal,omitempty"`
	Reset           bool                           `json:"reset,omitempty"`
	Operations      []conversationChangedOperation `json:"operations"`
}

type conversationChangedOperation struct {
	Kind    models.ConversationMutationKind `json:"kind"`
	Entity  models.ConversationEntityKind   `json:"entity"`
	ID      string                          `json:"id"`
	Message any                             `json:"message,omitempty"`
	Turn    any                             `json:"turn,omitempty"`
}

func (c *Client) handleConversationSubscribe(msg *ws.Message) {
	var req conversationSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendConversationFailure(msg, req.SessionID, "invalid_request", "invalid conversation subscription", false)
		return
	}
	if req.SessionID == "" || req.ScopeID == "" {
		c.sendConversationFailure(msg, req.SessionID, "invalid_request", "session_id and scope_id are required", false)
		return
	}
	if req.ConsumerKind != conversationConsumerPlugin && req.ConsumerKind != conversationConsumerCore {
		c.sendConversationFailure(msg, req.SessionID, "invalid_request", "unsupported conversation consumer", false)
		return
	}
	if !c.maySubscribeSession(msg, req.SessionID) {
		return
	}
	service := c.hub.pluginConversationService
	userID := c.ownUserTopic()
	if !c.authorizeConversationIdentity(msg, req, service, userID) {
		return
	}
	revision, ok := c.readConversationSubscription(msg, req)
	if !ok {
		return
	}
	h := c.hub

	sortOrder := strings.ToLower(req.Sort)
	if sortOrder == "" {
		sortOrder = "asc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		c.sendConversationFailure(msg, req.SessionID, "invalid_request", "sort must be asc or desc", false)
		return
	}
	epoch := h.conversationEpoch
	if req.ConsumerKind == conversationConsumerPlugin {
		epoch = service.ConversationEpoch()
	}
	subscription := conversationSubscription{
		instance: uuid.NewString(),
		ScopeID:  req.ScopeID, SessionID: req.SessionID, ConsumerKind: req.ConsumerKind,
		PluginID: req.PluginID, Generation: req.Generation, UserID: userID,
		TaskID: cloneOptionalString(req.TaskID), Authors: append([]string(nil), req.Authors...),
		Sort: sortOrder, Epoch: epoch,
	}
	c.mu.Lock()
	c.conversationSubscriptions[req.ScopeID] = subscription
	c.mu.Unlock()
	response, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"success": true, "protocol_version": 2, "scope_id": req.ScopeID,
		"session_id": req.SessionID, "epoch": epoch, "revision": formatRevision(revision.Revision),
	})
	c.sendMessage(response)
}

func (c *Client) authorizeConversationIdentity(msg *ws.Message, req conversationSubscribeRequest, service *plugins.Service, userID string) bool {
	if req.ConsumerKind == conversationConsumerCore {
		if req.PluginID != "" || req.Generation != 0 || req.BindingToken != "" {
			c.sendConversationFailure(msg, req.SessionID, "invalid_request", "invalid core conversation identity", false)
			return false
		}
		return true
	}
	if service == nil || req.PluginID == "" || req.Generation == 0 || req.BindingToken == "" {
		c.sendConversationFailure(msg, req.SessionID, "invalid_request", "invalid plugin conversation identity", false)
		return false
	}
	if err := service.AuthorizeConversationConsumer(req.PluginID, userID, req.Generation, req.BindingToken); err != nil {
		code, retryable := "invalid_binding", false
		if errors.Is(err, plugins.ErrConversationGenerationSuperseded) {
			code, retryable = "generation_superseded", true
		}
		c.sendConversationFailure(msg, req.SessionID, code, "plugin conversation binding rejected", retryable)
		return false
	}
	return true
}

func (c *Client) readConversationSubscription(msg *ws.Message, req conversationSubscribeRequest) (models.ConversationRevision, bool) {
	h := c.hub
	h.mu.RLock()
	reader := h.conversationSourceReader
	h.mu.RUnlock()
	if reader == nil {
		c.sendConversationFailure(msg, req.SessionID, "upstream_failure", "conversation source unavailable", true)
		return models.ConversationRevision{}, false
	}
	session, err := reader.GetTaskSession(c.dispatchContext(), req.SessionID)
	if err != nil || session == nil || session.ID != req.SessionID {
		c.sendConversationFailure(msg, req.SessionID, "not_found", "task session not found", false)
		return models.ConversationRevision{}, false
	}
	if req.TaskID != nil && *req.TaskID != session.TaskID {
		c.sendConversationFailure(msg, req.SessionID, "invalid_request", "task_id does not match the task session", false)
		return models.ConversationRevision{}, false
	}
	revision, err := reader.ReadConversationRevision(c.dispatchContext(), req.SessionID)
	if err != nil {
		c.sendConversationFailure(msg, req.SessionID, "upstream_failure", "conversation source unavailable", true)
		return models.ConversationRevision{}, false
	}
	if !revision.Exists {
		c.sendConversationFailure(msg, req.SessionID, "not_found", "task session not found", false)
		return models.ConversationRevision{}, false
	}
	return revision, true
}

func (c *Client) handleConversationUnsubscribe(msg *ws.Message) {
	var req conversationSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil || req.ScopeID == "" {
		c.sendConversationFailure(msg, req.SessionID, "invalid_request", "scope_id is required", false)
		return
	}
	c.mu.Lock()
	subscription, ok := c.conversationSubscriptions[req.ScopeID]
	if ok && (req.SessionID == "" || subscription.SessionID == req.SessionID) {
		delete(c.conversationSubscriptions, req.ScopeID)
	} else {
		ok = false
	}
	c.mu.Unlock()
	if !ok {
		c.sendConversationFailure(msg, req.SessionID, "invalid_request", "conversation subscription not found", false)
		return
	}
	response, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"success": true, "protocol_version": 2, "scope_id": req.ScopeID,
		"session_id": subscription.SessionID,
	})
	c.sendMessage(response)
}

func (c *Client) sendConversationFailure(msg *ws.Message, sessionID, code, message string, retryable bool) {
	payload := map[string]any{
		"success": false,
		"error":   map[string]any{"code": code, "message": message, "retryable": retryable},
	}
	if sessionID != "" {
		payload["session_id"] = sessionID
	}
	response, _ := ws.NewResponse(msg.ID, msg.Action, payload)
	c.sendMessage(response)
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// BroadcastConversationMutation projects a committed source receipt to every
// matching Host-only subscription. It accepts the bus event data so NATS and
// memory buses can both carry either a typed receipt or its JSON map form.
func (h *Hub) BroadcastConversationMutation(data any) {
	receipt, hasReceipt := conversationReceiptFromData(data)
	sessionID := ""
	if receipt != nil {
		sessionID = receipt.SessionID
	} else {
		sessionID = extractSessionID(data)
	}
	if sessionID == "" {
		return
	}
	for _, client := range h.conversationRecipients(sessionID) {
		client.mu.RLock()
		subscriptions := make([]conversationSubscription, 0, len(client.conversationSubscriptions))
		for _, subscription := range client.conversationSubscriptions {
			if subscription.SessionID == sessionID {
				subscriptions = append(subscriptions, subscription)
			}
		}
		client.mu.RUnlock()
		for _, subscription := range subscriptions {
			payload := h.conversationChanged(subscription, receipt, hasReceipt)
			h.sendConversationPayload(client, subscription, payload)
		}
	}
}

func (h *Hub) conversationRecipients(sessionID string) []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		client.mu.RLock()
		matched := false
		for _, subscription := range client.conversationSubscriptions {
			if subscription.SessionID == sessionID {
				matched = true
				break
			}
		}
		client.mu.RUnlock()
		if matched {
			clients = append(clients, client)
		}
	}
	return clients
}

func (h *Hub) conversationChanged(
	subscription conversationSubscription,
	receipt *models.ConversationMutationReceipt,
	hasReceipt bool,
) conversationChangedPayload {
	payload := conversationChangedPayload{
		ProtocolVersion: 2, ScopeID: subscription.ScopeID, SessionID: subscription.SessionID,
		Epoch: subscription.Epoch, Operations: []conversationChangedOperation{},
	}
	if !hasReceipt || receipt == nil || !receipt.Complete {
		payload.Reset = true
		if receipt != nil {
			payload.BaseRevision = formatRevision(receipt.BaseRevision)
			payload.Revision = formatRevision(receipt.Revision)
		}
		return payload
	}
	payload.BaseRevision = formatRevision(receipt.BaseRevision)
	payload.Revision = formatRevision(receipt.Revision)
	for _, operation := range receipt.Operations {
		if !conversationOperationSelected(subscription, operation) {
			continue
		}
		projected, ok := projectConversationOperation(subscription, operation)
		if !ok {
			payload.Reset = true
			payload.Operations = []conversationChangedOperation{}
			return payload
		}
		payload.Operations = append(payload.Operations, projected)
	}
	return payload
}

func formatRevision(value int64) string {
	return strconv.FormatInt(value, 10)
}

func conversationOperationSelected(subscription conversationSubscription, operation models.ConversationMutationOperation) bool {
	if operation.SessionID != subscription.SessionID {
		return false
	}
	if subscription.TaskID != nil && *subscription.TaskID != operation.TaskID {
		return false
	}
	if operation.Entity == models.ConversationEntityMessage && len(subscription.Authors) > 0 {
		for _, author := range subscription.Authors {
			if author == operation.AuthorType {
				return true
			}
		}
		return false
	}
	return operation.Entity == models.ConversationEntityMessage || operation.Entity == models.ConversationEntityTurn
}

func projectConversationOperation(subscription conversationSubscription, operation models.ConversationMutationOperation) (conversationChangedOperation, bool) {
	result := conversationChangedOperation{Kind: operation.Kind, Entity: operation.Entity, ID: operation.ID}
	if operation.Kind == models.ConversationMutationRemove {
		return result, true
	}
	if operation.Kind != models.ConversationMutationUpsert {
		return result, false
	}
	switch operation.Entity {
	case models.ConversationEntityMessage:
		if operation.Message == nil {
			return result, false
		}
		if subscription.ConsumerKind == conversationConsumerPlugin {
			result.Message = safePluginConversationMessage(operation.Message)
		} else {
			result.Message = operation.Message
		}
	case models.ConversationEntityTurn:
		if operation.Turn == nil {
			return result, false
		}
		if subscription.ConsumerKind == conversationConsumerPlugin {
			result.Turn = safePluginConversationTurn(operation.Turn)
		} else {
			result.Turn = operation.Turn
		}
	default:
		return result, false
	}
	return result, true
}

func safePluginConversationMessage(message *models.Message) map[string]any {
	payload := map[string]any{
		"id": message.ID, "sessionId": message.TaskSessionID, "authorType": string(message.AuthorType),
		"type": string(message.Type), "content": sysprompt.StripSystemContent(message.Content),
		"createdAt": message.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updatedAt": message.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if message.TaskID != "" {
		payload["taskId"] = message.TaskID
	}
	if message.TurnID != "" {
		payload["turnId"] = message.TurnID
	}
	if message.PromptIndex > 0 {
		payload["promptIndex"] = message.PromptIndex
	}
	if senderTaskID, ok := message.Metadata["sender_task_id"].(string); ok && senderTaskID != "" {
		payload["senderTaskId"] = senderTaskID
	}
	return payload
}

func safePluginConversationTurn(turn *models.Turn) map[string]any {
	payload := map[string]any{
		"id": turn.ID, "sessionId": turn.TaskSessionID, "startedAt": turn.StartedAt.UTC().Format(time.RFC3339Nano),
		"updatedAt": turn.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if turn.TaskID != "" {
		payload["taskId"] = turn.TaskID
	}
	if turn.CompletedAt != nil {
		payload["completedAt"] = turn.CompletedAt.UTC().Format(time.RFC3339Nano)
	}
	return payload
}

func conversationReceiptFromData(data any) (*models.ConversationMutationReceipt, bool) {
	if receipt, ok := data.(*models.ConversationMutationReceipt); ok {
		return receipt, true
	}
	if receipt, ok := data.(models.ConversationMutationReceipt); ok {
		return &receipt, true
	}
	if payload, ok := data.(map[string]any); ok {
		value, exists := payload["conversation_receipt"]
		if !exists {
			return nil, false
		}
		return conversationReceiptFromData(value)
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, false
	}
	var receipt models.ConversationMutationReceipt
	if err := json.Unmarshal(encoded, &receipt); err != nil || receipt.SessionID == "" {
		return nil, false
	}
	return &receipt, true
}
