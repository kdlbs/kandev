package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// recordingAgentLister proves buildRemoteAuthSpecs consults the wired lister
// instead of silently falling back to the agentless catalog.
type recordingAgentLister struct {
	calls int
}

func (l *recordingAgentLister) ListEnabled() []agents.Agent {
	l.calls++
	return nil
}

type fixedAgentLister struct {
	agents []agents.Agent
}

func (l fixedAgentLister) ListEnabled() []agents.Agent { return l.agents }

func newProfileHandlers(t *testing.T, repo *executorRepo, lister AgentLister) *ExecutorProfileHandlers {
	t.Helper()
	return NewExecutorProfileHandlers(newExecutorService(t, repo), lister, newTestLogger(t))
}

func decodeProfileList(t *testing.T, body []byte) dto.ListExecutorProfilesResponse {
	t.Helper()
	var resp dto.ListExecutorProfilesResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp
}

// TestHTTPListAllProfilesEnrichesWithExecutorIdentity pins the join: each
// profile must carry the name and type of *its own* executor.
func TestHTTPListAllProfilesEnrichesWithExecutorIdentity(t *testing.T) {
	repo := executorFixture()
	repo.profiles = append(repo.profiles,
		&models.ExecutorProfile{ID: "pr-2", ExecutorID: "ex-local", Name: "Local Default"})
	h := newProfileHandlers(t, repo, nil)
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/executor-profiles", "")

	h.httpListAllProfiles(c)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeProfileList(t, rec.Body.Bytes())
	require.Equal(t, 2, resp.Total)
	require.Equal(t, "Docker", resp.Profiles[0].ExecutorName)
	require.Equal(t, string(models.ExecutorTypeLocalDocker), resp.Profiles[0].ExecutorType)
	require.Equal(t, "Local", resp.Profiles[1].ExecutorName)
	require.Equal(t, string(models.ExecutorTypeLocal), resp.Profiles[1].ExecutorType)
}

func TestHTTPListAllProfilesReturnsInternalErrorWhenProfilesFail(t *testing.T) {
	repo := executorFixture()
	repo.listProfilesErr = errors.New("database unavailable")
	h := newProfileHandlers(t, repo, nil)
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/executor-profiles", "")

	h.httpListAllProfiles(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"Failed to list profiles"}`, rec.Body.String())
}

func TestHTTPListAllProfilesReturnsInternalErrorWhenExecutorsFail(t *testing.T) {
	repo := executorFixture()
	repo.listErr = errors.New("database unavailable")
	h := newProfileHandlers(t, repo, nil)
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/executor-profiles", "")

	h.httpListAllProfiles(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"Failed to list executors"}`, rec.Body.String())
}

func TestHTTPListScriptPlaceholdersReturnsTheRegistry(t *testing.T) {
	h := newProfileHandlers(t, executorFixture(), nil)
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/script-placeholders", "")

	h.httpListScriptPlaceholders(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Placeholders []struct {
			Name string `json:"name"`
		} `json:"placeholders"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Placeholders, len(scriptPlaceholders))
	require.NotEmpty(t, body.Placeholders, "the placeholder registry must not serialize as empty")
}

func TestHTTPGetDefaultScriptRequiresTypeAndReturnsTheScript(t *testing.T) {
	h := newProfileHandlers(t, executorFixture(), nil)

	missingCtx, missingRec := newWorkflowRequest(t, http.MethodGet,
		"/api/v1/executor-profiles/default-script", "")
	h.httpGetDefaultScript(missingCtx)
	require.Equal(t, http.StatusBadRequest, missingRec.Code)
	require.JSONEq(t, `{"error":"type query parameter is required"}`, missingRec.Body.String())

	c, rec := newWorkflowRequest(t, http.MethodGet,
		"/api/v1/executor-profiles/default-script?type=worktree", "")
	h.httpGetDefaultScript(c)
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		PrepareScript string `json:"prepare_script"`
		CleanupScript string `json:"cleanup_script"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, lifecycle.DefaultPrepareScript("worktree"), body.PrepareScript)
	require.NotEmpty(t, body.PrepareScript, "worktree has a default prepare script")
	require.Empty(t, body.CleanupScript)
}

func TestHTTPListRemoteCredentialsUsesTheWiredAgentLister(t *testing.T) {
	lister := &recordingAgentLister{}
	h := newProfileHandlers(t, executorFixture(), lister)
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/remote-credentials", "")

	h.httpListRemoteCredentials(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, lister.calls, "the handler must ask the lister which agents are enabled")
	var body struct {
		AuthSpecs []struct {
			ID      string `json:"id"`
			Methods []struct {
				MethodID string `json:"method_id"`
			} `json:"methods"`
		} `json:"auth_specs"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotEmpty(t, body.AuthSpecs, "the GitHub CLI spec is always present")
	require.NotEmpty(t, body.AuthSpecs[0].Methods)
}

func TestHTTPListRemoteCredentialsFallsBackWithoutAnAgentLister(t *testing.T) {
	h := newProfileHandlers(t, executorFixture(), nil)
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/remote-credentials", "")

	h.httpListRemoteCredentials(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"auth_specs"`)
}

func TestHTTPListAgentConfigBundlesReturnsMetadataOnly(t *testing.T) {
	h := newProfileHandlers(t, executorFixture(), fixedAgentLister{
		agents: []agents.Agent{agents.NewClaudeACP(), agents.NewCodexACP(), agents.NewOpenCodeACP()},
	})
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/agent-config-bundles", "")

	h.httpListAgentConfigBundles(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Bundles []struct {
			ID    string `json:"id"`
			Files []struct {
				SourcePath string `json:"source_path"`
				TargetPath string `json:"target_path"`
			} `json:"files"`
		} `json:"bundles"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Bundles, 3)
	for _, bundle := range body.Bundles {
		require.NotEmpty(t, bundle.ID)
		for _, file := range bundle.Files {
			require.NotEmpty(t, file.SourcePath)
			require.NotEmpty(t, file.TargetPath)
			require.NotContains(t, file.SourcePath, string(filepath.Separator)+"root"+string(filepath.Separator))
		}
	}
	require.NotContains(t, rec.Body.String(), "host-only-secret")
}

// TestHTTPGetGitIdentityReportsDetectionConsistently shells out to the real
// git, so the values depend on the host. The invariant that must hold either
// way is that "detected" agrees with the two fields it summarizes.
func TestHTTPGetGitIdentityReportsDetectionConsistently(t *testing.T) {
	h := newProfileHandlers(t, executorFixture(), nil)
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/git/identity", "")

	h.httpGetGitIdentity(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		UserName  string `json:"user_name"`
		UserEmail string `json:"user_email"`
		Detected  bool   `json:"detected"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, body.UserName != "" && body.UserEmail != "", body.Detected)
}

func TestHTTPListProfilesReturnsOnlyTheExecutorsProfiles(t *testing.T) {
	repo := executorFixture()
	h := newProfileHandlers(t, repo, nil)

	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/executors/ex-docker/profiles", "")
	c.Params = gin.Params{{Key: "id", Value: "ex-docker"}}
	h.httpListProfiles(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeProfileList(t, rec.Body.Bytes())
	require.Equal(t, 1, resp.Total)
	require.Equal(t, "pr-1", resp.Profiles[0].ID)

	otherCtx, otherRec := newWorkflowRequest(t, http.MethodGet, "/api/v1/executors/ex-local/profiles", "")
	otherCtx.Params = gin.Params{{Key: "id", Value: "ex-local"}}
	h.httpListProfiles(otherCtx)
	require.Equal(t, http.StatusOK, otherRec.Code)
	other := decodeProfileList(t, otherRec.Body.Bytes())
	require.Equal(t, 0, other.Total)
	require.NotNil(t, other.Profiles, "an empty list must serialize as [] rather than null")
}

func TestHTTPListProfilesSurfacesRepositoryFailure(t *testing.T) {
	repo := executorFixture()
	repo.listProfilesErr = errors.New("database unavailable")
	h := newProfileHandlers(t, repo, nil)
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/executors/ex-docker/profiles", "")
	c.Params = gin.Params{{Key: "id", Value: "ex-docker"}}

	h.httpListProfiles(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"Failed to list profiles"}`, rec.Body.String())
}

func TestHTTPCreateProfileRejectsInvalidRequests(t *testing.T) {
	repo := executorFixture()
	h := newProfileHandlers(t, repo, nil)

	for name, tc := range map[string]struct {
		body     string
		wantBody string
	}{
		"malformed json": {body: `{`, wantBody: `{"error":"invalid payload"}`},
		"missing name":   {body: `{"mcp_policy":"{}"}`, wantBody: `{"error":"name is required"}`},
	} {
		t.Run(name, func(t *testing.T) {
			c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/executors/ex-docker/profiles", tc.body)
			c.Params = gin.Params{{Key: "id", Value: "ex-docker"}}
			h.httpCreateProfile(c)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.JSONEq(t, tc.wantBody, rec.Body.String())
		})
	}
	require.Empty(t, repo.createdProfile)
}

func TestHTTPCreateProfileTakesExecutorIDFromThePath(t *testing.T) {
	repo := executorFixture()
	h := newProfileHandlers(t, repo, nil)
	c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/executors/ex-local/profiles",
		`{"name":"Fresh","mcp_policy":"{}","prepare_script":"echo hi"}`)
	c.Params = gin.Params{{Key: "id", Value: "ex-local"}}

	h.httpCreateProfile(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, repo.createdProfile, 1)
	require.Equal(t, "ex-local", repo.createdProfile[0].ExecutorID)
	require.Equal(t, "Fresh", repo.createdProfile[0].Name)
	require.Equal(t, "echo hi", repo.createdProfile[0].PrepareScript)

	var got dto.ExecutorProfileDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "Fresh", got.Name)
	require.Equal(t, "ex-local", got.ExecutorID)
}

// TestHTTPCreateProfileFailsForUnknownExecutor covers the service-side
// existence check surfacing through the handler's generic 500.
func TestHTTPCreateProfileFailsForUnknownExecutor(t *testing.T) {
	repo := executorFixture()
	h := newProfileHandlers(t, repo, nil)
	c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/executors/nope/profiles", `{"name":"Fresh"}`)
	c.Params = gin.Params{{Key: "id", Value: "nope"}}

	h.httpCreateProfile(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"profile not created"}`, rec.Body.String())
	require.Empty(t, repo.createdProfile)
}

func TestHTTPGetProfileReturnsProfileOrNotFound(t *testing.T) {
	repo := executorFixture()
	h := newProfileHandlers(t, repo, nil)

	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/executors/ex-docker/profiles/pr-1", "")
	c.Params = gin.Params{{Key: "id", Value: "ex-docker"}, {Key: "profileId", Value: "pr-1"}}
	h.httpGetProfile(c)
	require.Equal(t, http.StatusOK, rec.Code)
	var got dto.ExecutorProfileDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "pr-1", got.ID)

	missingCtx, missingRec := newWorkflowRequest(t, http.MethodGet,
		"/api/v1/executors/ex-docker/profiles/nope", "")
	missingCtx.Params = gin.Params{{Key: "id", Value: "ex-docker"}, {Key: "profileId", Value: "nope"}}
	h.httpGetProfile(missingCtx)
	require.Equal(t, http.StatusNotFound, missingRec.Code)
	require.JSONEq(t, `{"error":"profile not found"}`, missingRec.Body.String())
}

func TestHTTPUpdateProfileAppliesProvidedFieldsOnly(t *testing.T) {
	repo := executorFixture()
	repo.profileByID["pr-1"].PrepareScript = "original"
	h := newProfileHandlers(t, repo, nil)
	c, rec := newWorkflowRequest(t, http.MethodPatch, "/api/v1/executors/ex-docker/profiles/pr-1",
		`{"name":"Renamed"}`)
	c.Params = gin.Params{{Key: "id", Value: "ex-docker"}, {Key: "profileId", Value: "pr-1"}}

	h.httpUpdateProfile(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, repo.updatedProfile, 1)
	require.Equal(t, "Renamed", repo.updatedProfile[0].Name)
	require.Equal(t, "original", repo.updatedProfile[0].PrepareScript, "an omitted script must be preserved")
}

func TestHTTPUpdateProfileRejectsInvalidPayload(t *testing.T) {
	repo := executorFixture()
	h := newProfileHandlers(t, repo, nil)
	c, rec := newWorkflowRequest(t, http.MethodPatch, "/api/v1/executors/ex-docker/profiles/pr-1", `{`)
	c.Params = gin.Params{{Key: "id", Value: "ex-docker"}, {Key: "profileId", Value: "pr-1"}}

	h.httpUpdateProfile(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"invalid payload"}`, rec.Body.String())
	require.Empty(t, repo.updatedProfile)
}

func TestHTTPUpdateProfileSurfacesRepositoryFailure(t *testing.T) {
	repo := executorFixture()
	repo.updateErr = errors.New("disk full")
	h := newProfileHandlers(t, repo, nil)
	c, rec := newWorkflowRequest(t, http.MethodPatch, "/api/v1/executors/ex-docker/profiles/pr-1",
		`{"name":"Renamed"}`)
	c.Params = gin.Params{{Key: "id", Value: "ex-docker"}, {Key: "profileId", Value: "pr-1"}}

	h.httpUpdateProfile(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"profile not updated"}`, rec.Body.String())
}

// TestHTTPDeleteProfileWorksFromEitherRoute covers both registrations: the
// nested one under an executor and the top-level profile-only one. Both read
// the same :profileId param, so both must delete the same row.
func TestHTTPDeleteProfileWorksFromEitherRoute(t *testing.T) {
	for name, params := range map[string]gin.Params{
		"nested":    {{Key: "id", Value: "ex-docker"}, {Key: "profileId", Value: "pr-1"}},
		"top level": {{Key: "profileId", Value: "pr-1"}},
	} {
		t.Run(name, func(t *testing.T) {
			repo := executorFixture()
			h := newProfileHandlers(t, repo, nil)
			c, rec := newWorkflowRequest(t, http.MethodDelete, "/api/v1/executor-profiles/pr-1", "")
			c.Params = params

			h.httpDeleteProfile(c)

			require.Equal(t, http.StatusOK, rec.Code)
			require.JSONEq(t, `{"success":true}`, rec.Body.String())
			require.Equal(t, []string{"pr-1"}, repo.deletedProfile)
		})
	}
}

func TestHTTPDeleteProfileReturnsBadRequestForUnknownProfile(t *testing.T) {
	repo := executorFixture()
	h := newProfileHandlers(t, repo, nil)
	c, rec := newWorkflowRequest(t, http.MethodDelete, "/api/v1/executor-profiles/nope", "")
	c.Params = gin.Params{{Key: "profileId", Value: "nope"}}

	h.httpDeleteProfile(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"profile not deleted"}`, rec.Body.String())
	require.Empty(t, repo.deletedProfile)
}

func TestWSListProfilesRequiresExecutorID(t *testing.T) {
	h := newProfileHandlers(t, executorFixture(), nil)

	resp, err := h.wsListProfiles(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileList, map[string]any{}))
	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeValidation), payload.Code)
	require.Equal(t, "executor_id is required", payload.Message)

	malformed, err := h.wsListProfiles(context.Background(), wsMalformedRequest(ws.ActionExecutorProfileList))
	require.NoError(t, err)
	require.Equal(t, string(ws.ErrorCodeBadRequest), wsWorkflowError(t, malformed).Code)
}

func TestWSListProfilesReturnsTheExecutorsProfiles(t *testing.T) {
	h := newProfileHandlers(t, executorFixture(), nil)

	resp, err := h.wsListProfiles(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileList, map[string]any{"executor_id": "ex-docker"}))

	require.NoError(t, err)
	var list dto.ListExecutorProfilesResponse
	wsWorkflowResponse(t, resp, &list)
	require.Equal(t, 1, list.Total)
	require.Equal(t, "pr-1", list.Profiles[0].ID)
}

func TestWSListProfilesSurfacesRepositoryFailure(t *testing.T) {
	repo := executorFixture()
	repo.listProfilesErr = errors.New("database unavailable")
	h := newProfileHandlers(t, repo, nil)

	resp, err := h.wsListProfiles(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileList, map[string]any{"executor_id": "ex-docker"}))

	require.NoError(t, err)
	require.Equal(t, "Failed to list profiles", wsWorkflowError(t, resp).Message)
}

func TestWSListAllProfilesEnrichesWithExecutorIdentity(t *testing.T) {
	h := newProfileHandlers(t, executorFixture(), nil)

	resp, err := h.wsListAllProfiles(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileListAll, map[string]any{}))

	require.NoError(t, err)
	var list dto.ListExecutorProfilesResponse
	wsWorkflowResponse(t, resp, &list)
	require.Equal(t, 1, list.Total)
	require.Equal(t, "Docker", list.Profiles[0].ExecutorName)
}

func TestWSListAllProfilesSurfacesFailures(t *testing.T) {
	profilesFail := executorFixture()
	profilesFail.listProfilesErr = errors.New("database unavailable")
	resp, err := newProfileHandlers(t, profilesFail, nil).wsListAllProfiles(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileListAll, map[string]any{}))
	require.NoError(t, err)
	require.Equal(t, "Failed to list profiles", wsWorkflowError(t, resp).Message)

	executorsFail := executorFixture()
	executorsFail.listErr = errors.New("database unavailable")
	resp, err = newProfileHandlers(t, executorsFail, nil).wsListAllProfiles(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileListAll, map[string]any{}))
	require.NoError(t, err)
	require.Equal(t, "Failed to list executors", wsWorkflowError(t, resp).Message)
}

func TestWSCreateProfileValidatesAndCreates(t *testing.T) {
	repo := executorFixture()
	h := newProfileHandlers(t, repo, nil)

	invalid, err := h.wsCreateProfile(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileCreate, map[string]any{"name": "Fresh"}))
	require.NoError(t, err)
	require.Equal(t, "executor_id and name are required", wsWorkflowError(t, invalid).Message)
	require.Empty(t, repo.createdProfile)

	resp, err := h.wsCreateProfile(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileCreate, map[string]any{
			"executor_id": "ex-local", "name": "Fresh", "cleanup_script": "echo bye",
		}))
	require.NoError(t, err)
	var created dto.ExecutorProfileDTO
	wsWorkflowResponse(t, resp, &created)
	require.Equal(t, "Fresh", created.Name)
	require.Equal(t, "ex-local", created.ExecutorID)
	require.Len(t, repo.createdProfile, 1)
	require.Equal(t, "echo bye", repo.createdProfile[0].CleanupScript)
}

func TestWSCreateProfileSurfacesUnknownExecutor(t *testing.T) {
	repo := executorFixture()
	h := newProfileHandlers(t, repo, nil)

	resp, err := h.wsCreateProfile(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileCreate, map[string]any{
			"executor_id": "nope", "name": "Fresh",
		}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeInternalError), payload.Code)
	require.Equal(t, "Failed to create profile", payload.Message)
	require.Empty(t, repo.createdProfile)
}

func TestWSGetProfileReturnsProfileOrErrors(t *testing.T) {
	h := newProfileHandlers(t, executorFixture(), nil)

	resp, err := h.wsGetProfile(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileGet, map[string]any{"id": "pr-1"}))
	require.NoError(t, err)
	var got dto.ExecutorProfileDTO
	wsWorkflowResponse(t, resp, &got)
	require.Equal(t, "pr-1", got.ID)

	missing, err := h.wsGetProfile(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileGet, map[string]any{"id": "nope"}))
	require.NoError(t, err)
	require.Equal(t, string(ws.ErrorCodeNotFound), wsWorkflowError(t, missing).Code)

	empty, err := h.wsGetProfile(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileGet, map[string]any{}))
	require.NoError(t, err)
	require.Equal(t, "id is required", wsWorkflowError(t, empty).Message)
}

func TestWSUpdateProfileAppliesFields(t *testing.T) {
	repo := executorFixture()
	h := newProfileHandlers(t, repo, nil)

	resp, err := h.wsUpdateProfile(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileUpdate, map[string]any{
			"id": "pr-1", "name": "Renamed", "prepare_script": "echo hi",
		}))

	require.NoError(t, err)
	var updated dto.ExecutorProfileDTO
	wsWorkflowResponse(t, resp, &updated)
	require.Equal(t, "Renamed", updated.Name)
	require.Equal(t, "echo hi", updated.PrepareScript)
	require.Len(t, repo.updatedProfile, 1)
}

func TestWSUpdateProfileRequiresID(t *testing.T) {
	repo := executorFixture()
	h := newProfileHandlers(t, repo, nil)

	resp, err := h.wsUpdateProfile(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileUpdate, map[string]any{"name": "Renamed"}))

	require.NoError(t, err)
	require.Equal(t, "id is required", wsWorkflowError(t, resp).Message)
	require.Empty(t, repo.updatedProfile)
}

func TestWSDeleteProfileDeletesAndValidates(t *testing.T) {
	repo := executorFixture()
	h := newProfileHandlers(t, repo, nil)

	empty, err := h.wsDeleteProfile(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileDelete, map[string]any{}))
	require.NoError(t, err)
	require.Equal(t, "id is required", wsWorkflowError(t, empty).Message)
	require.Empty(t, repo.deletedProfile)

	resp, err := h.wsDeleteProfile(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileDelete, map[string]any{"id": "pr-1"}))
	require.NoError(t, err)
	var success dto.SuccessResponse
	wsWorkflowResponse(t, resp, &success)
	require.True(t, success.Success)
	require.Equal(t, []string{"pr-1"}, repo.deletedProfile)
}

func TestWSDeleteProfileSurfacesRepositoryFailure(t *testing.T) {
	repo := executorFixture()
	h := newProfileHandlers(t, repo, nil)

	resp, err := h.wsDeleteProfile(context.Background(),
		wsWorkflowRequest(t, ws.ActionExecutorProfileDelete, map[string]any{"id": "nope"}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeInternalError), payload.Code)
	require.Equal(t, "Failed to delete profile", payload.Message)
	require.Empty(t, repo.deletedProfile)
}
