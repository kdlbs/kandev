package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/dto"
)

func TestHTTPRepositorySetCarriesMemberBaseBranch(t *testing.T) {
	router, _ := newRepositorySetTestRouter(t)

	created := createSetViaHTTP(t, router, map[string]any{
		"name": "Base branches",
		"repositories": []map[string]any{
			{"repository_id": "repo-web", "base_branch": "develop"},
		},
	})

	require.Len(t, created.Repositories, 1)
	require.Equal(t, "repo-web", created.Repositories[0].RepositoryID)
	require.Equal(t, "develop", created.Repositories[0].BaseBranch)

	var fetched dto.RepositorySetDTO
	recorder := doJSON(t, router, http.MethodGet, "/api/v1/repository-sets/"+created.ID, nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &fetched))
	require.Equal(t, "develop", fetched.Repositories[0].BaseBranch)
}

func TestHTTPRepositorySetUpdateCarriesMemberBaseBranch(t *testing.T) {
	router, _ := newRepositorySetTestRouter(t)
	created := createSetViaHTTP(t, router, map[string]any{
		"name":           "Base branches",
		"repository_ids": []string{"repo-web"},
	})

	recorder := doJSON(t, router, http.MethodPatch, "/api/v1/repository-sets/"+created.ID, map[string]any{
		"repositories": []map[string]any{
			{"repository_id": "repo-gateway", "base_branch": "release"},
		},
	})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var updated dto.RepositorySetDTO
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &updated))
	require.Len(t, updated.Repositories, 1)
	require.Equal(t, "repo-gateway", updated.Repositories[0].RepositoryID)
	require.Equal(t, "release", updated.Repositories[0].BaseBranch)
}

func TestHTTPRepositorySetRejectsConflictingMemberInputs(t *testing.T) {
	router, _ := newRepositorySetTestRouter(t)

	recorder := doJSON(t, router, http.MethodPost, "/api/v1/workspaces/ws-1/repository-sets", map[string]any{
		"name":           "Conflict",
		"repository_ids": []string{"repo-web"},
		"repositories":   []map[string]any{{"repository_id": "repo-gateway"}},
	})
	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
}
