package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

var openingAttachment = v1.MessageAttachment{
	Type:         "resource",
	Data:         "dGVzdA==",
	MimeType:     "text/plain",
	Name:         "trace.txt",
	DeliveryMode: "path",
}

func quickChatRequestContext(t *testing.T, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}
	return c, rec
}

func TestHTTPQuickChatOpeningPayload(t *testing.T) {
	h, orch, repo := newQuickChatHandlerForTest(t)
	c, rec := quickChatRequestContext(t, "/workspaces/ws-1/quick-chat", `{
		"agent_profile_id":"profile-1",
		"prompt":"Review this trace",
		"attachments":[{"type":"resource","data":"dGVzdA==","mime_type":"text/plain","name":"trace.txt","delivery_mode":"path"}]
	}`)

	h.httpStartQuickChat(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotNil(t, repo.task)
	assert.Equal(t, "Review this trace", repo.task.Description)
	require.Len(t, orch.requests, 1)
	assert.Equal(t, orchestrator.IntentStart, orch.requests[0].Intent)
	assert.Equal(t, "Review this trace", orch.requests[0].Prompt)
	assert.Equal(t, []v1.MessageAttachment{openingAttachment}, orch.requests[0].Attachments)
}

func TestHTTPConfigChatOpeningPayload(t *testing.T) {
	h, orch, _ := newQuickChatHandlerForTest(t)
	orch.startCreatedCalled = make(chan struct{}, 1)
	c, rec := quickChatRequestContext(t, "/workspaces/ws-1/config-chat", `{
		"agent_profile_id":"profile-1",
		"prompt":"Review this trace",
		"attachments":[{"type":"resource","data":"dGVzdA==","mime_type":"text/plain","name":"trace.txt","delivery_mode":"path"}]
	}`)

	h.httpStartConfigChat(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	select {
	case <-orch.startCreatedCalled:
	case <-time.After(time.Second):
		t.Fatal("config chat did not dispatch its prompt-bearing start")
	}
	orch.mu.Lock()
	defer orch.mu.Unlock()
	require.Len(t, orch.requests, 2)
	prepare, start := orch.requests[0], orch.requests[1]
	assert.Equal(t, orchestrator.IntentPrepare, prepare.Intent)
	assert.True(t, prepare.DeferredStart)
	assert.Empty(t, prepare.Prompt)
	assert.Empty(t, prepare.Attachments)
	assert.Equal(t, orchestrator.IntentStartCreated, start.Intent)
	assert.Equal(t, "Review this trace", start.Prompt)
	assert.Equal(t, []v1.MessageAttachment{openingAttachment}, start.Attachments)
}

type configAttachmentClaimResult struct {
	request     *orchestrator.LaunchSessionRequest
	err         error
	ctxErr      error
	hasDeadline bool
}

type configAttachmentClaimOrchestrator struct {
	service                *service.Service
	results                chan configAttachmentClaimResult
	cancelRequestOnPrepare context.CancelFunc
	launchErr              error
}

func (o *configAttachmentClaimOrchestrator) LaunchSession(
	ctx context.Context,
	req *orchestrator.LaunchSessionRequest,
) (*orchestrator.LaunchSessionResponse, error) {
	if req.Intent == orchestrator.IntentPrepare {
		if o.cancelRequestOnPrepare != nil {
			o.cancelRequestOnPrepare()
		}
		return &orchestrator.LaunchSessionResponse{SessionID: "config-session"}, nil
	}
	claimErr := o.service.ClaimMessageAttachments(ctx, req.TaskID, req.SessionID, req.Attachments)
	_, hasDeadline := ctx.Deadline()
	o.results <- configAttachmentClaimResult{
		request: req, err: claimErr, ctxErr: ctx.Err(), hasDeadline: hasDeadline,
	}
	if claimErr != nil {
		return nil, claimErr
	}
	if o.launchErr != nil {
		return nil, o.launchErr
	}
	return &orchestrator.LaunchSessionResponse{SessionID: req.SessionID}, nil
}

func (*configAttachmentClaimOrchestrator) EnsureSession(
	context.Context,
	string,
	...orchestrator.EnsureSessionOptions,
) (*orchestrator.EnsureSessionResponse, error) {
	return nil, nil
}

func TestHTTPConfigChatOpeningAttachmentClaimUsesRequestIdentity(t *testing.T) {
	for _, tc := range []struct {
		name               string
		requestUserID      string
		wantClaimErr       error
		wantStatus         int
		cancelAfterPrepare bool
	}{
		{name: "owner survives request cancellation", requestUserID: "user-1", wantStatus: http.StatusOK, cancelAfterPrepare: true},
		{name: "foreign user is rejected", requestUserID: "user-2", wantClaimErr: models.ErrAttachmentClaimConflict, wantStatus: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "config-chat-attachment.db"))
			require.NoError(t, err)
			dbConnSQLX := sqlx.NewDb(dbConn, "sqlite3")
			t.Cleanup(func() { _ = dbConnSQLX.Close() })
			repo, cleanup, err := repository.Provide(dbConnSQLX, dbConnSQLX, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = cleanup() })
			log := newTestLogger(t)
			agentProfileID := "profile-1"
			executorID := models.ExecutorIDLocal
			require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
				ID: "ws-1", Name: "Workspace", DefaultAgentProfileID: &agentProfileID,
				DefaultExecutorID: &executorID,
			}))
			svc := service.NewService(service.Repos{
				Workspaces: repo, Tasks: repo, TaskRepos: repo,
				Workflows: repo, Messages: repo, Turns: repo,
				Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
				Executors: repo, Environments: repo, TaskEnvironments: repo,
				Reviews: repo,
			}, bus.NewMemoryEventBus(log), log, service.RepositoryDiscoveryConfig{})
			attachmentSvc, err := service.NewAttachmentService(repo, t.TempDir(), nil, log)
			require.NoError(t, err)
			svc.SetAttachmentService(attachmentSvc)
			attachment, err := attachmentSvc.Stage(
				context.Background(), "user-1", "ws-1", "trace.txt", "text/plain", "resource", "path", strings.NewReader("trace bytes"),
			)
			require.NoError(t, err)

			orch := &configAttachmentClaimOrchestrator{
				service: svc,
				results: make(chan configAttachmentClaimResult, 1),
			}
			h := &TaskHandlers{service: svc, orchestrator: orch, logger: log}
			body := `{"agent_profile_id":"profile-1","prompt":"Review this trace","attachments":[{"type":"resource","attachment_id":"` +
				attachment.ID + `","mime_type":"text/plain","name":"trace.txt","size_bytes":11,"delivery_mode":"path"}]}`
			c, rec := quickChatRequestContext(t, "/workspaces/ws-1/config-chat", body)
			requestCtx, cancel := context.WithCancel(authn.WithIdentity(
				c.Request.Context(), authn.Identity{UserID: tc.requestUserID, Role: authn.RoleMember},
			))
			c.Request = c.Request.WithContext(requestCtx)
			if tc.cancelAfterPrepare {
				orch.cancelRequestOnPrepare = cancel
			}

			h.httpStartConfigChat(c)
			cancel()

			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
			var result configAttachmentClaimResult
			select {
			case result = <-orch.results:
			case <-time.After(time.Second):
				t.Fatal("config chat did not attempt to claim its staged attachment")
			}
			if tc.wantClaimErr != nil {
				require.ErrorIs(t, result.err, tc.wantClaimErr)
			} else {
				require.NoError(t, result.err)
				assert.NoError(t, result.ctxErr, "the launch context must be detached from request cancellation")
				assert.True(t, result.hasDeadline, "the detached launch context must retain a bounded timeout")
			}
			require.Len(t, result.request.Attachments, 1)
			stored, err := repo.GetMessageAttachment(context.Background(), attachment.ID)
			require.NoError(t, err)
			if tc.wantClaimErr != nil {
				assert.Equal(t, models.AttachmentStateStaged, stored.State)
				assert.Empty(t, stored.TaskID)
			} else {
				assert.Equal(t, models.AttachmentStateClaimed, stored.State)
				assert.NotEmpty(t, stored.TaskID)
				assert.Equal(t, "config-session", stored.SessionID)
			}
		})
	}
}

func TestHTTPConfigChatRestoresClaimedAttachmentWhenLaunchFails(t *testing.T) {
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "config-chat-launch-failure.db"))
	require.NoError(t, err)
	dbConnSQLX := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = dbConnSQLX.Close() })
	repo, cleanup, err := repository.Provide(dbConnSQLX, dbConnSQLX, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })
	log := newTestLogger(t)
	agentProfileID := "profile-1"
	executorID := models.ExecutorIDLocal
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "ws-1", Name: "Workspace", DefaultAgentProfileID: &agentProfileID,
		DefaultExecutorID: &executorID,
	}))
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, bus.NewMemoryEventBus(log), log, service.RepositoryDiscoveryConfig{})
	attachmentSvc, err := service.NewAttachmentService(repo, t.TempDir(), nil, log)
	require.NoError(t, err)
	svc.SetAttachmentService(attachmentSvc)
	attachment, err := attachmentSvc.Stage(
		context.Background(), "user-1", "ws-1", "trace.txt", "text/plain", "resource", "path", strings.NewReader("trace bytes"),
	)
	require.NoError(t, err)
	orch := &configAttachmentClaimOrchestrator{
		service:   svc,
		results:   make(chan configAttachmentClaimResult, 1),
		launchErr: errors.New("launch failed after claim"),
	}
	h := &TaskHandlers{service: svc, orchestrator: orch, logger: log}
	body := `{"agent_profile_id":"profile-1","prompt":"Review this trace","attachments":[{"type":"resource","attachment_id":"` +
		attachment.ID + `","mime_type":"text/plain","name":"trace.txt","size_bytes":11,"delivery_mode":"path"}]}`
	c, rec := quickChatRequestContext(t, "/workspaces/ws-1/config-chat", body)
	c.Request = c.Request.WithContext(authn.WithIdentity(
		c.Request.Context(), authn.Identity{UserID: "user-1", Role: authn.RoleMember},
	))

	h.httpStartConfigChat(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	result := <-orch.results
	require.NoError(t, result.err, "the real claimer accepts the staged owner's attachment before launch fails")
	restored, err := repo.GetMessageAttachment(context.Background(), attachment.ID)
	require.NoError(t, err)
	assert.Equal(t, models.AttachmentStateStaged, restored.State)
	assert.Empty(t, restored.TaskID)
	_, file, err := attachmentSvc.Open(context.Background(), "user-1", attachment.ID)
	require.NoError(t, err, "the staged upload remains available for retry")
	require.NoError(t, file.Close())
	_, err = svc.GetTask(context.Background(), result.request.TaskID)
	require.Error(t, err, "the failed config chat task is rolled back")
}

func TestHTTPQuickChatRejectsAttachmentsWithoutPrompt(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
		call func(*TaskHandlers, *gin.Context)
	}{
		{
			name: "quick chat",
			path: "/workspaces/ws-1/quick-chat",
			body: `{"agent_profile_id":"profile-1","attachments":[{"type":"resource","data":"dGVzdA==","mime_type":"text/plain"}]}`,
			call: func(h *TaskHandlers, c *gin.Context) { h.httpStartQuickChat(c) },
		},
		{
			name: "config chat",
			path: "/workspaces/ws-1/config-chat",
			body: `{"agent_profile_id":"profile-1","attachments":[{"type":"resource","data":"dGVzdA==","mime_type":"text/plain"}]}`,
			call: func(h *TaskHandlers, c *gin.Context) { h.httpStartConfigChat(c) },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, orch, repo := newQuickChatHandlerForTest(t)
			c, rec := quickChatRequestContext(t, tc.path, tc.body)

			tc.call(h, c)

			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Nil(t, repo.task)
			assert.Empty(t, orch.requests)
		})
	}
}

type claimThenFailQuickChatOrchestrator struct {
	service  *service.Service
	requests []*orchestrator.LaunchSessionRequest
}

func (o *claimThenFailQuickChatOrchestrator) LaunchSession(
	ctx context.Context,
	req *orchestrator.LaunchSessionRequest,
) (*orchestrator.LaunchSessionResponse, error) {
	o.requests = append(o.requests, req)
	if err := o.service.ClaimMessageAttachments(ctx, req.TaskID, req.SessionID, req.Attachments); err != nil {
		return nil, err
	}
	return nil, errors.New("launch failed after attachment claim")
}

func (*claimThenFailQuickChatOrchestrator) EnsureSession(
	context.Context,
	string,
	...orchestrator.EnsureSessionOptions,
) (*orchestrator.EnsureSessionResponse, error) {
	return nil, nil
}

func TestHTTPChatOpeningAttachmentRollback(t *testing.T) {
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "quick-chat-opening.db"))
	require.NoError(t, err)
	dbConnSQLX := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = dbConnSQLX.Close() })
	repo, cleanup, err := repository.Provide(dbConnSQLX, dbConnSQLX, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	agentProfileID := "profile-1"
	executorID := models.ExecutorIDLocal
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "ws-1", Name: "Workspace", DefaultAgentProfileID: &agentProfileID,
		DefaultExecutorID: &executorID,
	}))
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, bus.NewMemoryEventBus(log), log, service.RepositoryDiscoveryConfig{})
	attachmentSvc, err := service.NewAttachmentService(repo, t.TempDir(), nil, log)
	require.NoError(t, err)
	svc.SetAttachmentService(attachmentSvc)
	orch := &claimThenFailQuickChatOrchestrator{service: svc}
	h := &TaskHandlers{service: svc, orchestrator: orch, logger: log}

	attachment, err := attachmentSvc.Stage(
		context.Background(), "user-1", "ws-1", "trace.txt", "text/plain", "resource", "path", strings.NewReader("trace bytes"),
	)
	require.NoError(t, err)
	requestBody := `{"agent_profile_id":"profile-1","prompt":"Review this trace","attachments":[{"type":"resource","attachment_id":"` +
		attachment.ID + `","mime_type":"text/plain","name":"trace.txt","size_bytes":11,"delivery_mode":"path"}]}`
	c, rec := quickChatRequestContext(t, "/workspaces/ws-1/quick-chat", requestBody)
	c.Request = c.Request.WithContext(authn.WithIdentity(
		c.Request.Context(), authn.Identity{UserID: "user-1", Role: authn.RoleMember},
	))

	h.httpStartQuickChat(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Len(t, orch.requests, 1)
	require.Len(t, orch.requests[0].Attachments, 1)
	restored, err := repo.GetMessageAttachment(context.Background(), attachment.ID)
	require.NoError(t, err)
	assert.Equal(t, models.AttachmentStateStaged, restored.State)
	assert.Empty(t, restored.TaskID)
	_, file, err := attachmentSvc.Open(context.Background(), "user-1", attachment.ID)
	require.NoError(t, err, "the original upload remains usable for an explicit retry")
	require.NoError(t, file.Close())
	_, err = repo.GetTask(context.Background(), orch.requests[0].TaskID)
	require.Error(t, err, "the failed Quick Chat task is rolled back")
}
