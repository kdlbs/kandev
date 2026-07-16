package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/plugins/store"
)

// newTestStateStore returns an in-memory-sqlite-backed *state.Store, for
// handler tests exercising plugins that declare the state capability.
func newTestStateStore(t *testing.T) *state.Store {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	st, err := state.NewStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new state store: %v", err)
	}
	return st
}

func newTestRouter(t *testing.T) (*gin.Engine, *Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, _, _ := newTestService(t)
	svc.SetState(newTestStateStore(t))
	router := gin.New()
	RegisterRoutes(router, svc, nil, testLogger(t))
	return router, svc
}

func doRequest(router *gin.Engine, method, path string, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func doMultipartInstall(t *testing.T, router *gin.Engine, pkg *bytes.Buffer) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("package", "plugin.tar.gz")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := io.Copy(part, pkg); err != nil {
		t.Fatalf("copy package into form: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/plugins/install", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestInstallHandlerFromURLCreatesActivePlugin(t *testing.T) {
	router, svc := newTestRouter(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(testPackage(t, "kandev-plugin-slack", "1.0.0", false).Bytes())
	}))
	defer srv.Close()

	rec := doRequest(router, http.MethodPost, "/api/plugins/install", fmt.Sprintf(`{"url":%q}`, srv.URL), nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp InstallResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Plugin.ID != "kandev-plugin-slack" {
		t.Fatalf("Plugin.ID = %q, want %q", resp.Plugin.ID, "kandev-plugin-slack")
	}
	if resp.Plugin.Status != StatusActive {
		t.Fatalf("Plugin.Status = %q, want %q", resp.Plugin.Status, StatusActive)
	}

	if _, err := svc.Get("kandev-plugin-slack"); err != nil {
		t.Fatalf("plugin not persisted in service: %v", err)
	}
}

func TestInstallHandlerMultipartUpload(t *testing.T) {
	router, svc := newTestRouter(t)

	rec := doMultipartInstall(t, router, testPackage(t, "kandev-plugin-slack", "1.0.0", false))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if _, err := svc.Get("kandev-plugin-slack"); err != nil {
		t.Fatalf("plugin not persisted in service: %v", err)
	}
}

func TestInstallHandlerMissingURLReturns400(t *testing.T) {
	router, _ := newTestRouter(t)
	rec := doRequest(router, http.MethodPost, "/api/plugins/install", `{}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestInstallHandlerDuplicateVersionReturns409(t *testing.T) {
	router, _ := newTestRouter(t)
	pkg := testPackage(t, "kandev-plugin-slack", "1.0.0", false)
	pkgBytes := pkg.Bytes()

	first := doMultipartInstall(t, router, bytes.NewBuffer(pkgBytes))
	if first.Code != http.StatusCreated {
		t.Fatalf("first install status = %d, want 201, body=%s", first.Code, first.Body.String())
	}

	second := doMultipartInstall(t, router, bytes.NewBuffer(pkgBytes))
	if second.Code != http.StatusConflict {
		t.Fatalf("second install status = %d, want 409, body=%s", second.Code, second.Body.String())
	}
}

func TestListHandlerReturnsInstalledPlugins(t *testing.T) {
	router, svc := newTestRouter(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	rec := doRequest(router, http.MethodGet, "/api/plugins", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Plugins []*store.Record `json:"plugins"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Plugins) != 1 {
		t.Fatalf("len(plugins) = %d, want 1", len(body.Plugins))
	}
}

func TestGetHandlerMissingReturns404(t *testing.T) {
	router, _ := newTestRouter(t)
	rec := doRequest(router, http.MethodGet, "/api/plugins/missing", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestEnableDisableHandlersTransitionStatus(t *testing.T) {
	router, svc := newTestRouter(t)
	installTestPlugin(t, svc, "kandev-plugin-slack") // already active after install

	rec := doRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-slack/disable", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	got, _ := svc.Get("kandev-plugin-slack")
	if got.Status != StatusDisabled {
		t.Fatalf("status after disable = %q, want %q", got.Status, StatusDisabled)
	}

	rec = doRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-slack/enable", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	got, _ = svc.Get("kandev-plugin-slack")
	if got.Status != StatusActive {
		t.Fatalf("status after enable = %q, want %q", got.Status, StatusActive)
	}
}

func TestUpdateConfigHandlerPersists(t *testing.T) {
	router, svc := newTestRouter(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	rec := doRequest(router, http.MethodPatch, "/api/plugins/kandev-plugin-slack", `{"config":{"default_channel":"#dev"}}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUninstallHandlerRemovesPlugin(t *testing.T) {
	router, svc := newTestRouter(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	rec := doRequest(router, http.MethodDelete, "/api/plugins/kandev-plugin-slack", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if _, err := svc.Get("kandev-plugin-slack"); err == nil {
		t.Fatal("plugin still present after uninstall")
	}
}

// toolPackage builds a valid runtime-managed package that also declares a
// single tool named "<id>_tool", for GET /api/plugins/tools tests.
func toolPackage(t *testing.T, id string) *bytes.Buffer {
	t.Helper()
	platformKey := goruntime.GOOS + "-" + goruntime.GOARCH
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 1
version: "1.0.0"
display_name: Test Plugin
tools:
  - name: %s_tool
    display_name: Tool
runtime:
  type: binary
  executables:
    %s: server/plugin
`, id, id, platformKey)

	var buf bytes.Buffer
	files := map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}
	if err := pkgtartest.WritePackage(&buf, files); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	return &buf
}

func TestListToolsOnlyIncludesActivePlugins(t *testing.T) {
	router, svc := newTestRouter(t)

	if _, err := svc.Install(t.Context(), toolPackage(t, "kandev-plugin-slack")); err != nil {
		t.Fatalf("Install(slack): %v", err)
	}
	if _, err := svc.Install(t.Context(), toolPackage(t, "kandev-plugin-jira")); err != nil {
		t.Fatalf("Install(jira): %v", err)
	}
	// kandev-plugin-jira gets disabled — its tool must not be listed.
	if err := svc.Disable("kandev-plugin-jira"); err != nil {
		t.Fatalf("Disable(jira): %v", err)
	}

	rec := doRequest(router, http.MethodGet, "/api/plugins/tools", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Tools []PluginToolDTO `json:"tools"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Tools) != 1 || body.Tools[0].PluginID != "kandev-plugin-slack" {
		t.Fatalf("tools = %+v, want exactly one tool from kandev-plugin-slack", body.Tools)
	}
}

func TestBundleHandlerServesFileFromDisk(t *testing.T) {
	router, svc := newTestRouter(t)
	if _, err := svc.Install(t.Context(), testPackage(t, "kandev-plugin-ui", "1.0.0", true)); err != nil {
		t.Fatalf("Install: %v", err)
	}

	rec := doRequest(router, http.MethodGet, "/api/plugins/kandev-plugin-ui/bundle", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/javascript; charset=utf-8", got)
	}
	if rec.Body.String() != "export default {};" {
		t.Fatalf("body = %q, want the bundle file contents", rec.Body.String())
	}
}

func TestUIHandlerServesStyleFileFromDisk(t *testing.T) {
	router, svc := newTestRouter(t)
	if _, err := svc.Install(t.Context(), testPackage(t, "kandev-plugin-ui", "1.0.0", true)); err != nil {
		t.Fatalf("Install: %v", err)
	}

	rec := doRequest(router, http.MethodGet, "/api/plugins/kandev-plugin-ui/ui/ui/style.css", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "body{}" {
		t.Fatalf("body = %q, want the style file contents", rec.Body.String())
	}
}

func TestUIHandlerPathTraversalRejected(t *testing.T) {
	router, svc := newTestRouter(t)
	if _, err := svc.Install(t.Context(), testPackage(t, "kandev-plugin-ui", "1.0.0", true)); err != nil {
		t.Fatalf("Install: %v", err)
	}

	rec := doRequest(router, http.MethodGet, "/api/plugins/kandev-plugin-ui/ui/../../../../etc/passwd", "", nil)
	if rec.Code == http.StatusOK {
		t.Fatalf("status = 200, want a non-200 response for a path-traversal attempt, body=%s", rec.Body.String())
	}
}

func TestBundleHandlerInactivePluginReturns503(t *testing.T) {
	router, svc := newTestRouter(t)
	if _, err := svc.Install(t.Context(), testPackage(t, "kandev-plugin-ui", "1.0.0", true)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := svc.Disable("kandev-plugin-ui"); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	rec := doRequest(router, http.MethodGet, "/api/plugins/kandev-plugin-ui/bundle", "", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body=%s", rec.Code, rec.Body.String())
	}
}

func TestWebhookHandlerUnknownPluginReturns404(t *testing.T) {
	router, _ := newTestRouter(t)
	rec := doRequest(router, http.MethodPost, "/api/plugins/missing/webhooks/key1", "{}", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestWebhookHandlerNotRunningReturns503(t *testing.T) {
	router, svc := newTestRouter(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	if err := svc.Disable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	rec := doRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-slack/webhooks/key1", "{}", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body=%s", rec.Code, rec.Body.String())
	}
}

func TestSyncHandlerRegistersDirSideload(t *testing.T) {
	router, svc := newTestRouter(t)
	pluginsDir := svc.pluginsDir
	versionDir := filepath.Join(pluginsDir, "kandev-plugin-side", "1.0.0")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	manifestYAML := fmt.Sprintf(`
id: kandev-plugin-side
api_version: 1
version: "1.0.0"
display_name: Sideloaded
runtime:
  type: binary
  executables:
    %s: server/plugin
`, goruntime.GOOS+"-"+goruntime.GOARCH)
	if err := os.WriteFile(filepath.Join(versionDir, "manifest.yaml"), []byte(manifestYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rec := doRequest(router, http.MethodPost, "/api/plugins/sync", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var resp SyncResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Added) != 1 || resp.Added[0] != "kandev-plugin-side" {
		t.Fatalf("Added = %v, want [kandev-plugin-side]", resp.Added)
	}

	got, err := svc.Get("kandev-plugin-side")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.Status != StatusDisabled {
		t.Fatalf("Status = %q, want %q", got.Status, StatusDisabled)
	}
}
