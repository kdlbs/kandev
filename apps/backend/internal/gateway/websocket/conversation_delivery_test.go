package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/pluginsdk"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type conversationSourceFixture struct {
	reads    *atomic.Int64
	session  *models.TaskSession
	revision models.ConversationRevision
}

type managedConversationGatewayStub struct {
	descriptor pluginsdk.AgentConversationDescriptor
}

func (s *managedConversationGatewayStub) Ensure(context.Context, string, pluginsdk.AgentConversationSpec) (pluginsdk.AgentConversationDescriptor, string, error) {
	return pluginsdk.AgentConversationDescriptor{}, "", nil
}

func (s *managedConversationGatewayStub) Dispatch(context.Context, string, string, string, string, string) (pluginsdk.AgentConversationDispatch, error) {
	return pluginsdk.AgentConversationDispatch{}, nil
}

func (s *managedConversationGatewayStub) Delete(context.Context, string, string, string) (int32, error) {
	return 0, nil
}

func (s *managedConversationGatewayStub) DeleteAllForPlugin(context.Context, string) (int32, error) {
	return 0, nil
}

func (s *managedConversationGatewayStub) ResolveManagedConversation(context.Context, string, string, string) (pluginsdk.AgentConversationDescriptor, error) {
	return s.descriptor, nil
}

func (f conversationSourceFixture) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	if f.session == nil || f.session.ID != id {
		return nil, models.ErrTaskSessionNotFound
	}
	return f.session, nil
}

func (conversationSourceFixture) ListMessagesPaginated(context.Context, taskservice.ListMessagesRequest) ([]*models.Message, bool, error) {
	return nil, false, nil
}

func (conversationSourceFixture) ListTurnsBySession(context.Context, string) ([]*models.Turn, error) {
	return nil, nil
}

func (f conversationSourceFixture) ReadConversationRevision(context.Context, string) (models.ConversationRevision, error) {
	if f.reads != nil {
		f.reads.Add(1)
	}
	return f.revision, nil
}

func (f conversationSourceFixture) AuthorizeWorkspaceAccess(context.Context, string) error {
	return nil
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

func TestConversationDeliveryProjectsSafeCoreTurnMetadataAndCompletion(t *testing.T) {
	hub := NewHub(ws.NewDispatcher(), testLogger())
	client := NewClient("conversation-client", authn.Identity{UserID: "user-1"}, nil, hub, testLogger())
	hub.clients[client] = true
	client.conversationSubscriptions["scope-1"] = conversationSubscription{
		ScopeID: "scope-1", SessionID: "session-1", ConsumerKind: conversationConsumerCore,
		UserID: "user-1", Epoch: "epoch-1",
	}
	hadOutput := false
	turn := &models.Turn{
		ID: "turn-core", TaskSessionID: "session-1", TaskID: "task-1",
		Metadata: map[string]any{
			models.TurnMetaKeyRuntimeConfigSnapshot: models.TurnRuntimeConfigSnapshot{Model: "mock-fast"},
			models.TurnMetaKeyPromptDispatchPending: true,
			"internal":                              "private turn state",
		},
	}
	hub.BroadcastConversationMutation(&models.ConversationMutationReceipt{
		SessionID: "session-1", BaseRevision: 1, Revision: 2, Complete: true,
		Operations: []models.ConversationMutationOperation{{
			Kind: models.ConversationMutationUpsert, Entity: models.ConversationEntityTurn,
			ID: turn.ID, SessionID: turn.TaskSessionID, TaskID: turn.TaskID,
			Turn: turn, HadOutput: &hadOutput,
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
	projected := operation["turn"].(map[string]any)
	if projected["had_output"] != false {
		t.Fatalf("core turn had_output = %#v, want false", projected["had_output"])
	}
	metadata := projected["metadata"].(map[string]any)
	if metadata[models.TurnMetaKeyRuntimeConfigSnapshot] == nil {
		t.Fatal("core turn dropped runtime configuration metadata")
	}
	if _, ok := metadata[models.TurnMetaKeyPromptDispatchPending]; ok {
		t.Fatal("core turn exposed prompt-dispatch metadata")
	}
	if _, ok := metadata["internal"]; ok {
		t.Fatal("core turn exposed private metadata")
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

func TestManagedConversationV2SubscriptionIsolation(t *testing.T) {
	hub := NewHub(ws.NewDispatcher(), testLogger())
	registry := plugins.NewRegistry()
	installedAt := time.Date(2026, 9, 19, 14, 0, 0, 0, time.UTC)
	registry.Add(&store.Record{
		Manifest: manifest.Manifest{ID: "managed-plugin", Capabilities: manifest.Capabilities{AgentConversation: true}},
		Status:   plugins.StatusActive, InstalledAt: installedAt,
	})
	service := plugins.NewService(nil, registry, nil, testLogger())
	bridge := &managedConversationGatewayStub{descriptor: pluginsdk.AgentConversationDescriptor{
		TaskID: "managed-task", SessionID: "managed-session", WorkspaceID: "managed-workspace",
	}}
	service.SetAgentConversations(bridge)
	hub.SetPluginConversationService(service)
	client := NewClient("managed-client", authn.Identity{UserID: "user-1"}, nil, hub, testLogger())
	hub.clients[client] = true
	client.conversationSubscriptions["managed-scope"] = conversationSubscription{
		ScopeID: "managed-scope", SessionID: "managed-session", ConsumerKind: conversationConsumerPlugin,
		PluginID: "managed-plugin", Generation: installedAt.UnixMicro(), UserID: "user-1", Epoch: "epoch",
		Managed: &plugins.ManagedConversationIdentity{
			PluginID: "managed-plugin", UserID: "user-1", Generation: installedAt.UnixMicro(),
			WorkspaceID: "managed-workspace", TaskID: "managed-task", SessionID: "managed-session",
		},
	}

	if !hub.conversationAuthorized(client, client.conversationSubscriptions["managed-scope"]) {
		t.Fatal("exact managed descriptor must authorize subscription")
	}
	bridge.descriptor.TaskID = "cross-task"
	if hub.conversationAuthorized(client, client.conversationSubscriptions["managed-scope"]) {
		t.Fatal("cross-task descriptor replacement must revoke subscription")
	}
	hub.BroadcastConversationMutation(&models.ConversationMutationReceipt{SessionID: "managed-session", Revision: 1, Complete: true})
	if len(client.conversationSubscriptions) != 0 {
		t.Fatal("revoked managed subscription remains active")
	}
}

func TestManagedConversationV2SubscriptionRequestIsolation(t *testing.T) {
	identity := authn.Identity{UserID: "user-1", Role: authn.RoleMember}
	installedAt := time.Date(2026, 9, 19, 15, 0, 0, 0, time.UTC)
	reads := &atomic.Int64{}
	source := conversationSourceFixture{
		reads: reads, session: &models.TaskSession{ID: "managed-session", TaskID: "managed-task"},
		revision: models.ConversationRevision{Exists: true, Revision: 17},
	}
	registry := plugins.NewRegistry()
	registry.Add(&store.Record{
		Manifest: manifest.Manifest{ID: "managed-plugin", Capabilities: manifest.Capabilities{AgentConversation: true}},
		Status:   plugins.StatusActive, InstalledAt: installedAt,
	})
	service := plugins.NewService(nil, registry, nil, testLogger())
	bridge := &managedConversationGatewayStub{descriptor: pluginsdk.AgentConversationDescriptor{
		TaskID: "managed-task", SessionID: "managed-session", WorkspaceID: "managed-workspace",
	}}
	service.SetAgentConversations(bridge)
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		ctx.Request = ctx.Request.WithContext(authn.WithIdentity(ctx.Request.Context(), identity))
		ctx.Next()
	})
	plugins.RegisterRoutes(router, service, nil, testLogger(), source)

	get := func(path string, headers map[string]string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	var binding struct {
		BindingToken string `json:"bindingToken"`
		Generation   int64  `json:"generation"`
	}
	bindingResponse := get("/api/plugins/managed-plugin/conversation/binding", nil)
	if bindingResponse.Code != http.StatusOK || json.Unmarshal(bindingResponse.Body.Bytes(), &binding) != nil {
		t.Fatalf("binding response = %d %s", bindingResponse.Code, bindingResponse.Body.String())
	}
	var grant struct {
		TaskID                   string `json:"taskId"`
		SessionID                string `json:"sessionId"`
		WorkspaceID              string `json:"workspaceId"`
		ManagedConversationToken string `json:"managedConversationToken"`
	}
	grantResponse := get(
		"/api/plugins/managed-plugin/conversation/managed/managed-session?workspace_id=managed-workspace",
		map[string]string{"X-Kandev-Plugin-Binding": binding.BindingToken},
	)
	if grantResponse.Code != http.StatusOK || json.Unmarshal(grantResponse.Body.Bytes(), &grant) != nil {
		t.Fatalf("grant response = %d %s", grantResponse.Code, grantResponse.Body.String())
	}

	hub := NewHub(ws.NewDispatcher(), testLogger())
	hub.SetPluginConversationService(service)
	hub.SetConversationSourceReader(source)
	client := NewClient("managed-client", identity, nil, hub, testLogger())
	hub.clients[client] = true
	subscribe := func(requestID, scopeID, sessionID, taskID string, generation int64) conversationSubscribeResponse {
		request, err := ws.NewRequest(requestID, ws.ActionSessionConversationSubscribe, map[string]any{
			"scope_id": scopeID, "session_id": sessionID, "consumer_kind": conversationConsumerPlugin,
			"plugin_id": "managed-plugin", "generation": generation, "binding_token": binding.BindingToken,
			"managed_conversation_token": grant.ManagedConversationToken, "task_id": taskID,
		})
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		client.handleConversationSubscribe(request)
		var response ws.Message
		if err := json.Unmarshal(<-client.controlSend, &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		var payload conversationSubscribeResponse
		if err := json.Unmarshal(response.Payload, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		return payload
	}

	for _, denied := range []struct {
		name, sessionID, taskID string
		generation              int64
		code                    string
		retryable               bool
	}{
		{name: "cross-task", sessionID: grant.SessionID, taskID: "other-task", generation: binding.Generation, code: "invalid_binding"},
		{name: "cross-session", sessionID: "other-session", taskID: grant.TaskID, generation: binding.Generation, code: "invalid_binding"},
		{name: "stale-generation", sessionID: grant.SessionID, taskID: grant.TaskID, generation: binding.Generation - 1, code: "generation_superseded", retryable: true},
	} {
		t.Run(denied.name, func(t *testing.T) {
			scopeID := "managed:" + denied.name
			payload := subscribe(denied.name, scopeID, denied.sessionID, denied.taskID, denied.generation)
			if payload.Success || payload.Error.Code != denied.code || payload.Error.Retryable != denied.retryable {
				t.Fatalf("denied payload = %+v", payload)
			}
			if _, exists := client.conversationSubscriptions[scopeID]; exists || reads.Load() != 0 {
				t.Fatalf("denied %s subscribed or read source: subscriptions=%+v reads=%d", denied.name, client.conversationSubscriptions, reads.Load())
			}
		})
	}

	payload := subscribe("valid", "managed:valid", grant.SessionID, grant.TaskID, binding.Generation)
	if !payload.Success || payload.ProtocolVersion != 2 || payload.Revision != "17" {
		t.Fatalf("valid payload = %+v", payload)
	}
	if reads.Load() != 1 {
		t.Fatalf("source reads = %d, want 1", reads.Load())
	}
	subscription, exists := client.conversationSubscriptions["managed:valid"]
	if !exists || subscription.Managed == nil || *subscription.Managed != (plugins.ManagedConversationIdentity{
		PluginID: "managed-plugin", UserID: "user-1", Generation: binding.Generation,
		WorkspaceID: grant.WorkspaceID, TaskID: grant.TaskID, SessionID: grant.SessionID,
	}) {
		t.Fatalf("managed subscription = %+v", subscription)
	}

	bridge.descriptor.TaskID = "replacement-task"
	hub.BroadcastConversationMutation(&models.ConversationMutationReceipt{
		SessionID: grant.SessionID, BaseRevision: 17, Revision: 18, Complete: true,
	})
	if _, exists := client.conversationSubscriptions["managed:valid"]; exists {
		t.Fatal("revoked managed subscription remains active")
	}
	var terminal ws.Message
	if err := json.Unmarshal(<-client.send, &terminal); err != nil {
		t.Fatalf("decode terminal frame: %v", err)
	}
	var terminalPayload conversationChangedPayload
	if err := json.Unmarshal(terminal.Payload, &terminalPayload); err != nil {
		t.Fatalf("decode terminal payload: %v", err)
	}
	if terminal.Action != ws.ActionSessionConversationChanged || !terminalPayload.Terminal ||
		terminalPayload.ScopeID != "managed:valid" || terminalPayload.SessionID != grant.SessionID ||
		len(terminalPayload.Operations) != 0 {
		t.Fatalf("terminal payload = %+v", terminalPayload)
	}
}

type conversationSubscribeResponse struct {
	Success         bool   `json:"success"`
	ProtocolVersion int    `json:"protocol_version"`
	Revision        string `json:"revision"`
	Error           struct {
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
	} `json:"error"`
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
