package promptsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const checkPath = "/api/v1/prompt-sync/check"

func TestHandlerCheckReportsConflicts(t *testing.T) {
	f := newSyncFixture(t)
	router := newTestRouter(t, f.svc, admin)
	_, err := f.prompts.CreatePrompt(context.Background(), "principles", "Mine")
	require.NoError(t, err)
	f.gitlab.files["principles.md"] = "Repo"

	for _, body := range []string{`{"provider":"gitlab","project_path":"acme/eng","workspace_id":"ws-1"}`, ""} {
		response := doRequest(router, http.MethodPost, checkPath, body)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var result CheckResult
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		require.Len(t, result.Conflicts, 1)
		assert.Equal(t, "principles", result.Conflicts[0].Name)
		assert.Equal(t, "Repo", result.Conflicts[0].RepositoryContent)
	}
}

func TestHandlerCheckFailureIsUnprocessable(t *testing.T) {
	f := newSyncFixture(t)
	router := newTestRouter(t, f.svc, admin)
	f.gitlab.listErr = errors.New("boom")
	response := doRequest(router, http.MethodPost, checkPath, `{"provider":"gitlab","project_path":"acme/eng","workspace_id":"ws-1"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
	assert.Contains(t, response.Body.String(), `"error"`)
}

func TestHandlerSaveSyncsWithResolutions(t *testing.T) {
	f := newSyncFixture(t)
	router := newTestRouter(t, f.svc, admin)
	_, err := f.prompts.CreatePrompt(context.Background(), "principles", "Mine")
	require.NoError(t, err)
	f.gitlab.files["principles.md"] = "Repo"

	response := doRequest(router, http.MethodPost, "/api/v1/prompt-sync/config",
		`{"provider":"gitlab","project_path":"acme/eng","workspace_id":"ws-1","resolutions":{"principles":"repository"}}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body struct {
		Config *Config     `json:"config"`
		Result *SyncResult `json:"result"`
		Error  string      `json:"error"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.NotNil(t, body.Config)
	assert.Equal(t, "acme/eng", body.Config.ProjectPath)
	require.NotNil(t, body.Result)
	assert.Equal(t, []string{"principles"}, body.Result.Updated)
	assert.Equal(t, "Repo", f.prompt(t, "principles").Content)
}

func TestHandlerSyncAcceptsResolutions(t *testing.T) {
	f := newSyncFixture(t)
	router := newTestRouter(t, f.svc, admin)
	_, err := f.prompts.CreatePrompt(context.Background(), "principles", "Mine")
	require.NoError(t, err)
	f.gitlab.files["principles.md"] = "Repo"
	_, err = f.svc.Sync(context.Background())
	require.NoError(t, err)

	invalid := doRequest(router, http.MethodPost, "/api/v1/prompt-sync/sync", `{"resolutions":{"principles":"both"}}`)
	assert.Equal(t, http.StatusBadRequest, invalid.Code)
	assert.True(t, f.prompt(t, "principles").Conflict)

	response := doRequest(router, http.MethodPost, "/api/v1/prompt-sync/sync", `{"resolutions":{"principles":"local"}}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.False(t, f.prompt(t, "principles").Conflict)
}
