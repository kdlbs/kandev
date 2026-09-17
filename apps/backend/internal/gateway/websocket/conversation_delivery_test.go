package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type conversationSourceFixture struct {
	reads    *atomic.Int64
	session  *models.TaskSession
	revision models.ConversationRevision
}

func (f conversationSourceFixture) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	if f.session == nil || f.session.ID != id {
		return nil, models.ErrTaskSessionNotFound
	}
	return f.session, nil
}

func (f conversationSourceFixture) ReadConversationRevision(context.Context, string) (models.ConversationRevision, error) {
	if f.reads != nil {
		f.reads.Add(1)
	}
	return f.revision, nil
}

func TestConversationDeliveryProjectsSelectedUpsertAndCoverage(t *testing.T) {
	hub := NewHub(ws.NewDispatcher(), testLogger())
	client := NewClient("conversation-client", authn.Identity{UserID: "user-1"}, nil, hub, testLogger())
	hub.clients[client] = true
	registry := plugins.NewRegistry()
	installedAt := time.Now()
	registry.Add(&store.Record{Manifest: manifest.Manifest{ID: "plugin-1", Capabilities: manifest.Capabilities{APIRead: []string{"messages"}}}, Status: plugins.StatusActive, InstalledAt: installedAt})
	hub.SetPluginConversationService(plugins.NewService(nil, registry, nil, testLogger()))
	taskID := "task-selected"
	client.conversationSubscriptions["scope-1"] = conversationSubscription{
		ScopeID: "scope-1", SessionID: "session-1", ConsumerKind: conversationConsumerPlugin,
		PluginID: "plugin-1", Generation: installedAt.UnixMicro(), UserID: "user-1", TaskID: &taskID, Sort: "asc", Epoch: "epoch-1",
	}

	message := &models.Message{
		ID: "message-1", TaskSessionID: "session-1", TaskID: taskID,
		AuthorType: models.MessageAuthorUser, Content: "prompt",
		CreatedAt: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
	}
	hub.BroadcastConversationMutation(&models.ConversationMutationReceipt{
		SessionID: "session-1", BaseRevision: 4, Revision: 5, Complete: true,
		Operations: []models.ConversationMutationOperation{{
			Kind: models.ConversationMutationUpsert, Entity: models.ConversationEntityMessage,
			ID: message.ID, SessionID: message.TaskSessionID, TaskID: taskID,
			AuthorType: string(message.AuthorType), Message: message,
		}},
	})

	var frame ws.Message
	if err := json.Unmarshal(<-client.send, &frame); err != nil {
		t.Fatalf("decode changed frame: %v", err)
	}
	if frame.Action != ws.ActionSessionConversationChanged {
		t.Fatalf("action = %q", frame.Action)
	}
	var payload conversationChangedPayload
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		t.Fatalf("decode changed payload: %v", err)
	}
	if payload.BaseRevision != "4" || payload.Revision != "5" || len(payload.Operations) != 1 || payload.Operations[0].ID != message.ID {
		t.Fatalf("changed payload = %+v", payload)
	}
}

func TestConversationDeliveryProjectsCoreEntitiesAtTheAPIBoundary(t *testing.T) {
	hub := NewHub(ws.NewDispatcher(), testLogger())
	client := NewClient("conversation-client", authn.Identity{UserID: "user-1"}, nil, hub, testLogger())
	hub.clients[client] = true
	client.conversationSubscriptions["scope-1"] = conversationSubscription{
		ScopeID: "scope-1", SessionID: "session-1", ConsumerKind: conversationConsumerCore,
		UserID: "user-1", Epoch: "epoch-1",
	}
	message := &models.Message{
		ID: "message-core", TaskSessionID: "session-1", TaskID: "task-1",
		AuthorType: models.MessageAuthorAgent,
		Content:    "<kandev-system>private prompt</kandev-system>visible",
		Metadata:   map[string]any{"normalized": map[string]any{"shell_exec": map[string]any{"output": map[string]any{"stdout": "secret shell body"}}}},
		CreatedAt:  time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
	}
	hub.BroadcastConversationMutation(&models.ConversationMutationReceipt{
		SessionID: "session-1", BaseRevision: 1, Revision: 2, Complete: true,
		Operations: []models.ConversationMutationOperation{{
			Kind: models.ConversationMutationUpsert, Entity: models.ConversationEntityMessage,
			ID: message.ID, SessionID: message.TaskSessionID, TaskID: message.TaskID, Message: message,
		}},
	})

	var frame ws.Message
	if err := json.Unmarshal(<-client.send, &frame); err != nil {
		t.Fatalf("decode changed frame: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		t.Fatalf("decode changed payload: %v", err)
	}
	operation := payload["operations"].([]any)[0].(map[string]any)
	projected := operation["message"].(map[string]any)
	if projected["content"] != "visible" {
		t.Fatalf("core content = %#v, want visible content", projected["content"])
	}
	encoded, _ := json.Marshal(projected)
	if strings.Contains(string(encoded), "secret shell body") {
		t.Fatal("core message leaked shell metadata at the transport boundary")
	}
	if message.Content == "visible" {
		t.Fatal("core projection mutated the source message")
	}
}

func TestConversationDeliverySendsCoverageOnlyForIrrelevantMutation(t *testing.T) {
	hub := NewHub(ws.NewDispatcher(), testLogger())
	client := NewClient("conversation-client", authn.Identity{UserID: "user-1"}, nil, hub, testLogger())
	hub.clients[client] = true
	registry := plugins.NewRegistry()
	installedAt := time.Now()
	registry.Add(&store.Record{Manifest: manifest.Manifest{ID: "plugin-1", Capabilities: manifest.Capabilities{APIRead: []string{"messages"}}}, Status: plugins.StatusActive, InstalledAt: installedAt})
	hub.SetPluginConversationService(plugins.NewService(nil, registry, nil, testLogger()))
	taskID := "task-selected"
	client.conversationSubscriptions["scope-1"] = conversationSubscription{
		ScopeID: "scope-1", SessionID: "session-1", ConsumerKind: conversationConsumerPlugin,
		PluginID: "plugin-1", Generation: installedAt.UnixMicro(), UserID: "user-1", TaskID: &taskID, Epoch: "epoch-1",
	}
	hub.BroadcastConversationMutation(&models.ConversationMutationReceipt{
		SessionID: "session-1", BaseRevision: 8, Revision: 9, Complete: true,
		Operations: []models.ConversationMutationOperation{{
			Kind: models.ConversationMutationUpsert, Entity: models.ConversationEntityMessage,
			ID: "message-other", SessionID: "session-1", TaskID: "task-other",
			AuthorType: string(models.MessageAuthorAgent),
		}},
	})

	var frame ws.Message
	if err := json.Unmarshal(<-client.send, &frame); err != nil {
		t.Fatalf("decode coverage frame: %v", err)
	}
	var payload conversationChangedPayload
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		t.Fatalf("decode coverage payload: %v", err)
	}
	if len(payload.Operations) != 0 || payload.BaseRevision != "8" || payload.Revision != "9" {
		t.Fatalf("coverage payload = %+v", payload)
	}
}

func TestConversationDeliveryResetsIncompleteReceipt(t *testing.T) {
	hub := NewHub(ws.NewDispatcher(), testLogger())
	client := NewClient("conversation-client", authn.Identity{UserID: "user-1"}, nil, hub, testLogger())
	hub.clients[client] = true
	client.conversationSubscriptions["scope-1"] = conversationSubscription{
		ScopeID: "scope-1", SessionID: "session-1", ConsumerKind: conversationConsumerCore,
		UserID: "user-1", Epoch: "epoch-1",
	}
	hub.BroadcastConversationMutation(&models.ConversationMutationReceipt{
		SessionID: "session-1", BaseRevision: 2, Revision: 4,
		Complete: false,
	})

	var frame ws.Message
	if err := json.Unmarshal(<-client.send, &frame); err != nil {
		t.Fatalf("decode reset frame: %v", err)
	}
	var payload conversationChangedPayload
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		t.Fatalf("decode reset payload: %v", err)
	}
	if !payload.Reset || len(payload.Operations) != 0 {
		t.Fatalf("reset payload = %+v", payload)
	}
}

func TestConversationSubscribeReturnsSourceRevisionForCore(t *testing.T) {
	hub := NewHub(ws.NewDispatcher(), testLogger())
	hub.SetConversationSourceReader(conversationSourceFixture{
		session:  &models.TaskSession{ID: "session-1", TaskID: "task-1"},
		revision: models.ConversationRevision{Exists: true, Revision: 17},
	})
	client := NewClient("conversation-client", authn.Identity{UserID: "user-1"}, nil, hub, testLogger())
	request, err := ws.NewRequest("request-1", ws.ActionSessionConversationSubscribe, map[string]any{
		"scope_id": "core:web:scope-1", "session_id": "session-1", "consumer_kind": "core",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	client.handleConversationSubscribe(request)
	var response ws.Message
	if err := json.Unmarshal(<-client.controlSend, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["revision"] != "17" || payload["epoch"] == "" || payload["success"] != true {
		t.Fatalf("subscribe payload = %+v", payload)
	}
}

func TestConversationDeliveryStopsAfterRevocation(t *testing.T) {
	for _, reason := range []string{"disabled", "capability", "generation", "session"} {
		t.Run(reason, func(t *testing.T) {
			hub := NewHub(ws.NewDispatcher(), testLogger())
			registry := plugins.NewRegistry()
			record := &store.Record{Manifest: manifest.Manifest{ID: "plugin-1", Capabilities: manifest.Capabilities{APIRead: []string{"messages"}}}, Status: plugins.StatusActive, InstalledAt: time.Now()}
			registry.Add(record)
			hub.SetPluginConversationService(plugins.NewService(nil, registry, nil, testLogger()))
			client := NewClient("client", authn.Identity{UserID: "user-1"}, nil, hub, testLogger())
			hub.clients[client] = true
			client.conversationSubscriptions["scope"] = conversationSubscription{ScopeID: "scope", SessionID: "session-1", ConsumerKind: conversationConsumerPlugin, PluginID: "plugin-1", Generation: record.InstalledAt.UnixMicro(), UserID: "user-1"}
			switch reason {
			case "disabled":
				record.Status = plugins.StatusDisabled
			case "capability":
				record.Capabilities.APIRead = nil
			case "generation":
				record.InstalledAt = record.InstalledAt.Add(time.Second)
			case "session":
				hub.authPolicy.Subscriptions.Session = func(context.Context, string) error { return errors.New("revoked") }
			}
			registry.Add(record)
			hub.BroadcastConversationMutation(&models.ConversationMutationReceipt{SessionID: "session-1", BaseRevision: 0, Revision: 1, Complete: true})
			if len(client.conversationSubscriptions) != 0 {
				t.Fatal("revoked subscription remains active")
			}
			select {
			case frame := <-client.send:
				var message ws.Message
				if err := json.Unmarshal(frame, &message); err != nil {
					t.Fatal(err)
				}
				var payload map[string]any
				if err := json.Unmarshal(message.Payload, &payload); err != nil {
					t.Fatal(err)
				}
				if payload["terminal"] != true {
					t.Fatalf("expected terminal notification, got %s", frame)
				}
			default:
				t.Fatal("missing terminal notification")
			}
		})
	}
}

func TestConversationDeliveryKeepsSyntheticSubscriptionWhenActiveUserCheckFails(t *testing.T) {
	hub := NewHub(ws.NewDispatcher(), testLogger())
	hub.setAuthPolicy(AuthPolicy{ActiveUser: func(context.Context, string) bool { return false }})
	client := NewClient("synthetic-client", authn.Identity{UserID: "default-user", Synthetic: true}, nil, hub, testLogger())
	hub.clients[client] = true
	client.conversationSubscriptions["scope-1"] = conversationSubscription{
		ScopeID: "scope-1", SessionID: "session-1", ConsumerKind: conversationConsumerCore, Epoch: "epoch-1",
	}

	hub.BroadcastConversationMutation(&models.ConversationMutationReceipt{
		SessionID: "session-1", BaseRevision: 1, Revision: 2, Complete: true,
	})

	var frame ws.Message
	if err := json.Unmarshal(<-client.send, &frame); err != nil {
		t.Fatalf("decode changed frame: %v", err)
	}
	if frame.Action != ws.ActionSessionConversationChanged {
		t.Fatalf("action = %q", frame.Action)
	}
	var payload conversationChangedPayload
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		t.Fatalf("decode changed payload: %v", err)
	}
	if payload.Terminal || payload.Revision != "2" {
		t.Fatalf("changed payload = %+v", payload)
	}
}

func TestConversationRevisionWorkerChecksIdleSubscriptions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub := NewHub(ws.NewDispatcher(), testLogger())
		reads := &atomic.Int64{}
		hub.SetConversationSourceReader(conversationSourceFixture{reads: reads, revision: models.ConversationRevision{Exists: true, Revision: 7}})
		client := NewClient("client", authn.Identity{}, nil, hub, testLogger())
		hub.clients[client] = true
		client.conversationSubscriptions["scope"] = conversationSubscription{ScopeID: "scope", SessionID: "session-1", ConsumerKind: conversationConsumerCore, Epoch: "epoch"}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go hub.Run(ctx)
		synctest.Wait()
		time.Sleep(5 * time.Second)
		synctest.Wait()
		select {
		case frame := <-client.send:
			var message ws.Message
			if err := json.Unmarshal(frame, &message); err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal(message.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["check"] != true || payload["revision"] != "7" {
				t.Fatalf("unexpected revision check: %s", frame)
			}
		default:
			t.Fatal("idle subscriber never receives source revision")
		}
		client.mu.Lock()
		delete(client.conversationSubscriptions, "scope")
		client.mu.Unlock()
		time.Sleep(5 * time.Second)
		synctest.Wait()
		if reads.Load() != 1 {
			t.Fatalf("reads after unsubscribe = %d", reads.Load())
		}
		cancel()
		synctest.Wait()
	})
}
