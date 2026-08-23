package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/auth"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// newEnabledAuthService builds an auth service in ModeEnabled (features.auth
// on + an admin identity exists) over an in-memory store pair — the state the
// plugin SSO bridge requires.
func newEnabledAuthService(t *testing.T, cfg *config.Config) *auth.Service {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	users, cleanup, err := userstore.Provide(conn, conn)
	if err != nil {
		t.Fatalf("user store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	st, err := authstore.New(conn, conn)
	if err != nil {
		t.Fatalf("auth store: %v", err)
	}
	svc, err := auth.NewService(context.Background(), auth.Deps{Cfg: cfg, Store: st, Users: users})
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	if _, _, err := svc.Setup(context.Background(), "admin@x.dev", "adminpass123", "Admin", "", ""); err != nil {
		t.Fatalf("setup admin: %v", err)
	}
	if svc.Mode() != auth.ModeEnabled {
		t.Fatalf("mode = %s, want enabled", svc.Mode())
	}
	return svc
}

// ssoBridgeCookieNames runs the real pluginSSOBridge LoginExternal on a Gin
// context whose request Host carries the given host, and returns the session
// cookie names set on the response.
func ssoBridgeCookieNames(t *testing.T, svc *auth.Service, host string) []string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/plugins/p/webhooks/cb", nil)
	c.Request.Host = host

	bridge := pluginSSOBridge{auth: svc}
	if err := bridge.LoginExternal(c, "oidc:iss", "sub-1", "alice@x.dev", "Alice"); err != nil {
		t.Fatalf("LoginExternal: %v", err)
	}
	var names []string
	for _, cookie := range rec.Result().Cookies() {
		names = append(names, cookie.Name)
	}
	return names
}

// TestPluginSSOBridgeSetsScopedSessionCookie drives the REAL bridge
// (backendapp/auth.go), not the plugins-package fake: LoginExternal on a Gin
// request Host 127.0.0.1:8443 must set kandev_session_8443. A stale bridge
// call to b.auth.CookieName() would pass resolver tests while emitting the
// plain name for ported SSO.
func TestPluginSSOBridgeSetsScopedSessionCookie(t *testing.T) {
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	svc := newEnabledAuthService(t, cfg)

	names := ssoBridgeCookieNames(t, svc, "127.0.0.1:8443")
	if len(names) != 1 || names[0] != "kandev_session_8443" {
		t.Fatalf("SSO Set-Cookie names = %v, want exactly [kandev_session_8443]", names)
	}
}

// TestPluginSSOBridgeCustomCookieNameVerbatim pins the custom-name contract
// through the real bridge: a configured auth.cookieName is set verbatim on a
// ported Host (never suffixed).
func TestPluginSSOBridgeCustomCookieNameVerbatim(t *testing.T) {
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	cfg.Auth.CookieName = "sso_session"
	svc := newEnabledAuthService(t, cfg)

	names := ssoBridgeCookieNames(t, svc, "127.0.0.1:8443")
	if len(names) != 1 || names[0] != "sso_session" {
		t.Fatalf("SSO Set-Cookie names = %v, want exactly [sso_session]", names)
	}
}

// TestPluginSSOBridgeDefaultPortPlainName pins the no-port branch through the
// real bridge: a default-port Host keeps the plain base name.
func TestPluginSSOBridgeDefaultPortPlainName(t *testing.T) {
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	svc := newEnabledAuthService(t, cfg)

	names := ssoBridgeCookieNames(t, svc, "example.com")
	if len(names) != 1 || names[0] != "kandev_session" {
		t.Fatalf("SSO Set-Cookie names = %v, want exactly [kandev_session]", names)
	}
}

// newRunSubscriptionCheckHarness builds a task service and office repository
// over one shared SQLite database, the same pairing gatewayAuthPolicy wires
// in production. WO-02 round 2's runSubscriptionCheck tests seed agent
// profiles and runs against it.
func newRunSubscriptionCheckHarness(t *testing.T) (*taskservice.Service, *officesqlite.Repository) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "run-subscribe.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	taskRepo, cleanup, err := repository.Provide(database, database, nil)
	if err != nil {
		t.Fatalf("task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	// agents/agent_profiles live in the settings store; officeRepo's runs
	// queue (GetRunWorkspaceID) joins against them.
	if _, _, err := settingsstore.Provide(database, database, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("office repository: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces:       taskRepo,
		Tasks:            taskRepo,
		TaskRepos:        taskRepo,
		Workflows:        taskRepo,
		Messages:         taskRepo,
		Turns:            taskRepo,
		Sessions:         taskRepo,
		GitSnapshots:     taskRepo,
		RepoEntities:     taskRepo,
		Executors:        taskRepo,
		Environments:     taskRepo,
		TaskEnvironments: taskRepo,
		Reviews:          taskRepo,
		ResourceCleanups: taskRepo,
	}, bus.NewMemoryEventBus(log), log, taskservice.RepositoryDiscoveryConfig{})
	return taskSvc, officeRepo
}

// seedRunForWorkspace inserts an agent + agent profile scoped to
// workspaceID (empty string reproduces an ordinary, non-Office Kanban agent
// profile — schema default, never backfilled) and a run against it,
// returning the run id.
func seedRunForWorkspace(t *testing.T, officeRepo *officesqlite.Repository, workspaceID string) string {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	agentID := "agent-" + workspaceID
	if workspaceID == "" {
		agentID = "agent-unscoped"
	}
	if _, err := officeRepo.ExecRaw(ctx, `INSERT INTO agents
		(id, name, workspace_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		agentID, agentID, workspaceID, now, now); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := officeRepo.ExecRaw(ctx, `INSERT INTO agent_profiles
		(id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, '', ?, ?)`,
		agentID, agentID, "name", "display", workspaceID, now, now); err != nil {
		t.Fatalf("seed agent profile: %v", err)
	}
	runID := "run-" + agentID
	if err := officeRepo.CreateRun(ctx, &models.Run{
		ID:             runID,
		AgentProfileID: agentID,
		Reason:         "task_assigned",
		Status:         models.RunStatus("queued"),
		CoalescedCount: 1,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	return runID
}

// TestRunSubscriptionCheckDeniesEmptyResolvedWorkspace pins WO-02 round 2's
// FINDING R1-1 fix: an ordinary (non-Office) Kanban agent profile resolves
// to workspace_id=” via GetRunWorkspaceID, which used to fall into
// AuthorizeWorkspaceAccess's unconditional-allow branch (workspaceID == ""
// => nil). The hook must deny outright instead of delegating — this is the
// dominant class of runs (workflow-engine queue_run against ordinary Kanban
// agent profiles), not an edge case.
func TestRunSubscriptionCheckDeniesEmptyResolvedWorkspace(t *testing.T) {
	taskSvc, officeRepo := newRunSubscriptionCheckHarness(t)
	runID := seedRunForWorkspace(t, officeRepo, "")

	check := runSubscriptionCheck(taskSvc, officeRepo)
	if err := check(context.Background(), runID); err == nil {
		t.Fatal("runSubscriptionCheck(empty workspace) = nil, want a denial error")
	}
}

// TestRunSubscriptionCheckAllowsResolvedWorkspace is the control: a run
// whose agent profile carries a real workspace id is unaffected by the
// empty-workspace fix.
func TestRunSubscriptionCheckAllowsResolvedWorkspace(t *testing.T) {
	taskSvc, officeRepo := newRunSubscriptionCheckHarness(t)
	runID := seedRunForWorkspace(t, officeRepo, "ws-1")

	check := runSubscriptionCheck(taskSvc, officeRepo)
	if err := check(context.Background(), runID); err != nil {
		t.Fatalf("runSubscriptionCheck(ws-1) = %v, want nil", err)
	}
}

// TestRunSubscriptionCheckDeniesWhenOfficeRepoNil pins FINDING R1-2: the
// nil-officeRepo guard must fail closed, not open. Not reachable in the
// shipped binary (repos.Office is never nil), but the wrong default for a
// security check.
func TestRunSubscriptionCheckDeniesWhenOfficeRepoNil(t *testing.T) {
	taskSvc, _ := newRunSubscriptionCheckHarness(t)

	check := runSubscriptionCheck(taskSvc, nil)
	if err := check(context.Background(), "any-run"); err == nil {
		t.Fatal("runSubscriptionCheck(nil officeRepo) = nil, want a denial error")
	}
}
