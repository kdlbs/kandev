package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
)

// quickChatAuthzRepo owns two workspaces belonging to different users and
// serves user-a's quick chat regardless of the workspace asked for, so any tab
// reaching a foreign caller is a leak rather than an artifact of the fixture.
type quickChatAuthzRepo struct {
	quickChatListRepo
	workspaces map[string]*models.Workspace
}

// GetWorkspace mirrors the sqlite repository's error shape exactly: a missing
// row wraps the sentinel with the requested ID (workspace.go:351). Returning the
// bare sentinel here would make the byte-identity assertion below pass for the
// wrong reason -- the point is that handleNotFound substitutes its own fallback
// message, so the ID the caller probed for never reaches the response.
func (r *quickChatAuthzRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	workspace, ok := r.workspaces[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", repoerrors.ErrWorkspaceNotFound, id)
	}
	return workspace, nil
}

func newQuickChatAuthzHandler(t *testing.T) *TaskHandlers {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	repo := &quickChatAuthzRepo{
		quickChatListRepo: quickChatListRepo{
			tasks: []*models.Task{{
				ID: "task-chat", WorkspaceID: "ws-a", Title: "A's secret chat", IsEphemeral: true,
				Metadata: map[string]interface{}{models.MetaKeyAgentProfileID: "agent-1"},
			}},
			sessions: map[string]*models.TaskSession{
				"task-chat": {ID: "session-chat", TaskID: "task-chat"},
			},
		},
		workspaces: map[string]*models.Workspace{
			"ws-a": {ID: "ws-a", Name: "A's", OwnerID: "user-a"},
			"ws-b": {ID: "ws-b", Name: "B's", OwnerID: "user-b"},
		},
	}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return &TaskHandlers{service: svc, logger: log}
}

func listQuickChats(t *testing.T, h *TaskHandlers, identity *authn.Identity, workspaceID string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/workspaces/"+workspaceID+"/quick-chats", nil)
	if identity != nil {
		req = req.WithContext(authn.WithIdentity(req.Context(), *identity))
	}
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: workspaceID}}
	h.httpListQuickChatSessions(c)
	return rec
}

// TestHTTPListQuickChatSessionsDeniesForeignWorkspace verifies end to end that
// the service denial reaches the wire as a plain 404 -- the whole
// no-existence-leak guarantee rests on the response, not on the sentinel.
func TestHTTPListQuickChatSessionsDeniesForeignWorkspace(t *testing.T) {
	h := newQuickChatAuthzHandler(t)
	userB := &authn.Identity{UserID: "user-b", Role: authn.RoleMember}

	foreign := listQuickChats(t, h, userB, "ws-a")
	missing := listQuickChats(t, h, userB, "ws-nonexistent")

	assert.Equal(t, http.StatusNotFound, foreign.Code)
	assert.NotContains(t, foreign.Body.String(), "A's secret chat")
	assert.NotContains(t, foreign.Body.String(), "session-chat")

	// The probed ID must not survive into either body; that is what lets the
	// two cases collapse into one response.
	assert.Equal(t, http.StatusNotFound, missing.Code)
	assert.NotContains(t, missing.Body.String(), "ws-nonexistent")

	// A workspace the caller may not see must be indistinguishable from one
	// that does not exist, byte for byte.
	assert.Equal(t, missing.Body.String(), foreign.Body.String())
}

// TestHTTPListQuickChatSessionsOwnerAndUnscopedCallers keeps the legitimate
// paths intact: the owner still gets their tabs, and with authentication
// disabled (synthetic identity, or no identity at all) nothing changes.
func TestHTTPListQuickChatSessionsOwnerAndUnscopedCallers(t *testing.T) {
	h := newQuickChatAuthzHandler(t)

	for name, identity := range map[string]*authn.Identity{
		"owner":         {UserID: "user-a", Role: authn.RoleMember},
		"auth disabled": {UserID: "default-user", Role: authn.RoleAdmin, Synthetic: true},
		"no identity":   nil,
	} {
		rec := listQuickChats(t, h, identity, "ws-a")
		require.Equal(t, http.StatusOK, rec.Code, "%s: body %s", name, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "A's secret chat", "%s", name)
	}
}
