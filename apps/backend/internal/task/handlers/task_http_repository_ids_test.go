package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type taskRepositoryIDsResponse struct {
	ID string `json:"id"`
}

type taskProjectRepositorySourceReaderFunc func(context.Context, string) (service.ProjectRepositorySources, error)

func (f taskProjectRepositorySourceReaderFunc) ReadProjectRepositorySources(
	ctx context.Context,
	projectID string,
) (service.ProjectRepositorySources, error) {
	return f(ctx, projectID)
}

func registeredTaskCreateRouter(t *testing.T) (*gin.Engine, *sqliterepo.Repository, *[]*bus.Event) {
	t.Helper()
	router, _, repo, recorded := registeredReplacementFixture(t)
	return router, repo, recorded
}

func taskCreateRequest(title string) map[string]any {
	return map[string]any{
		"workspace_id":     "replacement-ws",
		"workflow_id":      "replacement-wf",
		"workflow_step_id": "replacement-step",
		"title":            title,
	}
}

func postTaskCreateJSON(t *testing.T, router *gin.Engine, payload string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func taskCreateCount(t *testing.T, repo *sqliterepo.Repository, title string) int {
	t.Helper()
	var count int
	require.NoError(t, repo.DB().QueryRow(`SELECT COUNT(*) FROM tasks WHERE title = ?`, title).Scan(&count))
	return count
}

func taskIDFromCreateResponse(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var response taskRepositoryIDsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotEmpty(t, response.ID)
	return response.ID
}

func encodeTaskCreateRequest(t *testing.T, request map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(request)
	require.NoError(t, err)
	return string(encoded)
}

func requireTaskRepositoryIDs(t *testing.T, repo *sqliterepo.Repository, taskID string, want ...string) {
	t.Helper()
	rows, err := repo.ListTaskRepositories(context.Background(), taskID)
	require.NoError(t, err)
	require.Len(t, rows, len(want))
	for index, repository := range rows {
		require.Equal(t, want[index], repository.RepositoryID)
	}
}

// @covers AC-TASKS-HTTP-REPOSITORY-SELECTION-001.1
func TestHTTPCreateTaskRepositoryIDs(t *testing.T) {
	router, repo, recorded := registeredTaskCreateRouter(t)
	for _, selection := range []struct {
		title   string
		field   string
		entries any
	}{
		{title: "ID field", field: "repository_ids", entries: []string{"next-b", "next-a"}},
		{title: "canonical field", field: "repositories", entries: []map[string]string{
			{"repository_id": "next-b"}, {"repository_id": "next-a"},
		}},
	} {
		t.Run(selection.title, func(t *testing.T) {
			request := taskCreateRequest("selected " + selection.title)
			request[selection.field] = selection.entries
			created := postTaskCreateJSON(t, router, encodeTaskCreateRequest(t, request))
			require.Equal(t, http.StatusOK, created.Code, created.Body.String())
			requireTaskRepositoryIDs(t, repo, taskIDFromCreateResponse(t, created), "next-b", "next-a")
			require.Equal(t, 1, taskCreateCount(t, repo, "selected "+selection.title))
		})
	}
	require.Len(t, *recorded, 2)
	for _, event := range *recorded {
		require.Equal(t, "task.created", event.Type)
	}
}

func rejectTaskCreateWithoutEffects(
	t *testing.T,
	router *gin.Engine,
	repo *sqliterepo.Repository,
	recorded *[]*bus.Event,
	title string,
	payload string,
) *httptest.ResponseRecorder {
	t.Helper()
	beforeCount := taskCreateCount(t, repo, title)
	beforeEvents := len(*recorded)
	rejected := postTaskCreateJSON(t, router, payload)
	require.GreaterOrEqual(t, rejected.Code, http.StatusBadRequest, rejected.Body.String())
	require.Equal(t, beforeCount, taskCreateCount(t, repo, title))
	require.Len(t, *recorded, beforeEvents)
	return rejected
}

// @covers AC-TASKS-HTTP-REPOSITORY-SELECTION-001.3, AC-TASKS-HTTP-REPOSITORY-SELECTION-001.4
func TestHTTPCreateTaskRepositoryIDsRejection(t *testing.T) {
	router, repo, recorded := registeredTaskCreateRouter(t)
	control := taskCreateRequest("repository rejection control")
	control["repositories"] = []map[string]string{{"repository_id": "next-a"}}
	created := postTaskCreateJSON(t, router, encodeTaskCreateRequest(t, control))
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	requireTaskRepositoryIDs(t, repo, taskIDFromCreateResponse(t, created), "next-a")
	require.Len(t, *recorded, 1)
	require.Equal(t, "task.created", (*recorded)[0].Type)

	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "replacement-other-ws", Name: "Other"}))
	require.NoError(t, repo.CreateRepository(context.Background(), &models.Repository{
		ID: "foreign-repo", WorkspaceID: "replacement-other-ws", Name: "Foreign", DefaultBranch: "main",
	}))

	conflicts := []struct {
		name   string
		fields map[string]any
	}{
		{name: "two nonempty arrays", fields: map[string]any{
			"repositories": []map[string]string{{"repository_id": "next-a"}}, "repository_ids": []string{"next-a"},
		}},
		{name: "empty repositories", fields: map[string]any{
			"repositories": []map[string]string{}, "repository_ids": []string{"next-a"},
		}},
		{name: "empty repository IDs", fields: map[string]any{
			"repositories": []map[string]string{{"repository_id": "next-a"}}, "repository_ids": []string{},
		}},
		{name: "both arrays empty", fields: map[string]any{
			"repositories": []map[string]string{}, "repository_ids": []string{},
		}},
	}
	for _, conflict := range conflicts {
		t.Run(conflict.name, func(t *testing.T) {
			request := taskCreateRequest("rejected " + conflict.name)
			for key, value := range conflict.fields {
				request[key] = value
			}
			response := rejectTaskCreateWithoutEffects(t, router, repo, recorded, "rejected "+conflict.name, encodeTaskCreateRequest(t, request))
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"creation_rejected":true`)
			require.Contains(t, response.Body.String(), "repositories and repository_ids")
		})
	}

	invalidRequests := []struct {
		name  string
		field string
		value any
	}{
		{name: "non-array value", field: "repository_ids", value: "next-a"},
		{name: "empty ID", field: "repository_ids", value: []string{""}},
	}
	for _, invalid := range invalidRequests {
		t.Run(invalid.name, func(t *testing.T) {
			title := "rejected " + invalid.name
			request := taskCreateRequest(title)
			request[invalid.field] = invalid.value
			response := rejectTaskCreateWithoutEffects(t, router, repo, recorded, title, encodeTaskCreateRequest(t, request))
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"creation_rejected":true`)
		})
	}

	aliasRejected, canonicalRejected := rejectForeignRepositorySelections(t, router, repo, recorded)
	require.Equal(t, canonicalRejected.Code, aliasRejected.Code)
	var aliasError, canonicalError map[string]any
	require.NoError(t, json.Unmarshal(aliasRejected.Body.Bytes(), &aliasError))
	require.NoError(t, json.Unmarshal(canonicalRejected.Body.Bytes(), &canonicalError))
	require.Equal(t, canonicalError, aliasError)
}

func rejectForeignRepositorySelections(
	t *testing.T,
	router *gin.Engine,
	repo *sqliterepo.Repository,
	recorded *[]*bus.Event,
) (*httptest.ResponseRecorder, *httptest.ResponseRecorder) {
	t.Helper()
	aliasRequest := taskCreateRequest("foreign repository alias")
	aliasRequest["repository_ids"] = []string{"next-a", "foreign-repo"}
	aliasRejected := rejectTaskCreateWithoutEffects(t, router, repo, recorded, "foreign repository alias", encodeTaskCreateRequest(t, aliasRequest))
	canonicalRequest := taskCreateRequest("foreign repository canonical")
	canonicalRequest["repositories"] = []map[string]string{{"repository_id": "next-a"}, {"repository_id": "foreign-repo"}}
	canonicalRejected := rejectTaskCreateWithoutEffects(t, router, repo, recorded, "foreign repository canonical", encodeTaskCreateRequest(t, canonicalRequest))
	return aliasRejected, canonicalRejected
}

// @covers AC-TASKS-HTTP-REPOSITORY-SELECTION-001.2
func TestHTTPCreateTaskRepositoryIDsPresence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlers, repo, _, _ := newCreationAttachmentHandler(t)
	projectPath := initHandlerGitRepository(t, filepath.Join(t.TempDir(), "project-source"))
	explicitPath := initHandlerGitRepository(t, filepath.Join(t.TempDir(), "explicit-source"))
	require.NoError(t, repo.CreateRepository(context.Background(), &models.Repository{
		ID: "repo-explicit", WorkspaceID: "ws-http-creation", Name: "explicit",
		SourceType: "local", LocalPath: explicitPath, DefaultBranch: "main",
	}))
	handlers.service.SetProjectRepositorySourceReader(taskProjectRepositorySourceReaderFunc(func(context.Context, string) (service.ProjectRepositorySources, error) {
		return service.ProjectRepositorySources{WorkspaceID: "ws-http-creation", Sources: []string{projectPath}}, nil
	}))
	router := gin.New()
	RegisterTaskRoutes(router, ws.NewDispatcher(), handlers.service, nil, repo, nil, handlers.logger)

	cases := []struct {
		name       string
		fields     map[string]any
		wantRepos  []string
		wantSource bool
	}{
		{name: "omitted", wantRepos: []string{"project source"}, wantSource: true},
		{name: "repository IDs null", fields: map[string]any{"repository_ids": nil}, wantRepos: []string{"project source"}, wantSource: true},
		{name: "repositories null", fields: map[string]any{"repositories": nil}, wantRepos: []string{"project source"}, wantSource: true},
		{name: "empty ID list", fields: map[string]any{"repository_ids": []string{}}},
		{name: "null canonical plus alias array", fields: map[string]any{
			"repositories": nil, "repository_ids": []string{"repo-explicit"},
		}, wantRepos: []string{"repo-explicit"}},
		{name: "null alias plus canonical array", fields: map[string]any{
			"repository_ids": nil, "repositories": []map[string]string{{"repository_id": "repo-explicit"}},
		}, wantRepos: []string{"repo-explicit"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			title := "presence " + testCase.name
			request := map[string]any{
				"workspace_id": "ws-http-creation", "workflow_id": "wf-http-creation",
				"title": title, "project_id": "project-defaults",
			}
			for key, value := range testCase.fields {
				request[key] = value
			}
			created := postTaskCreateJSON(t, router, encodeTaskCreateRequest(t, request))
			require.Equal(t, http.StatusOK, created.Code, created.Body.String())
			id := taskIDFromCreateResponse(t, created)
			rows, err := repo.ListTaskRepositories(context.Background(), id)
			require.NoError(t, err)
			if testCase.wantSource {
				require.Len(t, rows, 1)
				repository, getErr := repo.GetRepository(context.Background(), rows[0].RepositoryID)
				require.NoError(t, getErr)
				require.Equal(t, projectPath, repository.LocalPath)
				return
			}
			require.Len(t, rows, len(testCase.wantRepos))
			for index, repository := range rows {
				require.Equal(t, testCase.wantRepos[index], repository.RepositoryID)
			}
		})
	}
}
