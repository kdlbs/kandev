package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func TestManagedConversationContinuationRenewalIsolation(t *testing.T) {
	created := time.Date(2026, 9, 19, 14, 0, 0, 0, time.UTC)
	service, _, _ := newTestService(t)
	service.registry.Add(managedConversationPluginRecord("managed-plugin", created))
	bridge := &managedConversationBridgeStub{descriptor: pluginsdk.AgentConversationDescriptor{TaskID: "managed-task", SessionID: "managed-session", WorkspaceID: "managed-workspace"}}
	service.SetAgentConversations(bridge)
	router := registerPluginRoutesWithIdentity(t, service, authn.Identity{UserID: "user_1", Role: authn.RoleMember}, &fakeConversationReader{})
	generation := conversationGeneration(created)
	binding, _, err := service.conversationTokens.mintBinding("managed-plugin", "user_1", generation)
	require.NoError(t, err)
	managed, err := service.conversationTokens.mintManagedConversation("managed-plugin", "user_1", generation, "managed-workspace", "managed-task", "managed-session")
	require.NoError(t, err)
	cursor, err := service.conversationTokens.mintCursor("managed-plugin", "user_1", generation, "managed-session", stringPtr("managed-task"), "desc", []string{"user"}, "message-1", 47, "fingerprint-1")
	require.NoError(t, err)
	snapshot, err := service.conversationTokens.mintSnapshot("managed-plugin", "user_1", generation, "managed-session", stringPtr("managed-task"), "desc", []string{"user"}, 47, "fingerprint-1")
	require.NoError(t, err)

	renew := func(cursorToken, snapshotToken string) *httptest.ResponseRecorder {
		return doAuthedRequest(router, http.MethodPost, "/api/plugins/managed-plugin/conversation/continuation/renew", `{"cursor":"`+cursorToken+`","snapshot_token":"`+snapshotToken+`"}`, map[string]string{"Content-Type": "application/json", "X-Kandev-Plugin-Binding": binding, "X-Kandev-Managed-Conversation": managed})
	}
	response := renew(cursor, snapshot)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var continuation conversationContinuationRenewResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &continuation))
	renewedCursor, err := service.conversationTokens.parse(continuation.Cursor)
	require.NoError(t, err)
	renewedSnapshot, err := service.conversationTokens.parse(continuation.SnapshotToken)
	require.NoError(t, err)
	require.Equal(t, renewalClaims(t, service.conversationTokens, cursor), normalizeRenewalClaims(renewedCursor))
	require.Equal(t, renewalClaims(t, service.conversationTokens, snapshot), normalizeRenewalClaims(renewedSnapshot))

	crossTaskCursor, err := service.conversationTokens.mintCursor("managed-plugin", "user_1", generation, "managed-session", stringPtr("other-task"), "desc", []string{"user"}, "message-1", 47, "fingerprint-1")
	require.NoError(t, err)
	crossTaskSnapshot, err := service.conversationTokens.mintSnapshot("managed-plugin", "user_1", generation, "managed-session", stringPtr("other-task"), "desc", []string{"user"}, 47, "fingerprint-1")
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, renew(crossTaskCursor, crossTaskSnapshot).Code)

	sessionCursor, err := service.conversationTokens.mintCursor("managed-plugin", "user_1", generation, "other-session", stringPtr("managed-task"), "desc", []string{"user"}, "message-1", 47, "fingerprint-1")
	require.NoError(t, err)
	sessionSnapshot, err := service.conversationTokens.mintSnapshot("managed-plugin", "user_1", generation, "other-session", stringPtr("managed-task"), "desc", []string{"user"}, 47, "fingerprint-1")
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, renew(sessionCursor, sessionSnapshot).Code)

	bridge.descriptor.TaskID = "replacement-task"
	require.Equal(t, http.StatusNotFound, renew(cursor, snapshot).Code)
	service.registry.Add(managedConversationPluginRecord("managed-plugin", created.Add(time.Minute)))
	require.Equal(t, http.StatusNotFound, renew(cursor, snapshot).Code)
}

func TestManagedConversationV2GrantIsolation(t *testing.T) {
	created := time.Date(2026, 9, 19, 14, 0, 0, 0, time.UTC)
	reader := &fakeConversationReader{session: &taskmodels.TaskSession{ID: "managed-session", TaskID: "managed-task"}, sourceMessages: taskmodels.ConversationMessagePage{Revision: 1}, sourceTurns: taskmodels.ConversationTurnPage{Revision: 1}, sourceRevision: taskmodels.ConversationRevision{Exists: true, Revision: 1}}
	service, _, _ := newTestService(t)
	service.registry.Add(managedConversationPluginRecord("managed-plugin", created))
	bridge := &managedConversationBridgeStub{descriptor: pluginsdk.AgentConversationDescriptor{TaskID: "managed-task", SessionID: "managed-session", WorkspaceID: "managed-workspace"}}
	service.SetAgentConversations(bridge)
	router := registerPluginRoutesWithIdentity(t, service, authn.Identity{UserID: "user_1", Role: authn.RoleMember}, reader)
	binding, _, err := service.conversationTokens.mintBinding("managed-plugin", "user_1", conversationGeneration(created))
	require.NoError(t, err)
	managed, err := service.conversationTokens.mintManagedConversation("managed-plugin", "user_1", conversationGeneration(created), "managed-workspace", "managed-task", "managed-session")
	require.NoError(t, err)
	headers := map[string]string{"X-Kandev-Plugin-Binding": binding, "X-Kandev-Managed-Conversation": managed}
	requestWithHeaders := func(path string, requestHeaders map[string]string) *httptest.ResponseRecorder {
		return doAuthedRequest(router, http.MethodGet, path, "", requestHeaders)
	}
	request := func(path string) *httptest.ResponseRecorder { return requestWithHeaders(path, headers) }
	paths := []string{
		"/api/plugins/managed-plugin/conversation/v2/task-sessions/managed-session/messages",
		"/api/plugins/managed-plugin/conversation/v2/task-sessions/managed-session/turns",
		"/api/plugins/managed-plugin/conversation/v2/task-sessions/managed-session/revision",
	}
	for _, path := range paths {
		response := request(path)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	}
	taskID := "managed-task"
	cursor, err := service.conversationTokens.mintSourceCursor(
		"managed-plugin", "user_1", conversationGeneration(created), "managed-session",
		&taskID, "desc", nil, "message-cursor", service.conversationEpoch, 20,
	)
	require.NoError(t, err)
	response := request(paths[0] + "?cursor=" + url.QueryEscape(cursor))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "message-cursor", reader.sourceMessageRequest.CursorID)
	require.Equal(t, "managed-task", optionalStringValue(reader.sourceMessageRequest.TaskID))
	require.Equal(t, "managed-task", optionalStringValue(reader.sourceTurnRequest.TaskID))
	require.Equal(t, 2, reader.sourceMessageReads)
	require.Equal(t, 1, reader.sourceTurnReads)
	require.Equal(t, 1, reader.sourceRevisionReads)
	for _, path := range paths[:2] {
		foreign := request(path + "?task_id=other-task")
		require.Equal(t, http.StatusBadRequest, foreign.Code, foreign.Body.String())
	}
	require.Equal(t, 2, reader.sourceMessageReads)
	require.Equal(t, 1, reader.sourceTurnReads)
	for _, path := range paths {
		response := request(strings.Replace(path, "managed-session", "other-session", 1))
		require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	}
	require.Equal(t, 2, reader.sourceMessageReads)
	require.Equal(t, 1, reader.sourceTurnReads)
	require.Equal(t, 1, reader.sourceRevisionReads)
	bridge.descriptor.TaskID = "replacement-task"
	for _, path := range paths {
		response := request(path)
		require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	}
	require.Equal(t, 2, reader.sourceMessageReads)
	require.Equal(t, 1, reader.sourceTurnReads)
	require.Equal(t, 1, reader.sourceRevisionReads)
	bridge.descriptor.TaskID = "managed-task"
	for _, invalidBinding := range []string{"", "corrupt-binding"} {
		invalidHeaders := map[string]string{"X-Kandev-Plugin-Binding": invalidBinding, "X-Kandev-Managed-Conversation": managed}
		for _, path := range paths {
			response := requestWithHeaders(path, invalidHeaders)
			require.Equal(t, http.StatusUnauthorized, response.Code, response.Body.String())
		}
	}
	require.Equal(t, 2, reader.sourceMessageReads)
	require.Equal(t, 1, reader.sourceTurnReads)
	require.Equal(t, 1, reader.sourceRevisionReads)
	service.registry.Add(managedConversationPluginRecord("managed-plugin", created.Add(time.Minute)))
	for _, path := range paths {
		response := request(path)
		require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	}
	require.Equal(t, 2, reader.sourceMessageReads)
	require.Equal(t, 1, reader.sourceTurnReads)
	require.Equal(t, 1, reader.sourceRevisionReads)
}

func renewalClaims(t *testing.T, tokens *conversationTokenManager, token string) conversationTokenClaims {
	t.Helper()
	claims, err := tokens.parse(token)
	require.NoError(t, err)
	return normalizeRenewalClaims(claims)
}

func normalizeRenewalClaims(claims conversationTokenClaims) conversationTokenClaims {
	claims.ExpiresAt = 0
	claims.Nonce = ""
	return claims
}

type managedConversationBridgeStub struct {
	descriptor pluginsdk.AgentConversationDescriptor
}

func (s *managedConversationBridgeStub) Ensure(context.Context, string, pluginsdk.AgentConversationSpec) (pluginsdk.AgentConversationDescriptor, string, error) {
	return pluginsdk.AgentConversationDescriptor{}, "", nil
}
func (s *managedConversationBridgeStub) Dispatch(context.Context, string, string, string, string, string) (pluginsdk.AgentConversationDispatch, error) {
	return pluginsdk.AgentConversationDispatch{}, nil
}
func (s *managedConversationBridgeStub) Delete(context.Context, string, string, string) (int32, error) {
	return 0, nil
}
func (s *managedConversationBridgeStub) DeleteAllForPlugin(context.Context, string) (int32, error) {
	return 0, nil
}
func (s *managedConversationBridgeStub) ResolveManagedConversation(context.Context, string, string, string) (pluginsdk.AgentConversationDescriptor, error) {
	return s.descriptor, nil
}

func managedConversationPluginRecord(id string, installedAt time.Time) *store.Record {
	return &store.Record{Manifest: manifest.Manifest{ID: id, Capabilities: manifest.Capabilities{AgentConversation: true}}, Status: StatusActive, InstalledAt: installedAt}
}
