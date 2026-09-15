package config

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

var errGuardUnavailable = errors.New("guard: source unavailable")

// newTestRouter mounts the config routes over a fresh service, with no
// config sync guard wired (nil guard: no workspace is ever active).
func newTestRouter(t *testing.T) (*gin.Engine, *testEnv) {
	t.Helper()
	return newTestRouterWithGuard(t, nil)
}

// newTestRouterWithGuard mounts the config routes over a fresh service with
// the given ActiveSourceChecker.
func newTestRouterWithGuard(t *testing.T, guard ActiveSourceChecker) (*gin.Engine, *testEnv) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	env := newTestEnv(t)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), NewHandler(env.svc, guard, logger.Default()))
	return router, env
}

// fakeActiveSourceChecker is a test double for ActiveSourceChecker.
type fakeActiveSourceChecker struct {
	active bool
	err    error
}

func (f *fakeActiveSourceChecker) HasActiveSource(context.Context, string) (bool, error) {
	return f.active, f.err
}

// do issues a request and returns the recorder.
func do(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// decodeBody unmarshals a JSON response body, failing on malformed output.
func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, out interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, want, rec.Body.String())
	}
}

// assertErrorBody checks a failure response carries a non-empty error field.
func assertErrorBody(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	decodeBody(t, rec, &body)
	if body.Error == "" {
		t.Errorf("expected a non-empty error field, got %s", rec.Body.String())
	}
}

func TestNewHandler_WiresDependencies(t *testing.T) {
	env := newTestEnv(t)
	guard := &fakeActiveSourceChecker{}
	h := NewHandler(env.svc, guard, logger.Default())
	if h.svc != env.svc {
		t.Error("service not wired")
	}
	if h.guard != guard {
		t.Error("guard not wired")
	}
	if h.logger == nil {
		t.Error("logger not wired")
	}
}

func TestRegisterRoutes_RegistersEveryEndpoint(t *testing.T) {
	router, _ := newTestRouter(t)

	want := map[string]string{
		"GET /api/v1/workspaces/:wsId/config/export":          "",
		"GET /api/v1/workspaces/:wsId/config/export/zip":      "",
		"POST /api/v1/workspaces/:wsId/config/preview":        "",
		"POST /api/v1/workspaces/:wsId/config/import":         "",
		"GET /api/v1/workspaces/:wsId/config/sync/incoming":   "",
		"GET /api/v1/workspaces/:wsId/config/sync/outgoing":   "",
		"POST /api/v1/workspaces/:wsId/config/sync/import-fs": "",
		"POST /api/v1/workspaces/:wsId/config/sync/export-fs": "",
	}
	for _, r := range router.Routes() {
		delete(want, r.Method+" "+r.Path)
	}
	for missing := range want {
		t.Errorf("route not registered: %s", missing)
	}
}

func TestHandler_ExportConfig(t *testing.T) {
	router, env := newTestRouter(t)
	seedFullDBState(t, env, testWorkspaceID)

	rec := do(t, router, http.MethodGet, "/api/v1/workspaces/"+testWorkspaceID+"/config/export", "")
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Bundle ConfigBundle `json:"bundle"`
	}
	decodeBody(t, rec, &body)
	// The path parameter reaches the service: it labels the settings block.
	assertEqual(t, "settings name", body.Bundle.Settings.Name, testWorkspaceID)
	assertEqual(t, "agents", len(body.Bundle.Agents), 2)
	assertEqual(t, "skills", len(body.Bundle.Skills), 1)
	assertEqual(t, "routines", len(body.Bundle.Routines), 1)
	assertEqual(t, "projects", len(body.Bundle.Projects), 1)
}

func TestHandler_ExportConfig_ServiceError(t *testing.T) {
	router, env := newTestRouter(t)
	env.closeDB(t)

	rec := do(t, router, http.MethodGet, "/api/v1/workspaces/ws/config/export", "")
	assertStatus(t, rec, http.StatusInternalServerError)
	assertErrorBody(t, rec)
}

func TestHandler_ExportConfigZip(t *testing.T) {
	router, env := newTestRouter(t)
	seedFullDBState(t, env, testWorkspaceID)

	rec := do(t, router, http.MethodGet,
		"/api/v1/workspaces/"+testWorkspaceID+"/config/export/zip", "")
	assertStatus(t, rec, http.StatusOK)
	assertEqual(t, "content type", rec.Header().Get("Content-Type"), "application/zip")
	assertEqual(t, "content disposition", rec.Header().Get("Content-Disposition"),
		"attachment; filename=kandev-config.zip")

	entries := readZip(t, rec.Body)
	if _, ok := entries[".kandev/agents/ada.yml"]; !ok {
		t.Errorf("agent entry missing from the archive, got %v", entries)
	}
}

func TestHandler_ExportConfigZip_ServiceError(t *testing.T) {
	router, env := newTestRouter(t)
	env.closeDB(t)

	rec := do(t, router, http.MethodGet, "/api/v1/workspaces/ws/config/export/zip", "")
	assertStatus(t, rec, http.StatusInternalServerError)
	assertErrorBody(t, rec)
	if got := rec.Header().Get("Content-Type"); strings.HasPrefix(got, "application/zip") {
		t.Errorf("error response should not claim to be a zip, got %q", got)
	}
}

func TestHandler_PreviewImport(t *testing.T) {
	router, env := newTestRouter(t)
	seedAgent(t, env, testWorkspaceID, "ada")

	body := `{"agents":[{"name":"ada"},{"name":"grace"}],"skills":[{"slug":"triage"}]}`
	rec := do(t, router, http.MethodPost,
		"/api/v1/workspaces/"+testWorkspaceID+"/config/preview", body)
	assertStatus(t, rec, http.StatusOK)

	var out struct {
		Preview ImportPreview `json:"preview"`
	}
	decodeBody(t, rec, &out)
	assertStrings(t, "agents updated", out.Preview.Agents.Updated, []string{"ada"})
	assertStrings(t, "agents created", out.Preview.Agents.Created, []string{"grace"})
	assertStrings(t, "skills created", out.Preview.Skills.Created, []string{"triage"})

	// Preview must not have written anything.
	agents, err := env.repo.ListAgentInstances(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	assertEqual(t, "row count unchanged", len(agents), 1)
}

func TestHandler_PreviewImport_MalformedBody(t *testing.T) {
	router, _ := newTestRouter(t)

	for _, body := range []string{"{not json", "", `{"agents":"not-an-array"}`} {
		rec := do(t, router, http.MethodPost, "/api/v1/workspaces/ws/config/preview", body)
		assertStatus(t, rec, http.StatusBadRequest)
		assertErrorBody(t, rec)
	}
}

func TestHandler_PreviewImport_ServiceError(t *testing.T) {
	router, env := newTestRouter(t)
	env.closeDB(t)

	rec := do(t, router, http.MethodPost, "/api/v1/workspaces/ws/config/preview", `{}`)
	assertStatus(t, rec, http.StatusInternalServerError)
	assertErrorBody(t, rec)
}

func TestHandler_ApplyImport(t *testing.T) {
	router, env := newTestRouter(t)

	body := `{"agents":[{"name":"ada","role":"cto","budget_monthly_cents":4200,
		"max_concurrent_sessions":3,"executor_preference":"local_docker"}],
		"projects":[{"name":"apollo","budget_cents":99000}]}`
	rec := do(t, router, http.MethodPost,
		"/api/v1/workspaces/"+testWorkspaceID+"/config/import", body)
	assertStatus(t, rec, http.StatusOK)

	var out struct {
		Result ImportResult `json:"result"`
	}
	decodeBody(t, rec, &out)
	assertEqual(t, "created count", out.Result.CreatedCount, 2)
	assertEqual(t, "updated count", out.Result.UpdatedCount, 0)

	// The workspace path parameter scopes the write.
	agent := agentByName(t, env, testWorkspaceID, "ada")
	assertEqual(t, "agent role", string(agent.Role), "cto")
	assertEqual(t, "agent budget", agent.BudgetMonthlyCents, 4200)
	assertEqual(t, "agent max sessions", agent.MaxConcurrentSessions, 3)
	assertEqual(t, "agent executor preference", agent.ExecutorPreference, "local_docker")
	assertEqual(t, "project budget",
		projectByName(t, env, testWorkspaceID, "apollo").BudgetCents, 99000)
}

func TestHandler_ApplyImport_MalformedBody(t *testing.T) {
	router, env := newTestRouter(t)

	rec := do(t, router, http.MethodPost,
		"/api/v1/workspaces/"+testWorkspaceID+"/config/import", "{not json")
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorBody(t, rec)

	// Scoped to the workspace the request actually targeted: listing a
	// workspace nothing writes to would pass no matter what the handler did.
	agents, err := env.repo.ListAgentInstances(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	assertEqual(t, "a rejected body writes nothing", len(agents), 0)
}

func TestHandler_ApplyImport_ServiceError(t *testing.T) {
	router, env := newTestRouter(t)
	env.closeDB(t)

	rec := do(t, router, http.MethodPost, "/api/v1/workspaces/ws/config/import",
		`{"agents":[{"name":"ada"}]}`)
	assertStatus(t, rec, http.StatusInternalServerError)
	assertErrorBody(t, rec)
}

func TestHandler_SyncIncomingDiff(t *testing.T) {
	router, env := newTestRouter(t)
	seedFullFSState(t, env)
	seedAgent(t, env, testWorkspaceID, "gone-agent")

	rec := do(t, router, http.MethodGet,
		"/api/v1/workspaces/"+testWorkspaceID+"/config/sync/incoming", "")
	assertStatus(t, rec, http.StatusOK)

	var out struct {
		Diff SyncDiff `json:"diff"`
	}
	decodeBody(t, rec, &out)
	assertEqual(t, "direction", out.Diff.Direction, "incoming")
	assertStrings(t, "agents created", out.Diff.Preview.Agents.Created, []string{"ada"})
	assertStrings(t, "agents deleted", out.Diff.Preview.Agents.Deleted, []string{"gone-agent"})
	assertEqual(t, "errors omitted when empty", len(out.Diff.Errors), 0)
}

func TestHandler_SyncIncomingDiff_ServiceError(t *testing.T) {
	router, env := newTestRouter(t)
	env.closeDB(t)

	rec := do(t, router, http.MethodGet, "/api/v1/workspaces/ws/config/sync/incoming", "")
	assertStatus(t, rec, http.StatusInternalServerError)
	assertErrorBody(t, rec)
}

func TestHandler_SyncOutgoingDiff(t *testing.T) {
	router, env := newTestRouter(t)
	seedAgent(t, env, testWorkspaceID, "ada")

	rec := do(t, router, http.MethodGet,
		"/api/v1/workspaces/"+testWorkspaceID+"/config/sync/outgoing", "")
	assertStatus(t, rec, http.StatusOK)

	var out struct {
		Diff SyncDiff `json:"diff"`
	}
	decodeBody(t, rec, &out)
	assertEqual(t, "direction", out.Diff.Direction, "outgoing")
	assertStrings(t, "agents created", out.Diff.Preview.Agents.Created, []string{"ada"})
}

func TestHandler_SyncOutgoingDiff_ServiceError(t *testing.T) {
	router, env := newTestRouter(t)
	env.closeDB(t)

	rec := do(t, router, http.MethodGet, "/api/v1/workspaces/ws/config/sync/outgoing", "")
	assertStatus(t, rec, http.StatusInternalServerError)
	assertErrorBody(t, rec)
}

func TestHandler_SyncApplyIncoming(t *testing.T) {
	router, env := newTestRouter(t)
	seedFullFSState(t, env)
	seedAgent(t, env, testWorkspaceID, "gone-agent")

	rec := do(t, router, http.MethodPost,
		"/api/v1/workspaces/"+testWorkspaceID+"/config/sync/import-fs", "")
	assertStatus(t, rec, http.StatusOK)

	var out struct {
		Result ImportResult `json:"result"`
	}
	decodeBody(t, rec, &out)
	assertEqual(t, "created count", out.Result.CreatedCount, 4)

	assertRemainingNames(t, env, testWorkspaceID, []string{"ada"}, []string{"code-review"},
		[]string{"standup"}, []string{"apollo"})
}

func TestHandler_SyncApplyIncoming_ServiceError(t *testing.T) {
	router, env := newTestRouter(t)
	env.closeDB(t)

	rec := do(t, router, http.MethodPost, "/api/v1/workspaces/ws/config/sync/import-fs", "")
	assertStatus(t, rec, http.StatusInternalServerError)
	assertErrorBody(t, rec)
}

func TestHandler_SyncApplyOutgoing(t *testing.T) {
	router, env := newTestRouter(t)
	seedFullDBState(t, env, testWorkspaceID)

	rec := do(t, router, http.MethodPost,
		"/api/v1/workspaces/"+testWorkspaceID+"/config/sync/export-fs", "")
	assertStatus(t, rec, http.StatusOK)

	var out struct {
		Status string `json:"status"`
	}
	decodeBody(t, rec, &out)
	assertEqual(t, "status", out.Status, "ok")

	bundle, _, err := env.svc.ScanFilesystem(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("ScanFilesystem: %v", err)
	}
	assertStrings(t, "agents on disk",
		sortedCopy(agentBundleNames(bundle.Agents)), []string{"ada", "grace"})
}

func TestHandler_SyncApplyOutgoing_ServiceError(t *testing.T) {
	router, env := newTestRouter(t)
	env.closeDB(t)

	rec := do(t, router, http.MethodPost, "/api/v1/workspaces/ws/config/sync/export-fs", "")
	assertStatus(t, rec, http.StatusInternalServerError)
	assertErrorBody(t, rec)
}

// guardedRoutes are the write routes that must refuse (409) when config sync
// is the active source for the target workspace (AC-OFFICE-CONFIG-SYNC-005.2/005.2b).
var guardedRoutes = []struct {
	name   string
	method string
	path   string
}{
	{"applyImport", http.MethodPost, "/api/v1/workspaces/" + testWorkspaceID + "/config/import"},
	{"syncApplyIncoming", http.MethodPost, "/api/v1/workspaces/" + testWorkspaceID + "/config/sync/import-fs"},
	{"syncApplyOutgoing", http.MethodPost, "/api/v1/workspaces/" + testWorkspaceID + "/config/sync/export-fs"},
}

func TestHandler_GuardedRoutes_RefuseWhenConfigSyncActive(t *testing.T) {
	for _, rt := range guardedRoutes {
		t.Run(rt.name, func(t *testing.T) {
			router, _ := newTestRouterWithGuard(t, &fakeActiveSourceChecker{active: true})
			rec := do(t, router, rt.method, rt.path, "{}")
			assertStatus(t, rec, http.StatusConflict)
			assertErrorBody(t, rec)
		})
	}
}

func TestHandler_GuardedRoutes_RefuseWhenGuardErrors(t *testing.T) {
	for _, rt := range guardedRoutes {
		t.Run(rt.name, func(t *testing.T) {
			router, _ := newTestRouterWithGuard(t, &fakeActiveSourceChecker{err: errGuardUnavailable})
			rec := do(t, router, rt.method, rt.path, "{}")
			assertStatus(t, rec, http.StatusConflict)
			assertErrorBody(t, rec)
		})
	}
}

func TestHandler_GuardedRoutes_ProceedWhenConfigSyncInactive(t *testing.T) {
	for _, rt := range guardedRoutes {
		t.Run(rt.name, func(t *testing.T) {
			router, _ := newTestRouterWithGuard(t, &fakeActiveSourceChecker{active: false})
			rec := do(t, router, rt.method, rt.path, "{}")
			if rec.Code == http.StatusConflict {
				t.Errorf("expected the route to proceed past the guard, got 409: %s", rec.Body.String())
			}
		})
	}
}
