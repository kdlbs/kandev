package plugins

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.7
func TestConversationBindingRequiresAuthentication(t *testing.T) {
	router, _ := newTestRouter(t)

	response := doRequest(
		router,
		http.MethodGet,
		"/api/plugins/kandev-plugin-history/conversation/binding",
		"",
		nil,
	)

	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.JSONEq(t, `{"error":{"code":"unauthenticated","message":"authentication required","retryable":false}}`, response.Body.String())
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
}

// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.7
func TestConversationBindingHidesUnavailablePlugins(t *testing.T) {
	router, service := newTestRouter(t)
	service.registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID:           "kandev-plugin-history",
			Capabilities: manifest.Capabilities{},
		},
		Status:      StatusActive,
		InstalledAt: time.Now().UTC(),
	})

	response := doAuthedRequest(
		router,
		http.MethodGet,
		"/api/plugins/kandev-plugin-history/conversation/binding",
		"",
		nil,
	)

	require.Equal(t, http.StatusNotFound, response.Code)
	require.JSONEq(t, `{"error":{"code":"not_found","message":"plugin not found","retryable":false}}`, response.Body.String())
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
}

// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.13
func TestConversationBindingReturnsShortLivedGenerationGrant(t *testing.T) {
	router, service := newTestRouter(t)
	installedAt := time.Date(2026, 9, 7, 12, 0, 0, 123, time.UTC)
	service.registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID: "kandev-plugin-history",
			Capabilities: manifest.Capabilities{
				APIRead: []string{"messages"},
			},
		},
		Status:      StatusActive,
		InstalledAt: installedAt,
	})

	response := doAuthedRequest(
		router,
		http.MethodGet,
		"/api/plugins/kandev-plugin-history/conversation/binding",
		"",
		nil,
	)

	require.Equal(t, http.StatusOK, response.Code)
	var body struct {
		BindingToken string    `json:"bindingToken"`
		Generation   int64     `json:"generation"`
		ExpiresAt    time.Time `json:"expiresAt"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.NotEmpty(t, body.BindingToken)
	require.Equal(t, conversationGeneration(installedAt), body.Generation)
	require.LessOrEqual(t, body.Generation, int64(1<<53-1))
	require.WithinDuration(t, time.Now().UTC().Add(10*time.Minute), body.ExpiresAt, time.Second)
	require.LessOrEqual(t, body.ExpiresAt.Sub(time.Now().UTC()), 10*time.Minute)
}

// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.7
func TestConversationReadsRequireAuthenticationBeforeLookup(t *testing.T) {
	router, _ := newTestRouter(t)

	for _, path := range []string{
		"/api/plugins/kandev-plugin-history/conversation/task-sessions/session-1/messages",
		"/api/plugins/kandev-plugin-history/conversation/task-sessions/session-1/turns",
	} {
		response := doRequest(router, http.MethodGet, path, "", nil)
		require.Equal(t, http.StatusUnauthorized, response.Code, path)
		require.JSONEq(t, `{"error":{"code":"unauthenticated","message":"authentication required","retryable":false}}`, response.Body.String())
	}
}

type fakeConversationReader struct {
	session  *taskmodels.TaskSession
	messages []*taskmodels.Message
	turns    []*taskmodels.Turn
	hasMore  bool
	request  taskservice.ListMessagesRequest
}

func (f *fakeConversationReader) GetTaskSession(
	_ context.Context,
	_ string,
) (*taskmodels.TaskSession, error) {
	return f.session, nil
}

func (f *fakeConversationReader) ListMessagesPaginated(
	_ context.Context,
	request taskservice.ListMessagesRequest,
) ([]*taskmodels.Message, bool, error) {
	f.request = request
	return f.messages, f.hasMore, nil
}

func (f *fakeConversationReader) ListTurnsBySession(
	_ context.Context,
	_ string,
) ([]*taskmodels.Turn, error) {
	return f.turns, nil
}

// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.1
// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.2
// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.3
// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.4
// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.12
func TestConversationMessagesReturnSanitizedNarrowDTOs(t *testing.T) {
	created := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	reader := &fakeConversationReader{
		session: &taskmodels.TaskSession{ID: "session-1", TaskID: "task-1"},
		messages: []*taskmodels.Message{{
			ID:            "message-1",
			TaskSessionID: "session-1",
			TaskID:        "task-1",
			TurnID:        "turn-1",
			AuthorType:    taskmodels.MessageAuthorUser,
			Content:       "<kandev-system>hidden</kandev-system>Visible prompt.",
			Metadata: map[string]any{
				"sender_task_id":    "sender-task",
				"sender_task_title": "must not leak",
				"raw_content":       "must not leak",
			},
			CreatedAt:   created,
			PromptIndex: 3,
		}},
	}
	_, service := newTestRouter(t)
	service.registry.Add(conversationPluginRecord("kandev-plugin-history", created))
	router := registerPluginRoutesWithIdentity(
		t,
		service,
		authn.Identity{UserID: "user_1", Role: authn.RoleMember},
		reader,
	)
	headers := conversationReadHeaders(t, service, router, "kandev-plugin-history", "session-1")

	response := doAuthedRequest(
		router,
		http.MethodGet,
		"/api/plugins/kandev-plugin-history/conversation/task-sessions/session-1/messages?task_id=task-1&author_type=user&sort=asc&limit=25",
		"",
		headers,
	)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.JSONEq(t, `{
		"messages":[{
			"id":"message-1",
			"taskId":"task-1",
			"sessionId":"session-1",
			"turnId":"turn-1",
			"authorType":"user",
			"type":"message",
			"content":"Visible prompt.",
			"createdAt":"2026-09-07T12:00:00Z",
			"updatedAt":"2026-09-07T12:00:00Z",
			"promptIndex":3,
			"senderTaskId":"sender-task"
		}],
		"hasMore":false,
		"cursor":null
	}`, response.Body.String())
	require.Equal(t, "session-1", reader.request.TaskSessionID)
	require.Equal(t, "asc", reader.request.Sort)
	require.Equal(t, "user", reader.request.AuthorType)
	require.Equal(t, 25, reader.request.Limit)
}

// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.2
// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.5
func TestConversationMessageCursorIsOpaqueAndQueryBound(t *testing.T) {
	created := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	reader := &fakeConversationReader{
		session: &taskmodels.TaskSession{ID: "session-1", TaskID: "task-1"},
		messages: []*taskmodels.Message{{
			ID:            "message-1",
			TaskSessionID: "session-1",
			TaskID:        "task-1",
			AuthorType:    taskmodels.MessageAuthorUser,
			Content:       "Visible prompt.",
			CreatedAt:     created,
		}},
		hasMore: true,
	}
	_, service := newTestRouter(t)
	service.registry.Add(conversationPluginRecord("kandev-plugin-history", created))
	router := registerPluginRoutesWithIdentity(
		t,
		service,
		authn.Identity{UserID: "user_1", Role: authn.RoleMember},
		reader,
	)
	headers := conversationReadHeaders(t, service, router, "kandev-plugin-history", "session-1")

	first := doAuthedRequest(
		router,
		http.MethodGet,
		"/api/plugins/kandev-plugin-history/conversation/task-sessions/session-1/messages?task_id=task-1&author_type=user&sort=desc&limit=1",
		"",
		headers,
	)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var page conversationMessagesResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &page))
	require.True(t, page.HasMore)
	require.NotNil(t, page.Cursor)
	require.NotContains(t, *page.Cursor, "message-1")

	changedQuery := doAuthedRequest(
		router,
		http.MethodGet,
		"/api/plugins/kandev-plugin-history/conversation/task-sessions/session-1/messages?task_id=task-1&author_type=agent&sort=desc&limit=1&cursor="+*page.Cursor,
		"",
		headers,
	)
	require.Equal(t, http.StatusBadRequest, changedQuery.Code)
	require.JSONEq(t, `{"error":{"code":"invalid_query","message":"invalid cursor","retryable":false}}`, changedQuery.Body.String())

	tampered := doAuthedRequest(
		router,
		http.MethodGet,
		"/api/plugins/kandev-plugin-history/conversation/task-sessions/session-1/messages?task_id=task-1&author_type=user&sort=desc&limit=1&cursor="+*page.Cursor+"x",
		"",
		headers,
	)
	require.Equal(t, http.StatusBadRequest, tampered.Code)
}

// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.4
// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.13
func TestConversationStreamGrantFirstPageRenewsQueryBoundCursor(t *testing.T) {
	installedAt := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	reader := &fakeConversationReader{
		session: &taskmodels.TaskSession{ID: "session-1", TaskID: "task-1"},
		messages: []*taskmodels.Message{{
			ID:            "message-1",
			TaskSessionID: "session-1",
			TaskID:        "task-1",
			AuthorType:    taskmodels.MessageAuthorUser,
			Content:       "Visible prompt.",
			CreatedAt:     installedAt,
		}},
		hasMore: true,
	}
	_, service := newTestRouter(t)
	service.registry.Add(conversationPluginRecord("kandev-plugin-history", installedAt))
	router := registerPluginRoutesWithIdentity(
		t,
		service,
		authn.Identity{UserID: "user_1", Role: authn.RoleMember},
		reader,
	)
	binding := conversationBindingToken(t, router, "kandev-plugin-history")
	snapshot, _, _, err := service.MintSessionStreamGrant(
		"kandev-plugin-history",
		"user_1",
		conversationGeneration(installedAt),
		"session-1",
		"consumer-1",
		"",
		0,
	)
	require.NoError(t, err)
	headers := map[string]string{
		"X-Kandev-Plugin-Binding": binding,
		"X-Kandev-Snapshot-Token": snapshot,
	}

	first := doAuthedRequest(
		router,
		http.MethodGet,
		"/api/plugins/kandev-plugin-history/conversation/task-sessions/session-1/messages?task_id=task-1&author_type=user&sort=desc&limit=1",
		"",
		headers,
	)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var page conversationMessagesResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &page))
	require.NotNil(t, page.Cursor)

	renewed := doAuthedRequest(
		router,
		http.MethodPost,
		"/api/plugins/kandev-plugin-history/conversation/continuation/renew",
		`{"cursor":"`+*page.Cursor+`","snapshot_token":"`+snapshot+`"}`,
		map[string]string{
			"Content-Type":            "application/json",
			"X-Kandev-Plugin-Binding": binding,
		},
	)
	require.Equal(t, http.StatusOK, renewed.Code, renewed.Body.String())
	var continuation conversationContinuationRenewResponse
	require.NoError(t, json.Unmarshal(renewed.Body.Bytes(), &continuation))
	require.NotEqual(t, *page.Cursor, continuation.Cursor)
	require.NotEqual(t, snapshot, continuation.SnapshotToken)
}

// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.1
// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.3
func TestConversationTurnsReturnNarrowNullableDTOs(t *testing.T) {
	started := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	completed := started.Add(time.Minute)
	reader := &fakeConversationReader{
		session: &taskmodels.TaskSession{ID: "session-1", TaskID: "task-1"},
		turns: []*taskmodels.Turn{{
			ID:            "turn-1",
			TaskSessionID: "session-1",
			TaskID:        "task-1",
			StartedAt:     started,
			CompletedAt:   &completed,
			Metadata:      map[string]any{"secret": "must not leak"},
		}},
	}
	_, service := newTestRouter(t)
	service.registry.Add(conversationPluginRecord("kandev-plugin-history", started))
	router := registerPluginRoutesWithIdentity(
		t,
		service,
		authn.Identity{UserID: "user_1", Role: authn.RoleMember},
		reader,
	)
	headers := conversationReadHeaders(t, service, router, "kandev-plugin-history", "session-1")

	response := doAuthedRequest(
		router,
		http.MethodGet,
		"/api/plugins/kandev-plugin-history/conversation/task-sessions/session-1/turns",
		"",
		headers,
	)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.JSONEq(t, `{"turns":[{
		"id":"turn-1",
		"taskId":"task-1",
		"sessionId":"session-1",
		"startedAt":"2026-09-07T12:00:00Z",
		"completedAt":"2026-09-07T12:01:00Z",
		"updatedAt":"2026-09-07T12:00:00Z"
	}]}`, response.Body.String())
}

// @covers AC-PLUGINS-PROMPT-HISTORY-HOST-002.6
func TestConversationContinuationRenewPreservesSnapshot(t *testing.T) {
	installedAt := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	_, service := newTestRouter(t)
	service.registry.Add(conversationPluginRecord("kandev-plugin-history", installedAt))
	tokens := newConversationTokenManager()
	binding, _, err := tokens.mintBinding(
		"kandev-plugin-history",
		"user_1",
		conversationGeneration(installedAt),
	)
	require.NoError(t, err)
	cursor, err := tokens.mintCursor(
		"kandev-plugin-history",
		"user_1",
		conversationGeneration(installedAt),
		"session-1",
		stringPtr("task-1"),
		"desc",
		[]string{"user"},
		"message-1",
		47,
		"fingerprint-1",
	)
	require.NoError(t, err)
	snapshot, err := tokens.mintSnapshot(
		"kandev-plugin-history",
		"user_1",
		conversationGeneration(installedAt),
		"session-1",
		stringPtr("task-1"),
		"desc",
		[]string{"user"},
		47,
		"fingerprint-1",
	)
	require.NoError(t, err)
	router := gin.New()
	ctrl := &Controller{
		svc: service, log: testLogger(t), conversationTokens: tokens,
	}
	registerConversationRoutes(router.Group("/api/plugins"), ctrl)

	response := doAuthedRequest(
		router,
		http.MethodPost,
		"/api/plugins/kandev-plugin-history/conversation/continuation/renew",
		`{"cursor":"`+cursor+`","snapshot_token":"`+snapshot+`"}`,
		map[string]string{
			"Content-Type":            "application/json",
			"X-Kandev-Plugin-Binding": binding,
		},
	)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	var renewed conversationContinuationRenewResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &renewed))
	require.NotEqual(t, cursor, renewed.Cursor)
	require.NotEqual(t, snapshot, renewed.SnapshotToken)
	require.Equal(t, int64(47), renewed.Cutoff)
	require.Equal(t, "fingerprint-1", renewed.Fingerprint)
	require.WithinDuration(t, time.Now().UTC().Add(10*time.Minute), renewed.ExpiresAt, time.Second)
}

func conversationPluginRecord(id string, installedAt time.Time) *store.Record {
	return &store.Record{
		Manifest: manifest.Manifest{
			ID: id,
			Capabilities: manifest.Capabilities{
				APIRead: []string{"messages"},
			},
		},
		Status:      StatusActive,
		InstalledAt: installedAt,
	}
}

func conversationBindingToken(t *testing.T, router *gin.Engine, pluginID string) string {
	t.Helper()
	response := doAuthedRequest(
		router,
		http.MethodGet,
		"/api/plugins/"+pluginID+"/conversation/binding",
		"",
		nil,
	)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var binding conversationBindingResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &binding))
	return binding.BindingToken
}

func conversationReadHeaders(
	t *testing.T,
	service *Service,
	router *gin.Engine,
	pluginID string,
	sessionID string,
) map[string]string {
	t.Helper()
	binding := conversationBindingToken(t, router, pluginID)
	record, err := service.Get(pluginID)
	require.NoError(t, err)
	snapshot, _, _, err := service.MintSessionStreamGrant(
		pluginID,
		"user_1",
		conversationGeneration(record.InstalledAt),
		sessionID,
		"test-consumer",
		"",
		service.SessionEvents().Watermark(sessionID),
	)
	require.NoError(t, err)
	return map[string]string{
		"X-Kandev-Plugin-Binding": binding,
		"X-Kandev-Snapshot-Token": snapshot,
	}
}

func TestFilterTurnsByTaskNarrowsMixedTaskSession(t *testing.T) {
	turns := []*taskmodels.Turn{
		{ID: "turn-a", TaskID: "task-session"},
		{ID: "turn-b", TaskID: "task-other"},
		{ID: "turn-c", TaskID: "task-session"},
	}
	filtered := filterTurnsByTask(turns, "task-session")
	if len(filtered) != 2 || filtered[0].ID != "turn-a" || filtered[1].ID != "turn-c" {
		t.Fatalf("filtered = %+v, want only the session task's turns", filtered)
	}
	if filtered := filterTurnsByTask(turns, ""); len(filtered) != 3 {
		t.Fatalf("empty task filter must be a no-op, got %d", len(filtered))
	}
}
