package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// executorRepo backs both the executor and executor-profile handler tests.
type executorRepo struct {
	mockRepository

	executors []*models.Executor
	byID      map[string]*models.Executor
	profiles  []*models.ExecutorProfile
	// profilesByExecutor answers ListExecutorProfiles(executorID).
	profilesByExecutor map[string][]*models.ExecutorProfile
	profileByID        map[string]*models.ExecutorProfile

	listErr           error
	listProfilesErr   error
	createErr         error
	updateErr         error
	deleteErr         error
	hasActiveSessions bool
	running           []*models.ExecutorRunning

	created        []*models.Executor
	updated        []*models.Executor
	deleted        []string
	createdProfile []*models.ExecutorProfile
	updatedProfile []*models.ExecutorProfile
	deletedProfile []string
}

func (r *executorRepo) ListExecutors(context.Context) ([]*models.Executor, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.executors, nil
}

func (r *executorRepo) GetExecutor(_ context.Context, id string) (*models.Executor, error) {
	if executor, ok := r.byID[id]; ok {
		return executor, nil
	}
	return nil, fmt.Errorf("executor not found: %s", id)
}

func (r *executorRepo) CreateExecutor(_ context.Context, executor *models.Executor) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.created = append(r.created, executor)
	return nil
}

func (r *executorRepo) UpdateExecutor(_ context.Context, executor *models.Executor) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.updated = append(r.updated, executor)
	return nil
}

func (r *executorRepo) DeleteExecutor(_ context.Context, id string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.deleted = append(r.deleted, id)
	return nil
}

func (r *executorRepo) HasActiveTaskSessionsByExecutor(context.Context, string) (bool, error) {
	return r.hasActiveSessions, nil
}

func (r *executorRepo) ListExecutorsRunning(context.Context) ([]*models.ExecutorRunning, error) {
	return r.running, nil
}

func (r *executorRepo) ListAllExecutorProfiles(context.Context) ([]*models.ExecutorProfile, error) {
	if r.listProfilesErr != nil {
		return nil, r.listProfilesErr
	}
	return r.profiles, nil
}

func (r *executorRepo) ListExecutorProfiles(_ context.Context, executorID string) ([]*models.ExecutorProfile, error) {
	if r.listProfilesErr != nil {
		return nil, r.listProfilesErr
	}
	return r.profilesByExecutor[executorID], nil
}

func (r *executorRepo) GetExecutorProfile(_ context.Context, id string) (*models.ExecutorProfile, error) {
	if profile, ok := r.profileByID[id]; ok {
		return profile, nil
	}
	return nil, fmt.Errorf("executor profile not found: %s", id)
}

func (r *executorRepo) CreateExecutorProfile(_ context.Context, profile *models.ExecutorProfile) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.createdProfile = append(r.createdProfile, profile)
	return nil
}

func (r *executorRepo) UpdateExecutorProfile(_ context.Context, profile *models.ExecutorProfile) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.updatedProfile = append(r.updatedProfile, profile)
	return nil
}

func (r *executorRepo) DeleteExecutorProfile(_ context.Context, id string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.deletedProfile = append(r.deletedProfile, id)
	return nil
}

func newExecutorService(t *testing.T, repo *executorRepo) *service.Service {
	t.Helper()
	log := newTestLogger(t)
	return service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
}

func newExecutorHandlers(t *testing.T, repo *executorRepo) *ExecutorHandlers {
	t.Helper()
	return NewExecutorHandlers(newExecutorService(t, repo), newTestLogger(t))
}

// executorFixture holds two executors, one of them carrying a profile, so the
// list handler's profile-grouping has something to get wrong.
func executorFixture() *executorRepo {
	docker := &models.Executor{ID: "ex-docker", Name: "Docker", Type: models.ExecutorTypeLocalDocker, IsSystem: true}
	local := &models.Executor{ID: "ex-local", Name: "Local", Type: models.ExecutorTypeLocal}
	profile := &models.ExecutorProfile{ID: "pr-1", ExecutorID: "ex-docker", Name: "Default"}
	return &executorRepo{
		executors: []*models.Executor{docker, local},
		byID:      map[string]*models.Executor{"ex-docker": docker, "ex-local": local},
		profiles:  []*models.ExecutorProfile{profile},
		profilesByExecutor: map[string][]*models.ExecutorProfile{
			"ex-docker": {profile},
		},
		profileByID: map[string]*models.ExecutorProfile{"pr-1": profile},
	}
}

func decodeExecutorList(t *testing.T, body []byte) dto.ListExecutorsResponse {
	t.Helper()
	var resp dto.ListExecutorsResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp
}

func TestHTTPListExecutorsAttachesProfilesToTheirExecutor(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/executors", "")

	h.httpListExecutors(c)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeExecutorList(t, rec.Body.Bytes())
	require.Equal(t, 2, resp.Total)
	require.Len(t, resp.Executors, 2)
	require.Equal(t, "ex-docker", resp.Executors[0].ID)
	require.Len(t, resp.Executors[0].Profiles, 1)
	require.Equal(t, "pr-1", resp.Executors[0].Profiles[0].ID)
	require.Empty(t, resp.Executors[1].Profiles, "an executor without profiles must not inherit another's")
}

func TestHTTPListExecutorsReturnsInternalErrorWhenListFails(t *testing.T) {
	repo := executorFixture()
	repo.listErr = errors.New("database unavailable")
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/executors", "")

	h.httpListExecutors(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"Failed to list executors"}`, rec.Body.String())
}

// TestHTTPListExecutorsToleratesProfileLookupFailure documents that the profile
// read is best-effort: a failure there must still return the executors rather
// than blanking the settings page.
func TestHTTPListExecutorsToleratesProfileLookupFailure(t *testing.T) {
	repo := executorFixture()
	repo.listProfilesErr = errors.New("profiles unavailable")
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/executors", "")

	h.httpListExecutors(c)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeExecutorList(t, rec.Body.Bytes())
	require.Len(t, resp.Executors, 2)
	require.Empty(t, resp.Executors[0].Profiles)
}

func TestHTTPCreateExecutorRejectsInvalidRequests(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)

	for name, tc := range map[string]struct {
		body     string
		wantBody string
	}{
		"malformed json": {body: `{`, wantBody: `{"error":"invalid payload"}`},
		"missing name":   {body: `{"type":"local"}`, wantBody: `{"error":"name and type are required"}`},
		"missing type":   {body: `{"name":"Local"}`, wantBody: `{"error":"name and type are required"}`},
	} {
		t.Run(name, func(t *testing.T) {
			c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/executors", tc.body)
			h.httpCreateExecutor(c)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.JSONEq(t, tc.wantBody, rec.Body.String())
		})
	}
	require.Empty(t, repo.created)
}

// TestHTTPCreateExecutorDefaultsStatusAndResumable pins the two implicit
// defaults: an omitted status becomes "active" and an omitted resumable
// becomes true (not the Go zero value false).
func TestHTTPCreateExecutorDefaultsStatusAndResumable(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/executors",
		`{"name":"New","type":"local"}`)

	h.httpCreateExecutor(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, repo.created, 1)
	require.Equal(t, models.ExecutorStatusActive, repo.created[0].Status)
	require.True(t, repo.created[0].Resumable)

	var got dto.ExecutorDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "New", got.Name)
	require.Equal(t, models.ExecutorTypeLocal, got.Type)
	require.True(t, got.Resumable)
}

func TestHTTPCreateExecutorHonorsExplicitResumableFalse(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/executors",
		`{"name":"New","type":"local","status":"inactive","resumable":false}`)

	h.httpCreateExecutor(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, repo.created, 1)
	require.False(t, repo.created[0].Resumable)
	require.Equal(t, models.ExecutorStatus("inactive"), repo.created[0].Status)
}

func TestHTTPCreateExecutorRejectsInvalidMCPPolicy(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/executors",
		`{"name":"New","type":"local","config":{"mcp_policy":"not-json"}}`)

	h.httpCreateExecutor(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), service.ErrInvalidExecutorConfig.Error())
	require.Empty(t, repo.created)
}

func TestHTTPCreateExecutorSurfacesRepositoryFailure(t *testing.T) {
	repo := executorFixture()
	repo.createErr = errors.New("disk full")
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/executors",
		`{"name":"New","type":"local"}`)

	h.httpCreateExecutor(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"executor not created"}`, rec.Body.String())
}

func TestHTTPGetExecutorReturnsExecutorOrNotFound(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)

	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/executors/ex-local", "")
	c.Params = gin.Params{{Key: "id", Value: "ex-local"}}
	h.httpGetExecutor(c)
	require.Equal(t, http.StatusOK, rec.Code)
	var got dto.ExecutorDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "ex-local", got.ID)
	require.Equal(t, "Local", got.Name)

	missingCtx, missingRec := newWorkflowRequest(t, http.MethodGet, "/api/v1/executors/nope", "")
	missingCtx.Params = gin.Params{{Key: "id", Value: "nope"}}
	h.httpGetExecutor(missingCtx)
	require.Equal(t, http.StatusNotFound, missingRec.Code)
	require.JSONEq(t, `{"error":"executor not found"}`, missingRec.Body.String())
}

func TestHTTPUpdateExecutorAppliesProvidedFieldsOnly(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodPatch, "/api/v1/executors/ex-local",
		`{"name":"Renamed","resumable":false}`)
	c.Params = gin.Params{{Key: "id", Value: "ex-local"}}

	h.httpUpdateExecutor(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, repo.updated, 1)
	require.Equal(t, "Renamed", repo.updated[0].Name)
	require.False(t, repo.updated[0].Resumable)
	require.Equal(t, models.ExecutorTypeLocal, repo.updated[0].Type, "an omitted field must be left alone")
}

func TestHTTPUpdateExecutorRejectsInvalidPayload(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodPatch, "/api/v1/executors/ex-local", `{`)
	c.Params = gin.Params{{Key: "id", Value: "ex-local"}}

	h.httpUpdateExecutor(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"invalid payload"}`, rec.Body.String())
	require.Empty(t, repo.updated)
}

func TestHTTPUpdateExecutorRejectsInvalidMCPPolicy(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodPatch, "/api/v1/executors/ex-local",
		`{"config":{"mcp_policy":"[1,2]"}}`)
	c.Params = gin.Params{{Key: "id", Value: "ex-local"}}

	h.httpUpdateExecutor(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "mcp_policy must be a JSON object")
	require.Empty(t, repo.updated)
}

func TestHTTPUpdateExecutorReturnsConflictForRetainedKubernetesInventory(t *testing.T) {
	repo := kubernetesHandlerFixture()
	repo.running = []*models.ExecutorRunning{{
		SessionID: "session-retained", TaskID: "task-retained",
		ExecutorID: "ex-kubernetes", Runtime: agentruntime.RuntimeKubernetes,
	}}
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodPatch, "/api/v1/executors/ex-kubernetes", `{"type":"local"}`)
	c.Params = gin.Params{{Key: "id", Value: "ex-kubernetes"}}
	c.Request = c.Request.WithContext(kubernetesAdminContext())

	h.httpUpdateExecutor(c)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.JSONEq(t, `{"error":"executor is used by an active agent session"}`, rec.Body.String())
	require.Empty(t, repo.updated)
}

// TestHTTPUpdateExecutorRefusesToRenameSystemExecutor covers the service-side
// system guard reaching the handler as a plain 500 rather than a 400 — the
// error is not an ErrInvalidExecutorConfig, so it takes the generic branch.
func TestHTTPUpdateExecutorRefusesToRenameSystemExecutor(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodPatch, "/api/v1/executors/ex-docker",
		`{"name":"Renamed"}`)
	c.Params = gin.Params{{Key: "id", Value: "ex-docker"}}

	h.httpUpdateExecutor(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"executor not updated"}`, rec.Body.String())
	require.Empty(t, repo.updated, "a system executor must not be modified")
}

func TestHTTPDeleteExecutorDeletesAndReportsConflicts(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodDelete, "/api/v1/executors/ex-local", "")
	c.Params = gin.Params{{Key: "id", Value: "ex-local"}}

	h.httpDeleteExecutor(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"success":true}`, rec.Body.String())
	require.Equal(t, []string{"ex-local"}, repo.deleted)
}

func TestHTTPDeleteExecutorReturnsConflictWhenSessionsAreActive(t *testing.T) {
	repo := executorFixture()
	repo.hasActiveSessions = true
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodDelete, "/api/v1/executors/ex-local", "")
	c.Params = gin.Params{{Key: "id", Value: "ex-local"}}

	h.httpDeleteExecutor(c)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.JSONEq(t, `{"error":"executor is used by an active agent session"}`, rec.Body.String())
	require.Empty(t, repo.deleted, "an in-use executor must survive the request")
}

func TestHTTPDeleteExecutorReturnsBadRequestForUnknownExecutor(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)
	c, rec := newWorkflowRequest(t, http.MethodDelete, "/api/v1/executors/nope", "")
	c.Params = gin.Params{{Key: "id", Value: "nope"}}

	h.httpDeleteExecutor(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"executor not deleted"}`, rec.Body.String())
}

func TestWSListExecutorsReturnsExecutorsWithProfiles(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)

	resp, err := h.wsListExecutors(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorList, map[string]any{}))

	require.NoError(t, err)
	var list dto.ListExecutorsResponse
	wsWorkflowResponse(t, resp, &list)
	require.Equal(t, 2, list.Total)
	require.Len(t, list.Executors[0].Profiles, 1)
}

func TestWSListExecutorsSurfacesRepositoryFailure(t *testing.T) {
	repo := executorFixture()
	repo.listErr = errors.New("database unavailable")
	h := newExecutorHandlers(t, repo)

	resp, err := h.wsListExecutors(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorList, map[string]any{}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeInternalError), payload.Code)
	require.Equal(t, "Failed to list executors", payload.Message)
}

func TestWSCreateExecutorValidatesAndCreates(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)

	invalid, err := h.wsCreateExecutor(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorCreate, map[string]any{"name": "New"}))
	require.NoError(t, err)
	require.Equal(t, "name and type are required", wsWorkflowError(t, invalid).Message)

	malformed, err := h.wsCreateExecutor(context.Background(), wsMalformedRequest(ws.ActionExecutorCreate))
	require.NoError(t, err)
	require.Equal(t, string(ws.ErrorCodeBadRequest), wsWorkflowError(t, malformed).Code)
	require.Empty(t, repo.created)

	resp, err := h.wsCreateExecutor(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorCreate, map[string]any{"name": "New", "type": "local"}))
	require.NoError(t, err)
	var created dto.ExecutorDTO
	wsWorkflowResponse(t, resp, &created)
	require.Equal(t, "New", created.Name)
	require.True(t, created.Resumable, "resumable defaults to true over the WS surface too")
	require.Len(t, repo.created, 1)
}

func TestWSCreateExecutorRejectsInvalidMCPPolicy(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)

	resp, err := h.wsCreateExecutor(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorCreate, map[string]any{
			"name": "New", "type": "local", "config": map[string]string{"mcp_policy": "nope"},
		}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeValidation), payload.Code)
	require.Equal(t, service.ErrInvalidExecutorConfig.Error(), payload.Message)
	require.Empty(t, repo.created)
}

func TestWSGetExecutorReturnsExecutorOrErrors(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)

	resp, err := h.wsGetExecutor(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorGet, map[string]any{"id": "ex-local"}))
	require.NoError(t, err)
	var got dto.ExecutorDTO
	wsWorkflowResponse(t, resp, &got)
	require.Equal(t, "ex-local", got.ID)

	missing, err := h.wsGetExecutor(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorGet, map[string]any{"id": "nope"}))
	require.NoError(t, err)
	require.Equal(t, string(ws.ErrorCodeNotFound), wsWorkflowError(t, missing).Code)

	empty, err := h.wsGetExecutor(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorGet, map[string]any{}))
	require.NoError(t, err)
	require.Equal(t, "id is required", wsWorkflowError(t, empty).Message)
}

func TestWSUpdateExecutorAppliesFields(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)

	resp, err := h.wsUpdateExecutor(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorUpdate, map[string]any{
			"id": "ex-local", "name": "Renamed", "status": "inactive",
		}))

	require.NoError(t, err)
	var updated dto.ExecutorDTO
	wsWorkflowResponse(t, resp, &updated)
	require.Equal(t, "Renamed", updated.Name)
	require.Equal(t, models.ExecutorStatus("inactive"), updated.Status)
	require.Len(t, repo.updated, 1)
}

func TestWSUpdateExecutorRequiresID(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)

	resp, err := h.wsUpdateExecutor(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorUpdate, map[string]any{"name": "Renamed"}))

	require.NoError(t, err)
	require.Equal(t, "id is required", wsWorkflowError(t, resp).Message)
	require.Empty(t, repo.updated)
}

func TestWSUpdateExecutorSurfacesRepositoryFailure(t *testing.T) {
	repo := executorFixture()
	repo.updateErr = errors.New("disk full")
	h := newExecutorHandlers(t, repo)

	resp, err := h.wsUpdateExecutor(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorUpdate, map[string]any{"id": "ex-local", "name": "Renamed"}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeInternalError), payload.Code)
	require.Equal(t, "Failed to update executor", payload.Message)
}

func TestWSUpdateExecutorReturnsValidationForRetainedKubernetesInventory(t *testing.T) {
	repo := kubernetesHandlerFixture()
	repo.running = []*models.ExecutorRunning{{
		SessionID: "session-retained", TaskID: "task-retained",
		ExecutorID: "ex-kubernetes", Runtime: agentruntime.RuntimeKubernetes,
	}}
	h := newExecutorHandlers(t, repo)

	resp, err := h.wsUpdateExecutor(kubernetesAdminContext(),
		wsWorkflowRequest(t, ws.ActionExecutorUpdate, map[string]any{
			"id": "ex-kubernetes", "type": string(models.ExecutorTypeLocal),
		}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeValidation), payload.Code)
	require.Equal(t, "executor is used by an active agent session", payload.Message)
	require.Empty(t, repo.updated)
}

func TestWSDeleteExecutorDeletesAndReportsActiveSessions(t *testing.T) {
	repo := executorFixture()
	h := newExecutorHandlers(t, repo)

	resp, err := h.wsDeleteExecutor(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorDelete, map[string]any{"id": "ex-local"}))
	require.NoError(t, err)
	var success dto.SuccessResponse
	wsWorkflowResponse(t, resp, &success)
	require.True(t, success.Success)
	require.Equal(t, []string{"ex-local"}, repo.deleted)

	busy := executorFixture()
	busy.hasActiveSessions = true
	busyHandlers := newExecutorHandlers(t, busy)
	blocked, err := busyHandlers.wsDeleteExecutor(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorDelete, map[string]any{"id": "ex-local"}))
	require.NoError(t, err)
	payload := wsWorkflowError(t, blocked)
	require.Equal(t, string(ws.ErrorCodeValidation), payload.Code)
	require.Equal(t, "executor is used by an active agent session", payload.Message)
	require.Empty(t, busy.deleted)
}
