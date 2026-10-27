package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

const quickChatCreationTestID = "a7c0fd51-6526-4732-95bc-c6e25f745e7f"

func quickChatCreationContext(t *testing.T, method, path, body, principalID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var c *gin.Context
	var rec *httptest.ResponseRecorder
	if method == http.MethodPost {
		c, rec = quickChatRequestContext(t, path, body)
	} else {
		gin.SetMode(gin.TestMode)
		rec = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
		c.Params = gin.Params{{Key: "id", Value: "ws-1"}, {Key: "creation_id", Value: quickChatCreationTestID}}
	}
	identity := authn.Identity{UserID: principalID, Role: authn.RoleMember, Synthetic: true}
	c.Request = c.Request.WithContext(authn.WithIdentity(c.Request.Context(), identity))
	return c, rec
}

func quickChatTestIdentityRepo(t *testing.T) (*TaskHandlers, *captureOrchestrator, *quickChatHandlerRepo) {
	t.Helper()
	h, orch, repo := newQuickChatHandlerForTest(t)
	return h, orch, repo
}

func quickChatIntentCount(orch *captureOrchestrator, intent orchestrator.SessionIntent) int {
	orch.mu.Lock()
	defer orch.mu.Unlock()
	count := 0
	for _, request := range orch.requests {
		if request.Intent == intent {
			count++
		}
	}
	return count
}

func TestQuickChatCreationIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("same token returns original settled chat before retry validation", func(t *testing.T) {
		h, orch, repo := quickChatTestIdentityRepo(t)
		orch.startCreatedCalled = make(chan struct{}, 1)
		c, rec := quickChatCreationContext(t, http.MethodPost, "/workspaces/ws-1/quick-chat",
			`{"creation_id":"`+quickChatCreationTestID+`","agent_profile_id":"profile-1"}`, "user-a")
		h.httpStartQuickChat(c)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var first httpStartQuickChatResponse
		require.NoError(t, jsonUnmarshal(rec.Body.Bytes(), &first))
		require.Equal(t, quickChatCreationTestID, first.CreationID)
		require.NotNil(t, first.CreationComplete)
		require.True(t, *first.CreationComplete)
		select {
		case <-orch.startCreatedCalled:
		case <-time.After(time.Second):
			t.Fatal("accepted Quick Chat did not dispatch its first start")
		}
		repo.primarySession = &models.TaskSession{ID: first.SessionID, TaskID: first.TaskID, State: models.TaskSessionStateCreated}

		// An invalid mutable field is ignored for a key that already resolves.
		retry, retryRec := quickChatCreationContext(t, http.MethodPost, "/workspaces/ws-1/quick-chat",
			`{"creation_id":"`+quickChatCreationTestID+`","repositories":[{"repository_id":"","base_branch":"main"}]}`, "user-a")
		h.httpStartQuickChat(retry)
		require.Equal(t, http.StatusOK, retryRec.Code, retryRec.Body.String())
		var repeated httpStartQuickChatResponse
		require.NoError(t, jsonUnmarshal(retryRec.Body.Bytes(), &repeated))
		require.Equal(t, first.TaskID, repeated.TaskID)
		require.Equal(t, first.SessionID, repeated.SessionID)
		require.Equal(t, quickChatCreationTestID, repeated.CreationID)
		require.NotNil(t, repeated.CreationComplete)
		require.True(t, *repeated.CreationComplete)
		require.Equal(t, 1, repo.createTaskCalls)
		require.Equal(t, 1, quickChatIntentCount(orch, orchestrator.IntentPrepare))
		require.Equal(t, 1, quickChatIntentCount(orch, orchestrator.IntentStartCreated))
	})

	t.Run("unsettled task without a session stays incomplete and is read only", func(t *testing.T) {
		h, orch, repo := quickChatTestIdentityRepo(t)
		externalID, err := service.NewQuickChatCreationExternalID("user-a", quickChatCreationTestID)
		require.NoError(t, err)
		repo.task = &models.Task{ID: "task-incomplete", WorkspaceID: "ws-1", IsEphemeral: true, ExternalID: externalID}

		post, postRec := quickChatCreationContext(t, http.MethodPost, "/workspaces/ws-1/quick-chat",
			`{"creation_id":"`+quickChatCreationTestID+`","agent_profile_id":"missing-profile"}`, "user-a")
		h.httpStartQuickChat(post)
		require.Equal(t, http.StatusOK, postRec.Code, postRec.Body.String())
		var accepted httpStartQuickChatResponse
		require.NoError(t, jsonUnmarshal(postRec.Body.Bytes(), &accepted))
		require.Equal(t, "task-incomplete", accepted.TaskID)
		require.Empty(t, accepted.SessionID)
		require.NotNil(t, accepted.CreationComplete)
		require.False(t, *accepted.CreationComplete)
		require.Zero(t, repo.createTaskCalls)
		require.Empty(t, orch.requests)

		status, statusRec := quickChatCreationContext(t, http.MethodGet, "/workspaces/ws-1/quick-chat/creations/"+quickChatCreationTestID, "", "user-a")
		h.httpGetQuickChatCreation(status)
		require.Equal(t, http.StatusOK, statusRec.Code, statusRec.Body.String())
		var inspected httpStartQuickChatResponse
		require.NoError(t, jsonUnmarshal(statusRec.Body.Bytes(), &inspected))
		require.Equal(t, "task-incomplete", inspected.TaskID)
		require.Empty(t, inspected.SessionID)
		require.NotNil(t, inspected.CreationComplete)
		require.False(t, *inspected.CreationComplete)
	})

	t.Run("foreign principal cannot resolve token", func(t *testing.T) {
		h, _, repo := quickChatTestIdentityRepo(t)
		externalID, err := service.NewQuickChatCreationExternalID("user-a", quickChatCreationTestID)
		require.NoError(t, err)
		repo.task = &models.Task{ID: "task-owner", WorkspaceID: "ws-1", IsEphemeral: true, ExternalID: externalID}
		status, rec := quickChatCreationContext(t, http.MethodGet, "/workspaces/ws-1/quick-chat/creations/"+quickChatCreationTestID, "", "user-b")
		h.httpGetQuickChatCreation(status)
		require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	})

	t.Run("conflicting holder is never adopted", func(t *testing.T) {
		h, orch, repo := quickChatTestIdentityRepo(t)
		externalID, err := service.NewQuickChatCreationExternalID("user-a", quickChatCreationTestID)
		require.NoError(t, err)
		repo.task = &models.Task{ID: "task-regular", WorkspaceID: "ws-1", IsEphemeral: false, ExternalID: externalID}
		post, rec := quickChatCreationContext(t, http.MethodPost, "/workspaces/ws-1/quick-chat",
			`{"creation_id":"`+quickChatCreationTestID+`"}`, "user-a")
		h.httpStartQuickChat(post)
		require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		require.Empty(t, orch.requests)
		require.Zero(t, repo.createTaskCalls)
	})

	t.Run("settlement error and concurrent deletion do not dispatch", func(t *testing.T) {
		for _, tc := range []struct {
			name       string
			settleErr  error
			deleteTask bool
			wantStatus int
		}{
			{name: "settlement failure", settleErr: errors.New("settlement unavailable"), wantStatus: http.StatusInternalServerError},
			{name: "task deleted before settlement", deleteTask: true, wantStatus: http.StatusNotFound},
		} {
			t.Run(tc.name, func(t *testing.T) {
				h, orch, repo := quickChatTestIdentityRepo(t)
				repo.settleExternalIDErr = tc.settleErr
				repo.deleteTaskOnSettle = tc.deleteTask
				post, rec := quickChatCreationContext(t, http.MethodPost, "/workspaces/ws-1/quick-chat",
					`{"creation_id":"`+quickChatCreationTestID+`","agent_profile_id":"profile-1"}`, "user-a")
				h.httpStartQuickChat(post)
				require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
				require.Equal(t, 1, quickChatIntentCount(orch, orchestrator.IntentPrepare))
				require.Zero(t, quickChatIntentCount(orch, orchestrator.IntentStartCreated))
			})
		}
	})

	t.Run("invalid UUID is rejected", func(t *testing.T) {
		h, orch, _ := quickChatTestIdentityRepo(t)
		post, rec := quickChatCreationContext(t, http.MethodPost, "/workspaces/ws-1/quick-chat",
			`{"creation_id":"not-a-uuid"}`, "user-a")
		h.httpStartQuickChat(post)
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		require.Empty(t, orch.requests)
	})
}

func jsonUnmarshal(data []byte, value any) error {
	return json.Unmarshal(data, value)
}

func TestQuickChatCreationRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := &TaskHandlers{logger: newTestLogger(t)}
	h.registerHTTP(router)
	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	require.True(t, registered["GET /api/v1/workspaces/:id/quick-chat/creations/:creation_id"])
}
