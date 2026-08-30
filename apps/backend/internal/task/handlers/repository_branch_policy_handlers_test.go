package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func newRepositoryBranchPolicyTestRouter(t *testing.T, workspaceName string) (*gin.Engine, *ws.Dispatcher, *taskrepo.Repository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "branch-policies.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, cleanup())
		require.NoError(t, sqlxDB.Close())
	})

	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-policy-handler", Name: workspaceName}))
	repositoryPath := initBranchPolicyGitRepository(t)
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-policy-handler", WorkspaceID: "ws-policy-handler", Name: "policy-repo",
		SourceType: "local", LocalPath: repositoryPath, DefaultBranch: "main",
	}))

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, RepoEntities: repo, BranchPolicies: repo,
	}, bus.NewMemoryEventBus(log), log, service.RepositoryDiscoveryConfig{})
	router := gin.New()
	dispatcher := ws.NewDispatcher()
	RegisterRepositoryBranchPolicyRoutes(router, dispatcher, svc, log)
	return router, dispatcher, repo
}

func initBranchPolicyGitRepository(t *testing.T) string {
	t.Helper()
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	require.NoError(t, os.MkdirAll(repositoryPath, 0o755))
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repositoryPath
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
		output, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v failed: %s", args, output)
	}
	runGit("init", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("branch policy test\n"), 0o644))
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Kandev Tests")
	runGit("add", "README.md")
	runGit("commit", "-m", "initial")
	runGit("branch", "develop")
	return repositoryPath
}

func seedHandlerBranchPolicy(t *testing.T, repo *taskrepo.Repository) *models.RepositoryBranchPolicy {
	t.Helper()
	policy := &models.RepositoryBranchPolicy{
		ID: "policy-handler-existing", RepositoryID: "repo-policy-handler", Name: "Existing",
		Description: "Existing policy", BaseBranch: "main", BranchTemplate: "feature/{title}-{suffix}",
		PullRequestTarget: "main",
	}
	require.NoError(t, repo.CreateRepositoryBranchPolicy(context.Background(), policy))
	return policy
}

func dispatchBranchPolicyAction(t *testing.T, dispatcher *ws.Dispatcher, action string, payload any) *ws.Message {
	t.Helper()
	message, err := ws.NewRequest("branch-policy-test", action, payload)
	require.NoError(t, err)
	response, err := dispatcher.Dispatch(context.Background(), message)
	require.NoError(t, err)
	require.NotNil(t, response)
	return response
}

func requireBranchPolicyWSConflict(t *testing.T, response *ws.Message) {
	t.Helper()
	require.Equal(t, ws.MessageTypeError, response.Type)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.Equal(t, ws.ErrorCodeConflict, payload.Code)
	require.Contains(t, payload.Message, "Improve Kandev")
}

func TestRepositoryBranchPolicyHTTPHandlersCoverCRUDAndStatusMappings(t *testing.T) {
	router, _, _ := newRepositoryBranchPolicyTestRouter(t, "Policy workspace")

	list := doJSON(t, router, http.MethodGet, "/api/v1/repositories/repo-policy-handler/branch-policies", nil)
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	var empty dto.ListRepositoryBranchPoliciesResponse
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &empty))
	require.Empty(t, empty.Policies)

	createdResponse := doJSON(t, router, http.MethodPost, "/api/v1/repositories/repo-policy-handler/branch-policies", map[string]any{
		"name": "Feature", "description": "Feature branches", "base_branch": "develop",
		"branch_template": "feature/{title}-{suffix}", "pull_request_target": "develop",
	})
	require.Equal(t, http.StatusCreated, createdResponse.Code, createdResponse.Body.String())
	var created dto.RepositoryBranchPolicyDTO
	require.NoError(t, json.Unmarshal(createdResponse.Body.Bytes(), &created))
	require.Equal(t, "Feature", created.Name)
	require.Equal(t, "develop", created.PullRequestTarget)

	get := doJSON(t, router, http.MethodGet, "/api/v1/repository-branch-policies/"+created.ID, nil)
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())

	updated := doJSON(t, router, http.MethodPatch, "/api/v1/repository-branch-policies/"+created.ID, map[string]any{
		"description": "Updated description", "pull_request_target": "main",
	})
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	var updatedPolicy dto.RepositoryBranchPolicyDTO
	require.NoError(t, json.Unmarshal(updated.Body.Bytes(), &updatedPolicy))
	require.Equal(t, "Updated description", updatedPolicy.Description)
	require.Equal(t, "main", updatedPolicy.PullRequestTarget)

	duplicate := doJSON(t, router, http.MethodPost, "/api/v1/repositories/repo-policy-handler/branch-policies", map[string]any{
		"name": "feature", "base_branch": "main", "branch_template": "feature/{title}-{suffix}",
		"pull_request_target": "main",
	})
	require.Equal(t, http.StatusConflict, duplicate.Code, duplicate.Body.String())

	invalid := doJSON(t, router, http.MethodPost, "/api/v1/repositories/repo-policy-handler/branch-policies", []string{"invalid"})
	require.Equal(t, http.StatusBadRequest, invalid.Code, invalid.Body.String())

	missing := doJSON(t, router, http.MethodGet, "/api/v1/repository-branch-policies/missing", nil)
	require.Equal(t, http.StatusNotFound, missing.Code, missing.Body.String())

	deleted := doJSON(t, router, http.MethodDelete, "/api/v1/repository-branch-policies/"+created.ID, nil)
	require.Equal(t, http.StatusNoContent, deleted.Code, deleted.Body.String())
}

func TestRepositoryBranchPolicyHTTPGitflowMapsConflicts(t *testing.T) {
	router, _, _ := newRepositoryBranchPolicyTestRouter(t, "Policy workspace")
	body := map[string]any{"production_branch": "main", "development_branch": "develop"}

	created := doJSON(t, router, http.MethodPost, "/api/v1/repositories/repo-policy-handler/branch-policies/gitflow", body)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var response dto.ListRepositoryBranchPoliciesResponse
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &response))
	require.Len(t, response.Policies, 4)

	conflict := doJSON(t, router, http.MethodPost, "/api/v1/repositories/repo-policy-handler/branch-policies/gitflow", body)
	require.Equal(t, http.StatusConflict, conflict.Code, conflict.Body.String())
}

func TestRepositoryBranchPolicyWSHandlersCoverCRUDAndGitflow(t *testing.T) {
	_, dispatcher, _ := newRepositoryBranchPolicyTestRouter(t, "Policy workspace")
	createPayload := map[string]any{
		"repository_id": "repo-policy-handler", "name": "Feature", "description": "Feature branches",
		"base_branch": "develop", "branch_template": "feature/{title}-{suffix}", "pull_request_target": "develop",
	}
	created := dispatchBranchPolicyAction(t, dispatcher, ws.ActionRepositoryBranchPolicyCreate, createPayload)
	require.NotEqual(t, ws.MessageTypeError, created.Type, string(created.Payload))
	var policy dto.RepositoryBranchPolicyDTO
	require.NoError(t, json.Unmarshal(created.Payload, &policy))

	list := dispatchBranchPolicyAction(t, dispatcher, ws.ActionRepositoryBranchPolicyList, map[string]any{"repository_id": "repo-policy-handler"})
	require.NotEqual(t, ws.MessageTypeError, list.Type, string(list.Payload))
	get := dispatchBranchPolicyAction(t, dispatcher, ws.ActionRepositoryBranchPolicyGet, map[string]any{"id": policy.ID})
	require.NotEqual(t, ws.MessageTypeError, get.Type, string(get.Payload))
	update := dispatchBranchPolicyAction(t, dispatcher, ws.ActionRepositoryBranchPolicyUpdate, map[string]any{
		"id": policy.ID, "description": "Updated", "pull_request_target": "main",
	})
	require.NotEqual(t, ws.MessageTypeError, update.Type, string(update.Payload))
	deleted := dispatchBranchPolicyAction(t, dispatcher, ws.ActionRepositoryBranchPolicyDelete, map[string]any{"id": policy.ID})
	require.NotEqual(t, ws.MessageTypeError, deleted.Type, string(deleted.Payload))

	gitflow := dispatchBranchPolicyAction(t, dispatcher, ws.ActionRepositoryBranchPolicyGitflow, map[string]any{
		"repository_id": "repo-policy-handler", "production_branch": "main", "development_branch": "develop",
	})
	require.NotEqual(t, ws.MessageTypeError, gitflow.Type, string(gitflow.Payload))
	gitflowConflict := dispatchBranchPolicyAction(t, dispatcher, ws.ActionRepositoryBranchPolicyGitflow, map[string]any{
		"repository_id": "repo-policy-handler", "production_branch": "main", "development_branch": "develop",
	})
	require.Equal(t, ws.MessageTypeError, gitflowConflict.Type)
	var conflictPayload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(gitflowConflict.Payload, &conflictPayload))
	require.Equal(t, ws.ErrorCodeConflict, conflictPayload.Code)
}

func TestRepositoryBranchPolicyHandlersRejectImproveWorkspaceMutations(t *testing.T) {
	router, dispatcher, repo := newRepositoryBranchPolicyTestRouter(t, models.WorkspaceNameImproveKandev)
	policy := seedHandlerBranchPolicy(t, repo)

	create := doJSON(t, router, http.MethodPost, "/api/v1/repositories/repo-policy-handler/branch-policies", map[string]any{
		"name": "Blocked", "base_branch": "main", "branch_template": "feature/{title}-{suffix}",
		"pull_request_target": "main",
	})
	require.Equal(t, http.StatusConflict, create.Code, create.Body.String())

	update := doJSON(t, router, http.MethodPatch, "/api/v1/repository-branch-policies/"+policy.ID, map[string]any{"name": "Renamed"})
	require.Equal(t, http.StatusConflict, update.Code, update.Body.String())
	delete := doJSON(t, router, http.MethodDelete, "/api/v1/repository-branch-policies/"+policy.ID, nil)
	require.Equal(t, http.StatusConflict, delete.Code, delete.Body.String())

	cases := []struct {
		name    string
		action  string
		payload any
	}{
		{"create", ws.ActionRepositoryBranchPolicyCreate, map[string]any{
			"repository_id": "repo-policy-handler", "name": "Blocked", "base_branch": "main",
			"branch_template": "feature/{title}-{suffix}", "pull_request_target": "main",
		}},
		{"update", ws.ActionRepositoryBranchPolicyUpdate, map[string]any{"id": policy.ID, "name": "Renamed"}},
		{"delete", ws.ActionRepositoryBranchPolicyDelete, map[string]any{"id": policy.ID}},
		{"gitflow", ws.ActionRepositoryBranchPolicyGitflow, map[string]any{
			"repository_id": "repo-policy-handler", "production_branch": "main", "development_branch": "develop",
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			requireBranchPolicyWSConflict(t, dispatchBranchPolicyAction(t, dispatcher, testCase.action, testCase.payload))
		})
	}

	list := doJSON(t, router, http.MethodGet, "/api/v1/repositories/repo-policy-handler/branch-policies", nil)
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	var policies dto.ListRepositoryBranchPoliciesResponse
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &policies))
	require.Len(t, policies.Policies, 1)
	require.Equal(t, policy.ID, policies.Policies[0].ID)
}
