package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// scriptRepo holds one editable repository owned by "user-b" and one living in
// the read-only Improve Kandev workspace.
type scriptRepo struct {
	mockRepository

	listErr   error
	createErr error
	updateErr error
	deleteErr error

	created []*models.RepositoryScript
	updated []*models.RepositoryScript
	deleted []string
}

func (r *scriptRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	switch id {
	case "ws-1":
		return &models.Workspace{ID: "ws-1", Name: "Team", OwnerID: "user-b"}, nil
	case "ws-improve":
		return &models.Workspace{ID: "ws-improve", Name: models.WorkspaceNameImproveKandev}, nil
	default:
		return nil, fmt.Errorf("workspace not found: %s", id)
	}
}

func (r *scriptRepo) GetRepository(_ context.Context, id string) (*models.Repository, error) {
	switch id {
	case "repo-1":
		return &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "kandev"}, nil
	case "repo-improve":
		return &models.Repository{ID: "repo-improve", WorkspaceID: "ws-improve", Name: "kandev"}, nil
	default:
		return nil, fmt.Errorf("repository not found: %s", id)
	}
}

func (r *scriptRepo) GetRepositoryScript(_ context.Context, id string) (*models.RepositoryScript, error) {
	switch id {
	case "s-1":
		return &models.RepositoryScript{ID: "s-1", RepositoryID: "repo-1", Name: "build", Command: "make", Position: 1}, nil
	case "s-improve":
		return &models.RepositoryScript{ID: "s-improve", RepositoryID: "repo-improve", Name: "build", Command: "make"}, nil
	default:
		return nil, fmt.Errorf("repository script not found: %s", id)
	}
}

func (r *scriptRepo) ListRepositoryScripts(_ context.Context, repositoryID string) ([]*models.RepositoryScript, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	if repositoryID != "repo-1" {
		return nil, nil
	}
	return []*models.RepositoryScript{
		{ID: "s-1", RepositoryID: "repo-1", Name: "build", Command: "make", Position: 1},
	}, nil
}

func (r *scriptRepo) CreateRepositoryScript(_ context.Context, script *models.RepositoryScript) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.created = append(r.created, script)
	return nil
}

func (r *scriptRepo) UpdateRepositoryScript(_ context.Context, script *models.RepositoryScript) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.updated = append(r.updated, script)
	return nil
}

func (r *scriptRepo) DeleteRepositoryScript(_ context.Context, id string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.deleted = append(r.deleted, id)
	return nil
}

func newScriptHandlers(t *testing.T, repo *scriptRepo) *RepositoryHandlers {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return NewRepositoryHandlers(svc, log)
}

// newScriptRouter drives the script routes through the real router so status
// codes written with c.Status (the 204 on delete) are actually flushed.
func newScriptRouter(t *testing.T, repo *scriptRepo) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	newScriptHandlers(t, repo).registerHTTP(router)
	return router
}

func scriptRequest(t *testing.T, router *gin.Engine, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestHTTPListRepositoryScriptsReturnsScripts(t *testing.T) {
	repo := &scriptRepo{}
	router := newScriptRouter(t, repo)

	rec := scriptRequest(t, router, http.MethodGet, "/api/v1/repositories/repo-1/scripts", "")

	require.Equal(t, http.StatusOK, rec.Code)
	var resp dto.ListRepositoryScriptsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Total)
	require.Equal(t, "s-1", resp.Scripts[0].ID)
	require.Equal(t, "make", resp.Scripts[0].Command)
}

func TestHTTPListRepositoryScriptsSurfacesRepositoryFailure(t *testing.T) {
	repo := &scriptRepo{listErr: errors.New("database unavailable")}
	router := newScriptRouter(t, repo)

	rec := scriptRequest(t, router, http.MethodGet, "/api/v1/repositories/repo-1/scripts", "")

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"failed to list repository scripts"}`, rec.Body.String())
}

func TestHTTPCreateRepositoryScriptRejectsInvalidRequests(t *testing.T) {
	repo := &scriptRepo{}
	router := newScriptRouter(t, repo)

	for name, tc := range map[string]struct {
		body     string
		wantBody string
	}{
		"malformed json":  {body: `{`, wantBody: `{"error":"invalid request body"}`},
		"missing name":    {body: `{"command":"make"}`, wantBody: `{"error":"name and command are required"}`},
		"missing command": {body: `{"name":"build"}`, wantBody: `{"error":"name and command are required"}`},
	} {
		t.Run(name, func(t *testing.T) {
			rec := scriptRequest(t, router, http.MethodPost, "/api/v1/repositories/repo-1/scripts", tc.body)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.JSONEq(t, tc.wantBody, rec.Body.String())
		})
	}
	require.Empty(t, repo.created)
}

func TestHTTPCreateRepositoryScriptTakesRepositoryIDFromThePath(t *testing.T) {
	repo := &scriptRepo{}
	router := newScriptRouter(t, repo)

	rec := scriptRequest(t, router, http.MethodPost, "/api/v1/repositories/repo-1/scripts",
		`{"name":"test","command":"go test ./...","position":3}`)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Len(t, repo.created, 1)
	require.Equal(t, "repo-1", repo.created[0].RepositoryID)
	require.Equal(t, "test", repo.created[0].Name)
	require.Equal(t, "go test ./...", repo.created[0].Command)
	require.Equal(t, 3, repo.created[0].Position)

	var created dto.RepositoryScriptDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Equal(t, repo.created[0].ID, created.ID)
}

func TestHTTPCreateRepositoryScriptSurfacesRepositoryFailure(t *testing.T) {
	repo := &scriptRepo{createErr: errors.New("disk full")}
	router := newScriptRouter(t, repo)

	rec := scriptRequest(t, router, http.MethodPost, "/api/v1/repositories/repo-1/scripts",
		`{"name":"test","command":"go test"}`)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"failed to create repository script"}`, rec.Body.String())
}

func TestHTTPGetRepositoryScriptReturnsScriptOrNotFound(t *testing.T) {
	router := newScriptRouter(t, &scriptRepo{})

	rec := scriptRequest(t, router, http.MethodGet, "/api/v1/scripts/s-1", "")
	require.Equal(t, http.StatusOK, rec.Code)
	var got dto.RepositoryScriptDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "s-1", got.ID)
	require.Equal(t, "build", got.Name)

	missing := scriptRequest(t, router, http.MethodGet, "/api/v1/scripts/nope", "")
	require.Equal(t, http.StatusNotFound, missing.Code)
	require.JSONEq(t, `{"error":"repository script not found"}`, missing.Body.String())
}

func TestHTTPUpdateRepositoryScriptAppliesProvidedFieldsOnly(t *testing.T) {
	repo := &scriptRepo{}
	router := newScriptRouter(t, repo)

	rec := scriptRequest(t, router, http.MethodPut, "/api/v1/scripts/s-1", `{"command":"make all"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, repo.updated, 1)
	require.Equal(t, "make all", repo.updated[0].Command)
	require.Equal(t, "build", repo.updated[0].Name, "an omitted name must be preserved")
	require.Equal(t, 1, repo.updated[0].Position)
}

func TestHTTPUpdateRepositoryScriptRejectsInvalidBody(t *testing.T) {
	repo := &scriptRepo{}
	router := newScriptRouter(t, repo)

	rec := scriptRequest(t, router, http.MethodPut, "/api/v1/scripts/s-1", `{`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"invalid request body"}`, rec.Body.String())
	require.Empty(t, repo.updated)
}

func TestHTTPDeleteRepositoryScriptReturnsNoContent(t *testing.T) {
	repo := &scriptRepo{}
	router := newScriptRouter(t, repo)

	rec := scriptRequest(t, router, http.MethodDelete, "/api/v1/scripts/s-1", "")

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.String())
	require.Equal(t, []string{"s-1"}, repo.deleted)
}

func TestHTTPDeleteRepositoryScriptReturnsNotFoundForUnknownScript(t *testing.T) {
	repo := &scriptRepo{}
	router := newScriptRouter(t, repo)

	rec := scriptRequest(t, router, http.MethodDelete, "/api/v1/scripts/nope", "")

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"repository script not found"}`, rec.Body.String())
	require.Empty(t, repo.deleted)
}

// TestRepositoryScriptRoutesDenyForeignWorkspace covers per-user scoping on the
// script surface: scripts hold shell commands and are executed inside the
// session workspace, so reading or editing another user's is a real breach.
// Each case runs as the owner too, so the denial is proven to come from
// scoping rather than a broken fixture.
func TestRepositoryScriptRoutesDenyForeignWorkspace(t *testing.T) {
	repo := &scriptRepo{}
	router := newScriptRouter(t, repo)

	for name, tc := range map[string]struct {
		method    string
		target    string
		body      string
		ownerCode int
	}{
		"list":   {method: http.MethodGet, target: "/api/v1/repositories/repo-1/scripts", ownerCode: http.StatusOK},
		"get":    {method: http.MethodGet, target: "/api/v1/scripts/s-1", ownerCode: http.StatusOK},
		"update": {method: http.MethodPut, target: "/api/v1/scripts/s-1", body: `{"command":"rm -rf /"}`, ownerCode: http.StatusOK},
		"delete": {method: http.MethodDelete, target: "/api/v1/scripts/s-1", ownerCode: http.StatusNoContent},
	} {
		t.Run(name, func(t *testing.T) {
			foreign := scriptRequestAs(t, router, "user-a", tc.method, tc.target, tc.body)
			require.NotEqual(t, tc.ownerCode, foreign.Code,
				"a foreign caller must not get the owner's outcome (body: %s)", foreign.Body.String())
			require.NotContains(t, foreign.Body.String(), "make", "the script command must not leak")

			owner := scriptRequestAs(t, router, "user-b", tc.method, tc.target, tc.body)
			require.Equal(t, tc.ownerCode, owner.Code,
				"the owner must succeed — otherwise the check above proves nothing (body: %s)", owner.Body.String())
		})
	}
}

func scriptRequestAs(t *testing.T, router *gin.Engine, userID, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(authn.WithIdentity(req.Context(),
		authn.Identity{UserID: userID, Role: authn.RoleMember}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestWSListRepositoryScriptsReturnsScripts(t *testing.T) {
	h := newScriptHandlers(t, &scriptRepo{})

	resp, err := h.wsListRepositoryScripts(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptList, map[string]any{"repository_id": "repo-1"}))

	require.NoError(t, err)
	var list dto.ListRepositoryScriptsResponse
	wsWorkflowResponse(t, resp, &list)
	require.Equal(t, 1, list.Total)
	require.Equal(t, "s-1", list.Scripts[0].ID)
}

func TestWSListRepositoryScriptsRejectsMalformedPayload(t *testing.T) {
	h := newScriptHandlers(t, &scriptRepo{})

	resp, err := h.wsListRepositoryScripts(context.Background(),
		wsMalformedRequest(ws.ActionRepositoryScriptList))

	require.NoError(t, err)
	require.Equal(t, string(ws.ErrorCodeBadRequest), wsWorkflowError(t, resp).Code)
}

func TestWSListRepositoryScriptsSurfacesRepositoryFailure(t *testing.T) {
	h := newScriptHandlers(t, &scriptRepo{listErr: errors.New("database unavailable")})

	resp, err := h.wsListRepositoryScripts(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptList, map[string]any{"repository_id": "repo-1"}))

	require.NoError(t, err)
	require.Equal(t, "Failed to list repository scripts", wsWorkflowError(t, resp).Message)
}

func TestWSCreateRepositoryScriptValidatesAndCreates(t *testing.T) {
	repo := &scriptRepo{}
	h := newScriptHandlers(t, repo)

	invalid, err := h.wsCreateRepositoryScript(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptCreate, map[string]any{"repository_id": "repo-1"}))
	require.NoError(t, err)
	require.Equal(t, "repository_id, name, and command are required", wsWorkflowError(t, invalid).Message)
	require.Empty(t, repo.created)

	resp, err := h.wsCreateRepositoryScript(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptCreate, map[string]any{
			"repository_id": "repo-1", "name": "lint", "command": "make lint", "position": 2,
		}))
	require.NoError(t, err)
	var created dto.RepositoryScriptDTO
	wsWorkflowResponse(t, resp, &created)
	require.Equal(t, "lint", created.Name)
	require.Len(t, repo.created, 1)
	require.Equal(t, 2, repo.created[0].Position)
}

func TestWSCreateRepositoryScriptReturnsNotFoundForUnknownRepository(t *testing.T) {
	repo := &scriptRepo{}
	h := newScriptHandlers(t, repo)

	resp, err := h.wsCreateRepositoryScript(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptCreate, map[string]any{
			"repository_id": "nope", "name": "lint", "command": "make lint",
		}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeNotFound), payload.Code)
	require.Equal(t, "Repository not found", payload.Message)
	require.Empty(t, repo.created)
}

// TestWSRepositoryScriptMutationsRejectImproveKandevWorkspace pins the
// read-only guard across create, update and delete, with the no-write
// assertion that makes each one meaningful.
func TestWSRepositoryScriptMutationsRejectImproveKandevWorkspace(t *testing.T) {
	repo := &scriptRepo{}
	h := newScriptHandlers(t, repo)

	create, err := h.wsCreateRepositoryScript(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptCreate, map[string]any{
			"repository_id": "repo-improve", "name": "lint", "command": "make lint",
		}))
	require.NoError(t, err)
	require.Equal(t, string(ws.ErrorCodeConflict), wsWorkflowError(t, create).Code)
	require.Equal(t, workspaceReadOnlyMsg, wsWorkflowError(t, create).Message)
	require.Empty(t, repo.created)

	update, err := h.wsUpdateRepositoryScript(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptUpdate, map[string]any{
			"id": "s-improve", "command": "make other",
		}))
	require.NoError(t, err)
	require.Equal(t, string(ws.ErrorCodeConflict), wsWorkflowError(t, update).Code)
	require.Empty(t, repo.updated)

	del, err := h.wsDeleteRepositoryScript(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptDelete, map[string]any{"id": "s-improve"}))
	require.NoError(t, err)
	require.Equal(t, string(ws.ErrorCodeConflict), wsWorkflowError(t, del).Code)
	require.Empty(t, repo.deleted)
}

func TestWSGetRepositoryScriptReturnsScriptOrErrors(t *testing.T) {
	h := newScriptHandlers(t, &scriptRepo{})

	resp, err := h.wsGetRepositoryScript(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptGet, map[string]any{"id": "s-1"}))
	require.NoError(t, err)
	var got dto.RepositoryScriptDTO
	wsWorkflowResponse(t, resp, &got)
	require.Equal(t, "s-1", got.ID)

	missing, err := h.wsGetRepositoryScript(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptGet, map[string]any{"id": "nope"}))
	require.NoError(t, err)
	require.Equal(t, string(ws.ErrorCodeNotFound), wsWorkflowError(t, missing).Code)

	empty, err := h.wsGetRepositoryScript(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptGet, map[string]any{}))
	require.NoError(t, err)
	require.Equal(t, "id is required", wsWorkflowError(t, empty).Message)
}

func TestWSUpdateRepositoryScriptAppliesFields(t *testing.T) {
	repo := &scriptRepo{}
	h := newScriptHandlers(t, repo)

	resp, err := h.wsUpdateRepositoryScript(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptUpdate, map[string]any{
			"id": "s-1", "name": "build-all",
		}))

	require.NoError(t, err)
	var updated dto.RepositoryScriptDTO
	wsWorkflowResponse(t, resp, &updated)
	require.Equal(t, "build-all", updated.Name)
	require.Len(t, repo.updated, 1)
	require.Equal(t, "make", repo.updated[0].Command, "an omitted command must be preserved")
}

func TestWSUpdateRepositoryScriptRequiresID(t *testing.T) {
	repo := &scriptRepo{}
	h := newScriptHandlers(t, repo)

	resp, err := h.wsUpdateRepositoryScript(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptUpdate, map[string]any{"name": "x"}))

	require.NoError(t, err)
	require.Equal(t, "id is required", wsWorkflowError(t, resp).Message)
	require.Empty(t, repo.updated)
}

func TestWSDeleteRepositoryScriptDeletesAndValidates(t *testing.T) {
	repo := &scriptRepo{}
	h := newScriptHandlers(t, repo)

	empty, err := h.wsDeleteRepositoryScript(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptDelete, map[string]any{}))
	require.NoError(t, err)
	require.Equal(t, "id is required", wsWorkflowError(t, empty).Message)
	require.Empty(t, repo.deleted)

	resp, err := h.wsDeleteRepositoryScript(context.Background(),
		wsWorkflowRequest(t, ws.ActionRepositoryScriptDelete, map[string]any{"id": "s-1"}))
	require.NoError(t, err)
	var body map[string]any
	wsWorkflowResponse(t, resp, &body)
	require.Equal(t, true, body["deleted"])
	require.Equal(t, []string{"s-1"}, repo.deleted)
}
