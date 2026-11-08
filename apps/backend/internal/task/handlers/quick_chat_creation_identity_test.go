package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
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

type gatedQuickChatCreationOrchestrator struct {
	delegate *captureOrchestrator
	prepared chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (o *gatedQuickChatCreationOrchestrator) LaunchSession(
	ctx context.Context,
	req *orchestrator.LaunchSessionRequest,
) (*orchestrator.LaunchSessionResponse, error) {
	if req.Intent == orchestrator.IntentPrepare {
		o.once.Do(func() { close(o.prepared) })
		<-o.release
	}
	return o.delegate.LaunchSession(ctx, req)
}

func (o *gatedQuickChatCreationOrchestrator) EnsureSession(
	ctx context.Context,
	taskID string,
	options ...orchestrator.EnsureSessionOptions,
) (*orchestrator.EnsureSessionResponse, error) {
	return o.delegate.EnsureSession(ctx, taskID, options...)
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

	t.Run("status lookup is read only for an unsettled task without a session", func(t *testing.T) {
		h, orch, repo := quickChatTestIdentityRepo(t)
		externalID, err := service.NewQuickChatCreationExternalID("user-a", quickChatCreationTestID)
		require.NoError(t, err)
		repo.task = &models.Task{ID: "task-incomplete", WorkspaceID: "ws-1", IsEphemeral: true, ExternalID: externalID}

		status, statusRec := quickChatCreationContext(t, http.MethodGet, "/workspaces/ws-1/quick-chat/creations/"+quickChatCreationTestID, "", "user-a")
		h.httpGetQuickChatCreation(status)
		require.Equal(t, http.StatusOK, statusRec.Code, statusRec.Body.String())
		var inspected httpStartQuickChatResponse
		require.NoError(t, jsonUnmarshal(statusRec.Body.Bytes(), &inspected))
		require.Equal(t, "task-incomplete", inspected.TaskID)
		require.Empty(t, inspected.SessionID)
		require.NotNil(t, inspected.CreationComplete)
		require.False(t, *inspected.CreationComplete)
		require.Zero(t, repo.createTaskCalls)
		require.Empty(t, orch.requests)
	})

	t.Run("same token resumes preparation after an incomplete response", func(t *testing.T) {
		h, orch, repo := quickChatTestIdentityRepo(t)
		orch.prepErr = errors.New("temporary preparation failure")
		orch.startCreatedCalled = make(chan struct{}, 1)
		body := `{"creation_id":"` + quickChatCreationTestID + `","agent_profile_id":"profile-1","prompt":"accepted opening"}`
		first, firstRec := quickChatCreationContext(t, http.MethodPost, "/workspaces/ws-1/quick-chat", body, "user-a")
		h.httpStartQuickChat(first)
		require.Equal(t, http.StatusOK, firstRec.Code, firstRec.Body.String())
		var incomplete httpStartQuickChatResponse
		require.NoError(t, jsonUnmarshal(firstRec.Body.Bytes(), &incomplete))
		require.False(t, *incomplete.CreationComplete)
		require.NotNil(t, repo.task)
		require.Nil(t, repo.task.ExternalIDSettledAt)

		orch.prepErr = nil
		retryBody := `{"creation_id":"` + quickChatCreationTestID + `","agent_profile_id":"changed-profile","prompt":"changed opening"}`
		retry, retryRec := quickChatCreationContext(t, http.MethodPost, "/workspaces/ws-1/quick-chat", retryBody, "user-a")
		h.httpStartQuickChat(retry)
		require.Equal(t, http.StatusOK, retryRec.Code, retryRec.Body.String())
		var recovered httpStartQuickChatResponse
		require.NoError(t, jsonUnmarshal(retryRec.Body.Bytes(), &recovered))
		require.Equal(t, incomplete.TaskID, recovered.TaskID)
		require.Equal(t, "sess-1", recovered.SessionID)
		require.True(t, *recovered.CreationComplete)
		require.NotNil(t, repo.task.ExternalIDSettledAt)
		require.Equal(t, 1, repo.createTaskCalls)
		require.Equal(t, 2, quickChatIntentCount(orch, orchestrator.IntentPrepare))
		orch.mu.Lock()
		prepareRequests := make([]*orchestrator.LaunchSessionRequest, 0, 2)
		for _, request := range orch.requests {
			if request.Intent == orchestrator.IntentPrepare {
				prepareRequests = append(prepareRequests, request)
			}
		}
		orch.mu.Unlock()
		require.Equal(t, "profile-1", prepareRequests[1].AgentProfileID)
		require.Equal(t, "accepted opening", prepareRequests[1].InitialPromptPreview.Content)
		select {
		case <-orch.startCreatedCalled:
		case <-time.After(time.Second):
			t.Fatal("recovered Quick Chat did not dispatch its first start")
		}
		require.Equal(t, 1, quickChatIntentCount(orch, orchestrator.IntentStartCreated))
	})

	t.Run("concurrent same-token retry waits for the current creation", func(t *testing.T) {
		h, orch, repo := quickChatTestIdentityRepo(t)
		orch.startCreatedCalled = make(chan struct{}, 1)
		gate := &gatedQuickChatCreationOrchestrator{
			delegate: orch,
			prepared: make(chan struct{}),
			release:  make(chan struct{}),
		}
		h.orchestrator = gate
		var releaseOnce sync.Once
		release := func() { releaseOnce.Do(func() { close(gate.release) }) }
		t.Cleanup(release)

		type result struct {
			status int
			body   httpStartQuickChatResponse
			err    error
		}
		results := make(chan result, 2)
		post := func() {
			c, rec := quickChatCreationContext(t, http.MethodPost, "/workspaces/ws-1/quick-chat",
				`{"creation_id":"`+quickChatCreationTestID+`","agent_profile_id":"profile-1"}`, "user-a")
			h.httpStartQuickChat(c)
			var response httpStartQuickChatResponse
			err := jsonUnmarshal(rec.Body.Bytes(), &response)
			results <- result{status: rec.Code, body: response, err: err}
		}

		go post()
		select {
		case <-gate.prepared:
		case <-time.After(time.Second):
			t.Fatal("first request did not reach session preparation")
		}
		go post()
		require.Eventually(t, func() bool {
			h.quickChatCreationLocksMu.Lock()
			defer h.quickChatCreationLocksMu.Unlock()
			for _, lock := range h.quickChatCreationLocks {
				if lock.references == 2 {
					return true
				}
			}
			return false
		}, time.Second, time.Millisecond)
		repo.primarySession = &models.TaskSession{ID: "sess-1", TaskID: repo.task.ID, State: models.TaskSessionStateCreated}
		release()

		first, second := <-results, <-results
		require.NoError(t, first.err)
		require.NoError(t, second.err)
		require.Equal(t, http.StatusOK, first.status)
		require.Equal(t, http.StatusOK, second.status)
		require.Equal(t, first.body.TaskID, second.body.TaskID)
		require.Equal(t, first.body.SessionID, second.body.SessionID)
		require.Equal(t, 1, repo.createTaskCalls)
		require.Equal(t, 1, quickChatIntentCount(orch, orchestrator.IntentPrepare))
		select {
		case <-orch.startCreatedCalled:
		case <-time.After(time.Second):
			t.Fatal("accepted Quick Chat did not dispatch its first start")
		}
		require.Equal(t, 1, quickChatIntentCount(orch, orchestrator.IntentStartCreated))
	})

	t.Run("same token reuses its prepared session after settlement fails", func(t *testing.T) {
		h, orch, repo := quickChatTestIdentityRepo(t)
		repo.settleExternalIDErr = errors.New("temporary settlement failure")
		orch.startCreatedCalled = make(chan struct{}, 1)
		body := `{"creation_id":"` + quickChatCreationTestID + `","agent_profile_id":"profile-1"}`
		first, firstRec := quickChatCreationContext(t, http.MethodPost, "/workspaces/ws-1/quick-chat", body, "user-a")
		h.httpStartQuickChat(first)
		require.Equal(t, http.StatusInternalServerError, firstRec.Code, firstRec.Body.String())
		require.Equal(t, 1, quickChatIntentCount(orch, orchestrator.IntentPrepare))
		require.Zero(t, quickChatIntentCount(orch, orchestrator.IntentStartCreated))
		repo.primarySession = &models.TaskSession{
			ID: "sess-1", TaskID: repo.task.ID, State: models.TaskSessionStateCreated,
		}
		repo.settleExternalIDErr = nil

		retry, retryRec := quickChatCreationContext(t, http.MethodPost, "/workspaces/ws-1/quick-chat", body, "user-a")
		h.httpStartQuickChat(retry)
		require.Equal(t, http.StatusOK, retryRec.Code, retryRec.Body.String())
		require.Equal(t, 1, repo.createTaskCalls)
		require.Equal(t, 1, quickChatIntentCount(orch, orchestrator.IntentPrepare), "a retained session must not be allocated twice")
		select {
		case <-orch.startCreatedCalled:
		case <-time.After(time.Second):
			t.Fatal("settled Quick Chat did not dispatch its first start")
		}
		require.Equal(t, 1, quickChatIntentCount(orch, orchestrator.IntentStartCreated))
	})

	t.Run("pre-allocation repository rejection can be corrected with the same token", func(t *testing.T) {
		h, orch, repo := quickChatTestIdentityRepo(t)
		repo.repositoryErr = taskrepository.ErrRepositoryNotFound
		bad, badRec := quickChatCreationContext(t, http.MethodPost, "/workspaces/ws-1/quick-chat",
			`{"creation_id":"`+quickChatCreationTestID+`","agent_profile_id":"profile-1","repositories":[{"repository_id":"deleted-repo","base_branch":"main"}]}`, "user-a")
		h.httpStartQuickChat(bad)
		require.Equal(t, http.StatusNotFound, badRec.Code, badRec.Body.String())
		var rejected map[string]any
		require.NoError(t, jsonUnmarshal(badRec.Body.Bytes(), &rejected))
		require.Equal(t, "quick_chat_creation_rejected", rejected[errorDetailKeyErrorCode])
		require.Zero(t, repo.createTaskCalls)
		require.Nil(t, repo.task)

		repo.repositoryErr = nil
		corrected, correctedRec := quickChatCreationContext(t, http.MethodPost, "/workspaces/ws-1/quick-chat",
			`{"creation_id":"`+quickChatCreationTestID+`","agent_profile_id":"profile-1","repositories":[{"repository_id":"repo-1","base_branch":"main"}]}`, "user-a")
		h.httpStartQuickChat(corrected)
		require.Equal(t, http.StatusOK, correctedRec.Code, correctedRec.Body.String())
		require.Equal(t, 1, repo.createTaskCalls)
		require.Equal(t, 1, quickChatIntentCount(orch, orchestrator.IntentPrepare))
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
