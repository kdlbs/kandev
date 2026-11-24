package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/httpmw"
	"github.com/kandev/kandev/internal/common/config"
	officeagents "github.com/kandev/kandev/internal/office/agents"
	officeprojects "github.com/kandev/kandev/internal/office/projects"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeruntime "github.com/kandev/kandev/internal/office/runtime"
	"github.com/kandev/kandev/internal/task/service"
	userstore "github.com/kandev/kandev/internal/user/store"
)

func TestOfficePATAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	authSvc := newEnabledAuthService(t, cfg)
	taskSvc, _, officeRepo := newRunSubscriptionCheckHarness(t)

	ownedWorkspaceID := seedOwnedWorkspace(t, taskSvc, userstore.DefaultUserID)
	foreignWorkspaceID := seedOwnedWorkspace(t, taskSvc, "office-pat-foreign-user")
	projectSvc := officeprojects.NewProjectService(officeRepo, testLogger(t), nil)
	if err := projectSvc.CreateProject(context.Background(), &officeprojects.Project{
		ID: "office-pat-project", Name: "PAT project", WorkspaceID: ownedWorkspaceID,
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	_, pat, err := authSvc.MintToken(context.Background(), userstore.DefaultUserID, "office-pat", 0)
	if err != nil {
		t.Fatalf("mint PAT: %v", err)
	}
	_, session, err := authSvc.Login(context.Background(), "admin@x.dev", "adminpass123", "test", "127.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	router := mountOfficePATRoutes(t, authSvc, taskSvc, officeRepo)
	ownedProjectsPath := "/api/v1/office/workspaces/" + ownedWorkspaceID + "/projects"

	t.Run("owning_PAT", func(t *testing.T) {
		rec := officePATGet(t, router, authSvc, ownedProjectsPath, pat, "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"office-pat-project"`) {
			t.Fatalf("owning PAT response = %d %s, want project response", rec.Code, rec.Body.String())
		}
	})

	t.Run("PAT_whitespace", func(t *testing.T) {
		rec := officePATGet(t, router, authSvc, ownedProjectsPath, "  "+pat+"  ", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"office-pat-project"`) {
			t.Fatalf("whitespace PAT response = %d %s, want project response", rec.Code, rec.Body.String())
		}
	})

	t.Run("foreign_workspace", func(t *testing.T) {
		path := "/api/v1/office/workspaces/" + foreignWorkspaceID + "/projects"
		rec := officePATGet(t, router, authSvc, path, pat, "")
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "workspace not found") {
			t.Fatalf("foreign workspace response = %d %s, want 404 workspace not found", rec.Code, rec.Body.String())
		}
	})

	t.Run("unrecognized_PAT", func(t *testing.T) {
		rec := officePATGet(t, router, authSvc, "/api/v1/office/meta", auth.PATPrefix+"unregistered", "")
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"error":"invalid token"`) {
			t.Fatalf("unrecognized PAT response = %d %s, want 401 invalid token", rec.Code, rec.Body.String())
		}
	})

	t.Run("cookie_precedence", func(t *testing.T) {
		rec := officePATGet(t, router, authSvc, ownedProjectsPath, auth.PATPrefix+"unregistered", session)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"office-pat-project"`) {
			t.Fatalf("session and unrecognized PAT response = %d %s, want project response", rec.Code, rec.Body.String())
		}
	})

	t.Run("runtime_requires_agent_JWT", func(t *testing.T) {
		rec := officePATGet(t, router, authSvc, "/api/v1/office/runtime/projects", pat, "")
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"error":"invalid runtime token"`) {
			t.Fatalf("runtime PAT response = %d %s, want 401 invalid runtime token", rec.Code, rec.Body.String())
		}
	})
}

func TestOfficePATAuthenticationDisabledMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authSvc := newDisabledAuthService(t)
	taskSvc, _, officeRepo := newRunSubscriptionCheckHarness(t)
	router := mountOfficePATRoutes(t, authSvc, taskSvc, officeRepo)

	rec := officePATGet(t, router, authSvc, "/api/v1/office/meta", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("credentialless metadata response = %d %s, want 200", rec.Code, rec.Body.String())
	}

	rec = officePATGet(t, router, authSvc, "/api/v1/office/meta", auth.PATPrefix+"unregistered", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("disabled-mode PAT response = %d %s, want 200 like a request without a bearer", rec.Code, rec.Body.String())
	}
}

func mountOfficePATRoutes(
	t *testing.T,
	authSvc *auth.Service,
	taskSvc *service.Service,
	officeRepo *officesqlite.Repository,
) *gin.Engine {
	t.Helper()
	agentSvc := officeagents.NewAgentService(officeRepo, testLogger(t), nil)
	agentSvc.SetAuth(officeagents.NewAgentAuth("office-pat-test-signing-key"))
	svcs := officeTestServices()
	svcs.Agents = agentSvc
	svcs.Projects = officeprojects.NewProjectService(officeRepo, testLogger(t), nil)
	router := gin.New()
	router.Use(httpmw.Middleware(authSvc))
	mountOfficeRoutes(router, svcs, authSvc, taskSvc, officeRepo, nil, officeruntime.HandoffDependencies{}, testLogger(t))
	return router
}

func officePATGet(
	t *testing.T,
	router *gin.Engine,
	authSvc *auth.Service,
	path, bearer, session string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if session != "" {
		req.AddCookie(&http.Cookie{Name: authSvc.CookieNameForRequest(req), Value: session})
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
