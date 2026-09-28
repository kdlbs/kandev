package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
)

type taskManagementClaimHTTPRepo struct {
	mockRepository
	task    *models.Task
	claim   *models.TaskManagementClaim
	history []*models.TaskManagementClaimHistory
	change  models.TaskManagementClaimChange
}

func (r *taskManagementClaimHTTPRepo) GetTask(context.Context, string) (*models.Task, error) {
	copy := *r.task
	return &copy, nil
}

func (r *taskManagementClaimHTTPRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	return &models.Workspace{ID: id, OwnerID: "current-user"}, nil
}

func (r *taskManagementClaimHTTPRepo) GetTaskManagementClaim(context.Context, string) (*models.TaskManagementClaim, error) {
	if r.claim == nil {
		return nil, nil
	}
	copy := *r.claim
	return &copy, nil
}

func (r *taskManagementClaimHTTPRepo) ListTaskManagementClaimHistory(context.Context, string) ([]*models.TaskManagementClaimHistory, error) {
	return r.history, nil
}

func (r *taskManagementClaimHTTPRepo) ChangeTaskManagementClaim(_ context.Context, change models.TaskManagementClaimChange) (*models.TaskManagementClaim, error) {
	r.change = change
	if change.ExpectedTaskResourceVersion != r.task.UpdatedAt.UTC().Format(time.RFC3339Nano) {
		return nil, repoerrors.ErrTaskVersionConflict
	}
	if r.claim == nil || change.ExpectedClaimResourceVersion != r.claim.ResourceVersion {
		return nil, repoerrors.ErrTaskManagementClaimConflict
	}
	updated := *r.claim
	updated.Generation++
	updated.ResourceVersion = "claim-rv-next"
	updated.UpdatedByActor = change.ActorID
	updated.UpdatedAt = time.Now().UTC()
	updated.AcquiredAt = &updated.UpdatedAt
	if change.Action == models.TaskManagementClaimTransfer {
		updated.OwnerKind = change.OwnerKind
		updated.OwnerActorID = change.OwnerActorID
		updated.InstallationID = change.InstallationID
		updated.InstanceKey = change.InstanceKey
	} else {
		updated.OwnerKind = ""
		updated.OwnerActorID = ""
		updated.InstallationID = ""
		updated.InstanceKey = ""
	}
	r.claim = &updated
	return &updated, nil
}

func newTaskManagementClaimHTTPHandler(t *testing.T, repo *taskManagementClaimHTTPRepo, identity *authn.Identity) (*TaskHandlers, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := service.NewService(service.Repos{Tasks: repo, Workspaces: repo}, nil, newTestLogger(t), service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: newTestLogger(t)}
	router := gin.New()
	if identity != nil {
		router.Use(func(c *gin.Context) {
			authn.SetOnGin(c, *identity)
			c.Next()
		})
	}
	h.registerHTTP(router)
	return h, router
}

func newTaskManagementClaimHTTPRepo() *taskManagementClaimHTTPRepo {
	updatedAt := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	claimUpdatedAt := updatedAt.Add(time.Minute)
	return &taskManagementClaimHTTPRepo{
		task: &models.Task{ID: "task-1", WorkspaceID: "workspace-1", UpdatedAt: updatedAt},
		claim: &models.TaskManagementClaim{
			TaskID: "task-1", WorkspaceID: "workspace-1", OwnerKind: "plugin",
			InstallationID: "uninstalled-plugin", InstanceKey: "manager-a", Generation: 3,
			ResourceVersion: "claim-rv-3", AcquiredAt: &claimUpdatedAt, UpdatedAt: claimUpdatedAt,
		},
		history: []*models.TaskManagementClaimHistory{{
			ID: "history-1", TaskID: "task-1", WorkspaceID: "workspace-1", Action: "acquired",
			InstallationID: "uninstalled-plugin", InstanceKey: "manager-a", Generation: 3,
			ActorID: "plugin:uninstalled-plugin", Reason: "accepted task", ResourceVersion: "claim-rv-3", CreatedAt: claimUpdatedAt,
		}},
	}
}

func TestHTTPGetTaskManagementClaimShowsUnavailableOwnerAndHistory(t *testing.T) {
	repo := newTaskManagementClaimHTTPRepo()
	_, router := newTaskManagementClaimHTTPHandler(t, repo, &authn.Identity{UserID: "current-user", Synthetic: true})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-1/management-claim", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body struct {
		Claim   models.TaskManagementClaim          `json:"claim"`
		History []models.TaskManagementClaimHistory `json:"history"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, "uninstalled-plugin", body.Claim.InstallationID)
	require.Equal(t, "manager-a", body.Claim.InstanceKey)
	require.Len(t, body.History, 1)
	require.Equal(t, "plugin:uninstalled-plugin", body.History[0].ActorID)
}

func TestHTTPTransferTaskManagementClaimBindsCurrentHumanActor(t *testing.T) {
	repo := newTaskManagementClaimHTTPRepo()
	_, router := newTaskManagementClaimHTTPHandler(t, repo, &authn.Identity{UserID: "current-user", Synthetic: true})
	body := `{"action":"transfer","expected_task_resource_version":"2026-09-26T10:00:00Z","expected_claim_resource_version":"claim-rv-3","reason":"Take over unavailable manager"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/management-claim", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "current-user", repo.change.ActorID)
	require.Equal(t, "human", repo.change.OwnerKind)
	require.Equal(t, "current-user", repo.change.OwnerActorID)
	require.Equal(t, "Take over unavailable manager", repo.change.Reason)
	require.Equal(t, "human", repo.claim.OwnerKind)
	require.Equal(t, "current-user", repo.claim.OwnerActorID)
}

func TestHTTPReleaseTaskManagementClaim(t *testing.T) {
	repo := newTaskManagementClaimHTTPRepo()
	_, router := newTaskManagementClaimHTTPHandler(t, repo, &authn.Identity{UserID: "current-user", Synthetic: true})
	body := `{"action":"release","expected_task_resource_version":"2026-09-26T10:00:00Z","expected_claim_resource_version":"claim-rv-3","reason":"Return task to shared management"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/management-claim", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "current-user", repo.change.ActorID)
	require.Empty(t, repo.claim.OwnerKind)
	require.Empty(t, repo.claim.InstallationID)
}

func TestHTTPChangeTaskManagementClaimRejectsStaleVersionsAndMissingIdentity(t *testing.T) {
	tests := []struct {
		name     string
		identity *authn.Identity
		body     string
		wantCode int
	}{
		{
			name:     "stale claim",
			identity: &authn.Identity{UserID: "current-user", Synthetic: true},
			body:     `{"action":"release","expected_task_resource_version":"2026-09-26T10:00:00Z","expected_claim_resource_version":"old-claim-rv","reason":"release"}`,
			wantCode: http.StatusConflict,
		},
		{
			name:     "missing identity",
			body:     `{"action":"release","expected_task_resource_version":"2026-09-26T10:00:00Z","expected_claim_resource_version":"claim-rv-3","reason":"release"}`,
			wantCode: http.StatusUnauthorized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, router := newTaskManagementClaimHTTPHandler(t, newTaskManagementClaimHTTPRepo(), tt.identity)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/management-claim", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, tt.wantCode, response.Code, response.Body.String())
		})
	}
}
