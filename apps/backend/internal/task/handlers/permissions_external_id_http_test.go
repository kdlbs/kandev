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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

// ownedIdempotentCreateTaskRepo adds owner-scoped workspace visibility on top
// of idempotentCreateTaskRepo, which otherwise returns (nil, nil) from
// GetWorkspace — and Service.authorizeWorkspaceID treats a nil workspace as
// visible to everyone, so it cannot exercise a real denial. Only "ws-1",
// owned by ownerID, is registered; every other id is not found.
type ownedIdempotentCreateTaskRepo struct {
	*idempotentCreateTaskRepo
	ownerID string
}

func newOwnedIdempotentCreateTaskRepo(ownerID string) *ownedIdempotentCreateTaskRepo {
	return &ownedIdempotentCreateTaskRepo{idempotentCreateTaskRepo: newIdempotentCreateTaskRepo(), ownerID: ownerID}
}

func (r *ownedIdempotentCreateTaskRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	if id != "ws-1" {
		return nil, nil
	}
	return &models.Workspace{ID: "ws-1", OwnerID: r.ownerID}, nil
}

// newOwnedIdempotencyTestHandlers mirrors newIdempotencyTestHandlers but
// wires repo itself (not its embedded idempotentCreateTaskRepo) as the
// Workspaces repository, so Service.authorizeWorkspaceID sees the
// owner-scoped GetWorkspace override rather than the embedded no-op.
func newOwnedIdempotencyTestHandlers(t *testing.T, repo *ownedIdempotentCreateTaskRepo) *TaskHandlers {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	svc.SetWorkflowStepGetter(repo)
	return &TaskHandlers{service: svc, logger: log}
}

// requestWithIdentity mirrors session_scope_http_test.go's requestAs, but
// generalized for a method/path/body since the by-external-id routes take
// query params rather than a JSON body.
func requestWithIdentity(t *testing.T, method, target, body, userID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	c.Request = httptest.NewRequest(method, target, reader)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request = c.Request.WithContext(
		authn.WithIdentity(c.Request.Context(), authn.Identity{UserID: userID, Role: authn.RoleMember}),
	)
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}
	return c, rec
}

// seedOwnedTaskForPermissions seeds a task holding externalID in ws-1
// directly through the repository, settling it when settle is true. It
// deliberately bypasses httpCreateTask: that handler always settles before
// responding (settlement happens unconditionally on the Created outcome, per
// the spec's REST table), so it cannot produce a genuinely unsettled row —
// only a direct repository write can.
func seedOwnedTaskForPermissions(t *testing.T, repo *idempotentCreateTaskRepo, externalID string, settle bool) {
	t.Helper()
	ctx := context.Background()
	task := &models.Task{WorkspaceID: "ws-1", Title: "Owner task", ExternalID: externalID}
	require.NoError(t, repo.CreateTask(ctx, task))
	if !settle {
		return
	}
	ok, err := repo.SettleTaskExternalID(ctx, task.ID, externalID, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, ok)
}

// TestHTTPCreateTaskWithExternalIDDeniesUnauthorizedWorkspace is the HTTP-level
// pin for PE1/PE2: an unauthorized caller retrying a create against another
// user's held external_id — settled or unsettled — gets exactly the
// standard-shaped 404, with no deduplicated/creation_complete field and no
// field of the owner's task, and no task is created for the attacker.
func TestHTTPCreateTaskWithExternalIDDeniesUnauthorizedWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name       string
		externalID string
		settle     bool
	}{
		{"SettledTarget", "ext-settled", true},
		{"UnsettledTarget", "ext-unsettled", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newOwnedIdempotentCreateTaskRepo("user-a")
			h := newOwnedIdempotencyTestHandlers(t, repo)
			seedOwnedTaskForPermissions(t, repo.idempotentCreateTaskRepo, tc.externalID, tc.settle)

			c, rec := requestWithIdentity(t, http.MethodPost, "/tasks",
				`{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Attempt","external_id":"`+tc.externalID+`"}`,
				"user-b")
			h.httpCreateTask(c)

			require.Equal(t, http.StatusNotFound, rec.Code, "body: %s", rec.Body.String())
			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "task not created", resp["error"])
			assert.Len(t, resp, 1, "the 404 body must carry nothing beyond the standard error field")
			_, hasDedup := resp["deduplicated"]
			assert.False(t, hasDedup, "must not reveal deduplicated across the authorization boundary")
			_, hasComplete := resp["creation_complete"]
			assert.False(t, hasComplete, "must not reveal creation_complete across the authorization boundary")

			repo.mu.Lock()
			taskCount := len(repo.tasks)
			repo.mu.Unlock()
			require.Equal(t, 1, taskCount, "the denied attempt must not have created a second task")
		})
	}
}

// TestHTTPLookupAndReleaseExternalIDDenyUnauthorizedWorkspace is the HTTP-level
// pin for PE3: an unauthorized caller GETting or DELETEing the by-external-id
// routes gets 404 {"error": "task not found"}, and a denied release leaves
// the identity untouched.
func TestHTTPLookupAndReleaseExternalIDDenyUnauthorizedWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newOwnedIdempotentCreateTaskRepo("user-a")
	h := newOwnedIdempotencyTestHandlers(t, repo)
	seedOwnedTaskForPermissions(t, repo.idempotentCreateTaskRepo, "ext-1", true)

	lookupC, lookupRec := requestWithIdentity(t, http.MethodGet, "/workspaces/ws-1/tasks/by-external-id?external_id=ext-1", "", "user-b")
	h.httpGetTaskByExternalID(lookupC)
	require.Equal(t, http.StatusNotFound, lookupRec.Code, "lookup body: %s", lookupRec.Body.String())
	var lookupResp map[string]interface{}
	require.NoError(t, json.Unmarshal(lookupRec.Body.Bytes(), &lookupResp))
	assert.Equal(t, "task not found", lookupResp["error"])

	releaseC, releaseRec := requestWithIdentity(t, http.MethodDelete, "/workspaces/ws-1/tasks/by-external-id?external_id=ext-1", "", "user-b")
	h.httpReleaseTaskExternalID(releaseC)
	releaseC.Writer.WriteHeaderNow()
	require.Equal(t, http.StatusNotFound, releaseRec.Code, "release body: %s", releaseRec.Body.String())
	var releaseResp map[string]interface{}
	require.NoError(t, json.Unmarshal(releaseRec.Body.Bytes(), &releaseResp))
	assert.Equal(t, "task not found", releaseResp["error"])

	// The identity must survive the denied release attempt untouched.
	ownerC, ownerRec := requestWithIdentity(t, http.MethodGet, "/workspaces/ws-1/tasks/by-external-id?external_id=ext-1", "", "user-a")
	h.httpGetTaskByExternalID(ownerC)
	require.Equal(t, http.StatusOK, ownerRec.Code, "owner lookup after denied release, body: %s", ownerRec.Body.String())
}

// TestHTTPCreateTaskWithExternalIDAuthorizationPrecedesValidation is the
// HTTP-level pin for PE4: an unauthorized caller supplying an oversized
// (300-byte) external_id gets 404, not 400 — authorization runs before
// external_id validation, so the caller never learns their payload was
// invalid for a workspace they cannot see.
func TestHTTPCreateTaskWithExternalIDAuthorizationPrecedesValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newOwnedIdempotentCreateTaskRepo("user-a")
	h := newOwnedIdempotencyTestHandlers(t, repo)

	oversized := strings.Repeat("x", 300)
	c, rec := requestWithIdentity(t, http.MethodPost, "/tasks",
		`{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Attempt","external_id":"`+oversized+`"}`,
		"user-b")
	h.httpCreateTask(c)

	require.Equal(t, http.StatusNotFound, rec.Code, "body: %s — authorization must precede validation", rec.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "task not created", resp["error"])
}
