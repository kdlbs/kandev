package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
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
	// A vault is mandatory for plugins with secret config fields (Service
	// fails closed without one), so wire an in-memory one, matching prod
	// where Provide always attaches the shared vault.
	svc.SetSecrets(newFakeSecretRevealer())
	router := gin.New()
	RegisterRoutes(router, svc, nil, testLogger(t))
	return router, svc
}

func newTestRouterWithIdentity(t *testing.T, identity authn.Identity) (*gin.Engine, *Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, _, _ := newTestService(t)
	svc.SetState(newTestStateStore(t))
	// A vault is mandatory for plugins with secret config fields (Service
	// fails closed without one), so wire an in-memory one, matching prod
	// where Provide always attaches the shared vault.
	svc.SetSecrets(newFakeSecretRevealer())
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		authn.SetOnGin(ctx, identity)
		ctx.Next()
	})
	RegisterRoutes(router, svc, nil, testLogger(t))
	return router, svc
}

func newAdminTestRouter(t *testing.T) (*gin.Engine, *Service) {
	t.Helper()
	return newTestRouterWithIdentity(t, authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin})
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

// doAuthedRequest attaches a resolved caller identity, standing in for
// httpmw.Middleware without wiring it into focused handler tests.
func doAuthedRequest(router *gin.Engine, method, path string, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "user_1", Role: authn.RoleMember}))
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
	router, svc := newAdminTestRouter(t)

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

func TestInstallHandlerRejectsMemberBeforeURLFetch(t *testing.T) {
	router, _ := newTestRouterWithIdentity(t, authn.Identity{
		UserID: "member-1",
		Role:   authn.RoleMember,
	})
	var fetched atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetched.Store(true)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	rec := doRequest(router, http.MethodPost, "/api/plugins/install", fmt.Sprintf(`{"url":%q}`, srv.URL), nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if fetched.Load() {
		t.Fatal("member-controlled install URL reached the network")
	}
}

func TestInstallHandlerMultipartUpload(t *testing.T) {
	router, svc := newAdminTestRouter(t)

	rec := doMultipartInstall(t, router, testPackage(t, "kandev-plugin-slack", "1.0.0", false))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if _, err := svc.Get("kandev-plugin-slack"); err != nil {
		t.Fatalf("plugin not persisted in service: %v", err)
	}
}

func TestInstallHandlerMissingURLReturns400(t *testing.T) {
	router, _ := newAdminTestRouter(t)
	rec := doRequest(router, http.MethodPost, "/api/plugins/install", `{}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestInstallHandlerDuplicateVersionReturns409(t *testing.T) {
	router, _ := newAdminTestRouter(t)
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

// TestListHandlerExposesUIKeybindings confirms that ui.keybindings declared
// in the manifest round-trips through GET /api/plugins into the JSON each
// plugin entry carries for the frontend (record.ui.keybindings, since
// store.Record embeds manifest.Manifest inline).
func TestListHandlerExposesUIKeybindings(t *testing.T) {
	router, svc := newTestRouter(t)
	platformKey := goruntime.GOOS + "-" + goruntime.GOARCH
	manifestYAML := fmt.Sprintf(`
id: kandev-plugin-keybound
api_version: 1
version: "1.0.0"
display_name: Test Plugin
capabilities:
  events: ["task.*"]
runtime:
  type: binary
  executables:
    %s: server/plugin
ui:
  bundle: "/ui/bundle.js"
  keybindings:
    - id: open-demo
      default: mod+shift+k
      description: Open the demo modal
`, platformKey)
	var buf bytes.Buffer
	if err := pkgtartest.WritePackage(&buf, map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
		"ui/bundle.js":  []byte("export default {};"),
	}); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	if _, err := svc.Install(context.Background(), &buf); err != nil {
		t.Fatalf("Install: %v", err)
	}

	rec := doRequest(router, http.MethodGet, "/api/plugins", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Plugins []struct {
			UI struct {
				Keybindings []struct {
					ID          string `json:"id"`
					Default     string `json:"default"`
					Description string `json:"description"`
				} `json:"keybindings"`
			} `json:"ui"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Plugins) != 1 {
		t.Fatalf("len(plugins) = %d, want 1", len(body.Plugins))
	}
	kbs := body.Plugins[0].UI.Keybindings
	if len(kbs) != 1 {
		t.Fatalf("len(ui.keybindings) = %d, want 1", len(kbs))
	}
	if kbs[0].ID != "open-demo" || kbs[0].Default != "mod+shift+k" || kbs[0].Description != "Open the demo modal" {
		t.Fatalf("ui.keybindings[0] = %+v, want id=open-demo default=mod+shift+k description=%q", kbs[0], "Open the demo modal")
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

// webhookPackage builds a valid runtime-managed package that declares
// exactly one webhook with the given key, for POST/GET
// /api/plugins/:id/webhooks/:key tests.
func webhookPackage(t *testing.T, id, key string) *bytes.Buffer {
	t.Helper()
	platformKey := goruntime.GOOS + "-" + goruntime.GOARCH
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 1
version: "1.0.0"
display_name: Test Plugin
webhooks:
  - key: %s
    method: POST
runtime:
  type: binary
  executables:
    %s: server/plugin
`, id, key, platformKey)

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

func authenticatedWebhookPackage(t *testing.T, id, key string, maxBodyBytes int64) *bytes.Buffer {
	t.Helper()
	platformKey := goruntime.GOOS + "-" + goruntime.GOARCH
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 1
version: "1.0.0"
display_name: Test Plugin
webhooks:
  - key: %s
    access: authenticated
    max_body_bytes: %d
runtime:
  type: binary
  executables:
    %s: server/plugin
`, id, key, maxBodyBytes, platformKey)
	var buf bytes.Buffer
	if err := pkgtartest.WritePackage(&buf, map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	return &buf
}

func TestWebhookHandlerUnknownPluginReturns404(t *testing.T) {
	router, _ := newTestRouter(t)
	rec := doAuthedRequest(router, http.MethodPost, "/api/plugins/missing/webhooks/key1", "{}", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

// TestWebhookHandlerUndeclaredKeyReturns404 pins the fix that the webhook
// relay validates :key against the plugin's manifest-declared webhooks
// before ever reaching the live subprocess: a key the plugin never
// registered must 404, not be blindly forwarded.
func TestWebhookHandlerUndeclaredKeyReturns404(t *testing.T) {
	router, svc := newTestRouter(t)
	if _, err := svc.Install(t.Context(), webhookPackage(t, "kandev-plugin-slack", "declared-key")); err != nil {
		t.Fatalf("Install: %v", err)
	}

	rec := doAuthedRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-slack/webhooks/undeclared-key", "{}", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

// TestWebhookHandlerOversizedBodyReturns413 pins the fix that the webhook
// relay caps the request body instead of an unbounded io.ReadAll (an
// external-caller-triggerable OOM DoS otherwise).
func TestWebhookHandlerOversizedBodyReturns413(t *testing.T) {
	router, svc := newTestRouter(t)
	if _, err := svc.Install(t.Context(), webhookPackage(t, "kandev-plugin-slack", "key1")); err != nil {
		t.Fatalf("Install: %v", err)
	}

	oversized := strings.Repeat("a", int(maxWebhookBodyBytes+1))
	rec := doAuthedRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-slack/webhooks/key1", oversized, nil)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413, body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthenticatedWebhookRejectsAnonymousRequest(t *testing.T) {
	router, svc := newTestRouter(t)
	if _, err := svc.Install(t.Context(), authenticatedWebhookPackage(t, "kandev-plugin-voice", "transcribe", 16<<20)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	rec := doRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-voice/webhooks/transcribe", "audio", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body.String())
	}
}

func TestReadCappedWebhookBodyUsesDeclaredLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("12345"))
	_, err := readCappedWebhookBody(ctx, 4)
	if err == nil || rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("error = %v, status = %d, want size error and 413", err, rec.Code)
	}
}

func TestWebhookRelayHeadersStripHostCredentials(t *testing.T) {
	headers := http.Header{
		"Authorization": []string{"Bearer kandev_pat_secret"},
		"Cookie":        []string{"kandev_session=secret"},
		"Content-Type":  []string{"audio/webm"},
		"X-Plugin-Key":  []string{"plugin-secret"},
	}

	got := flattenHeaders(headers, "kandev_session", false)

	if _, ok := got["Authorization"]; ok {
		t.Fatal("Authorization header was forwarded to plugin")
	}
	if _, ok := got["Cookie"]; ok {
		t.Fatal("Cookie header was forwarded to plugin")
	}
	if got["Content-Type"] != "audio/webm" || got["X-Plugin-Key"] != "plugin-secret" {
		t.Fatalf("relay-safe headers = %#v", got)
	}
}

func TestWebhookHandlerNotRunningReturns503(t *testing.T) {
	router, svc := newTestRouter(t)
	if _, err := svc.Install(t.Context(), webhookPackage(t, "kandev-plugin-slack", "key1")); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := svc.Disable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	rec := doAuthedRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-slack/webhooks/key1", "{}", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body=%s", rec.Code, rec.Body.String())
	}
}

type recordingActionInvoker struct {
	calls      int
	request    *pluginsdk.PluginActionRequest
	generation pluginDispatchGeneration
	response   *pluginsdk.PluginActionResponse
	err        error
	invoke     func(context.Context, string, *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error)
}

func (f *recordingActionInvoker) InvokeAction(
	ctx context.Context, id string, generation pluginDispatchGeneration, req *pluginsdk.PluginActionRequest,
) (*pluginsdk.PluginActionResponse, error) {
	f.calls++
	f.request = req
	f.generation = generation
	if f.invoke != nil {
		return f.invoke(ctx, id, req)
	}
	return f.response, f.err
}

// actionPackage builds a valid runtime-managed package with one declared,
// workspace-scoped action. Its small, explicit body cap makes the action
// relay's cap behavior testable without large test inputs.
func actionPackage(t *testing.T, id, key, resourceScope string, maxBodyBytes int) *bytes.Buffer {
	t.Helper()
	platformKey := goruntime.GOOS + "-" + goruntime.GOARCH
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 1
version: "1.0.0"
display_name: Test Plugin
actions:
  - key: %s
    resource_scope: %s
    max_body_bytes: %d
runtime:
  type: binary
  executables:
    %s: server/plugin
`, id, key, resourceScope, maxBodyBytes, platformKey)

	var buf bytes.Buffer
	if err := pkgtartest.WritePackage(&buf, map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	return &buf
}

func newActionTestRouter(t *testing.T, invoker *recordingActionInvoker) (*gin.Engine, *Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, _, _ := newTestService(t)
	svc.taskData = &fakeTaskDataSource{
		workspaces: []*taskmodels.Workspace{{ID: "workspace-1"}},
	}
	router := gin.New()
	ctrl := &Controller{svc: svc, log: testLogger(t), actionInvoker: invoker}
	router.POST("/api/plugins/:id/actions/:key", ctrl.action)
	return router, svc
}

func doAuthenticatedActionRequest(router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "user-1", Role: authn.RoleMember}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestActionHandlerDispatchesVerifiedContext proves browser supplied scope
// selectors are verified by the host and that plugins receive the trusted
// actor/workspace context, never raw authentication material.
func TestActionHandlerDispatchesVerifiedContext(t *testing.T) {
	invoker := &recordingActionInvoker{response: &pluginsdk.PluginActionResponse{Body: []byte(`{"ok":true}`)}}
	router, svc := newActionTestRouter(t, invoker)
	if _, err := svc.Install(t.Context(), actionPackage(t, "kandev-plugin-actions", "create-task", "workspace", 128)); err != nil {
		t.Fatalf("Install: %v", err)
	}

	rec := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/create-task",
		`{"workspaceId":"workspace-1","body":{"title":"Ship it"}}`,
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if invoker.calls != 1 {
		t.Fatalf("action invocations = %d, want 1", invoker.calls)
	}
	if got := invoker.request.Context; got.ActorID != "user-1" || got.WorkspaceID != "workspace-1" || got.TaskID != "" || got.RepositoryID != "" {
		t.Fatalf("verified action context = %+v, want actor=user-1 workspace=workspace-1", got)
	}
	if got := string(invoker.request.Body); got != `{"title":"Ship it"}` {
		t.Fatalf("action body = %s, want original action body", got)
	}
}

func TestActionHandlerRejectsUndeclaredAndUnauthorizedResources(t *testing.T) {
	invoker := &recordingActionInvoker{response: &pluginsdk.PluginActionResponse{Body: []byte(`{}`)}}
	router, svc := newActionTestRouter(t, invoker)
	if _, err := svc.Install(t.Context(), actionPackage(t, "kandev-plugin-actions", "declared", "workspace", 128)); err != nil {
		t.Fatalf("Install: %v", err)
	}

	undeclared := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/undeclared",
		`{"workspaceId":"workspace-1","body":{}}`,
	)
	if undeclared.Code != http.StatusNotFound {
		t.Fatalf("undeclared status = %d, want 404, body=%s", undeclared.Code, undeclared.Body.String())
	}

	unauthorized := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/declared",
		`{"workspaceId":"workspace-not-visible","body":{}}`,
	)
	if unauthorized.Code != http.StatusNotFound {
		t.Fatalf("unauthorized status = %d, want 404, body=%s", unauthorized.Code, unauthorized.Body.String())
	}
	if invoker.calls != 0 {
		t.Fatalf("action invocations = %d, want 0", invoker.calls)
	}

	unauthenticatedReq := httptest.NewRequest(
		http.MethodPost,
		"/api/plugins/kandev-plugin-actions/actions/declared",
		strings.NewReader(`{"workspaceId":"workspace-1","body":{}}`),
	)
	unauthenticatedReq.Header.Set("Content-Type", "application/json")
	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, unauthenticatedReq)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401, body=%s", unauthenticated.Code, unauthenticated.Body.String())
	}
}

func TestActionHandlerDerivesTaskWorkspaceAndRejectsMismatch(t *testing.T) {
	invoker := &recordingActionInvoker{response: &pluginsdk.PluginActionResponse{Body: []byte(`{}`)}}
	router, svc := newActionTestRouter(t, invoker)
	svc.taskData = &fakeTaskDataSource{
		tasksByID: map[string]*taskmodels.Task{
			"task-1": {ID: "task-1", WorkspaceID: "workspace-1"},
		},
	}
	if _, err := svc.Install(t.Context(), actionPackage(t, "kandev-plugin-actions", "review", "task", 128)); err != nil {
		t.Fatalf("Install: %v", err)
	}

	valid := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/review",
		`{"taskId":"task-1","body":{}}`,
	)
	if valid.Code != http.StatusOK {
		t.Fatalf("valid task status = %d, want 200, body=%s", valid.Code, valid.Body.String())
	}
	if got := invoker.request.Context; got.WorkspaceID != "workspace-1" || got.TaskID != "task-1" {
		t.Fatalf("verified task context = %+v, want workspace-1/task-1", got)
	}

	mismatch := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/review",
		`{"workspaceId":"workspace-not-visible","taskId":"task-1","body":{}}`,
	)
	if mismatch.Code != http.StatusBadRequest {
		t.Fatalf("mismatch status = %d, want 400, body=%s", mismatch.Code, mismatch.Body.String())
	}
	if invoker.calls != 1 {
		t.Fatalf("action invocations = %d, want 1", invoker.calls)
	}
}

func TestActionHandlerVerifiesOptionalTaskRepository(t *testing.T) {
	invoker := &recordingActionInvoker{response: &pluginsdk.PluginActionResponse{Body: []byte(`{}`)}}
	router, svc := newActionTestRouter(t, invoker)
	svc.taskData = &fakeTaskDataSource{
		tasksByID: map[string]*taskmodels.Task{
			"task-1": {
				ID:          "task-1",
				WorkspaceID: "workspace-1",
				Repositories: []*taskmodels.TaskRepository{
					{RepositoryID: "repository-1"},
				},
			},
		},
		sessionsByTask: map[string][]*taskmodels.TaskSession{
			"task-1": {{
				ID:     "session-1",
				TaskID: "task-1",
				Worktrees: []*taskmodels.TaskEnvironmentRepo{{
					RepositoryID:   "repository-1",
					WorktreeBranch: "feature/native-create",
				}},
			}},
		},
	}
	if _, err := svc.Install(t.Context(), actionPackage(t, "kandev-plugin-actions", "create-review", "task", 128)); err != nil {
		t.Fatalf("Install: %v", err)
	}

	valid := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/create-review",
		`{"workspaceId":"workspace-1","taskId":"task-1","sessionId":"session-1","repositoryId":"repository-1","body":{}}`,
	)
	if valid.Code != http.StatusOK {
		t.Fatalf("task repository status = %d, want 200, body=%s", valid.Code, valid.Body.String())
	}
	if got := invoker.request.Context; got.WorkspaceID != "workspace-1" || got.TaskID != "task-1" || got.RepositoryID != "repository-1" {
		t.Fatalf("verified task repository context = %+v", got)
	}
	if got := invoker.request.Context.SessionID; got != "session-1" {
		t.Fatalf("verified session = %q, want session-1", got)
	}
	if got := invoker.request.Context.HeadBranch; got != "feature/native-create" {
		t.Fatalf("verified head branch = %q, want feature/native-create", got)
	}

	unverifiedSession := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/create-review",
		`{"workspaceId":"workspace-1","taskId":"task-1","sessionId":"session-not-on-task","repositoryId":"repository-1","body":{}}`,
	)
	if unverifiedSession.Code != http.StatusNotFound {
		t.Fatalf("unverified session status = %d, want 404, body=%s", unverifiedSession.Code, unverifiedSession.Body.String())
	}

	unattached := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/create-review",
		`{"workspaceId":"workspace-1","taskId":"task-1","repositoryId":"repository-2","body":{}}`,
	)
	if unattached.Code != http.StatusNotFound {
		t.Fatalf("unattached repository status = %d, want 404, body=%s", unattached.Code, unattached.Body.String())
	}
	if invoker.calls != 1 {
		t.Fatalf("action invocations = %d, want 1", invoker.calls)
	}
}

func TestActionHandlerVerifiesRepositoryScope(t *testing.T) {
	invoker := &recordingActionInvoker{response: &pluginsdk.PluginActionResponse{Body: []byte(`{}`)}}
	router, svc := newActionTestRouter(t, invoker)
	svc.taskData = &fakeTaskDataSource{
		repositories: map[string][]*taskmodels.Repository{
			"workspace-1": {{ID: "repository-1", WorkspaceID: "workspace-1"}},
		},
	}
	if _, err := svc.Install(t.Context(), actionPackage(t, "kandev-plugin-actions", "browse", "repository", 128)); err != nil {
		t.Fatalf("Install: %v", err)
	}

	valid := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/browse",
		`{"workspaceId":"workspace-1","repositoryId":"repository-1","body":{}}`,
	)
	if valid.Code != http.StatusOK {
		t.Fatalf("repository status = %d, want 200, body=%s", valid.Code, valid.Body.String())
	}
	if got := invoker.request.Context; got.WorkspaceID != "workspace-1" || got.RepositoryID != "repository-1" || got.TaskID != "" {
		t.Fatalf("verified repository context = %+v, want workspace-1/repository-1", got)
	}

	missing := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/browse",
		`{"workspaceId":"workspace-1","repositoryId":"not-visible","body":{}}`,
	)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing repository status = %d, want 404, body=%s", missing.Code, missing.Body.String())
	}
	if invoker.calls != 1 {
		t.Fatalf("action invocations = %d, want 1", invoker.calls)
	}
}

func TestActionHandlerEnforcesBodyResponseAndHeaderLimits(t *testing.T) {
	invoker := &recordingActionInvoker{response: &pluginsdk.PluginActionResponse{
		Headers: map[string]string{
			"Cache-Control": "no-store",
			"Content-Type":  "text/plain; charset=utf-8",
			"Set-Cookie":    "session=attacker",
			"X-Plugin":      "untrusted",
		},
		Body: []byte("safe"),
	}}
	router, svc := newActionTestRouter(t, invoker)
	if _, err := svc.Install(t.Context(), actionPackage(t, "kandev-plugin-actions", "declared", "workspace", 16)); err != nil {
		t.Fatalf("Install: %v", err)
	}

	overBody := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/declared",
		fmt.Sprintf(`{"workspaceId":"workspace-1","body":%q}`, strings.Repeat("x", 17)),
	)
	if overBody.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body status = %d, want 413, body=%s", overBody.Code, overBody.Body.String())
	}
	if invoker.calls != 0 {
		t.Fatalf("action invocations after oversized body = %d, want 0", invoker.calls)
	}

	valid := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/declared",
		`{"workspaceId":"workspace-1","body":{}}`,
	)
	if valid.Code != http.StatusOK {
		t.Fatalf("safe response status = %d, want 200, body=%s", valid.Code, valid.Body.String())
	}
	if got := valid.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want allowed plugin value", got)
	}
	if got := valid.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want allowed plugin value", got)
	}
	if got := valid.Header().Get("Set-Cookie"); got != "" {
		t.Fatalf("Set-Cookie = %q, want blocked", got)
	}
	if got := valid.Header().Get("X-Plugin"); got != "" {
		t.Fatalf("X-Plugin = %q, want blocked", got)
	}

	invoker.response = &pluginsdk.PluginActionResponse{Body: make([]byte, maxPluginActionResponseBytes+1)}
	overResponse := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/declared",
		`{"workspaceId":"workspace-1","body":{}}`,
	)
	if overResponse.Code != http.StatusBadGateway {
		t.Fatalf("oversized response status = %d, want 502, body=%s", overResponse.Code, overResponse.Body.String())
	}
}

func TestActionHandlerPreservesValidatedDomainStatusAndRetryHeader(t *testing.T) {
	invoker := &recordingActionInvoker{response: &pluginsdk.PluginActionResponse{
		Status: http.StatusTooManyRequests,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"ETag":         `"review-42"`,
			"Retry-After":  "30",
		},
		Body: []byte(`{"error":"rate_limited"}`),
	}}
	router, svc := newActionTestRouter(t, invoker)
	if _, err := svc.Install(t.Context(), actionPackage(t, "kandev-plugin-actions", "declared", "workspace", 16)); err != nil {
		t.Fatalf("Install: %v", err)
	}

	response := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/declared",
		`{"workspaceId":"workspace-1","body":{}}`,
	)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429, body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After = %q, want 30", got)
	}
	if got := response.Header().Get("ETag"); got != `"review-42"` {
		t.Fatalf("ETag = %q, want review-42", got)
	}

	invoker.response.Status = http.StatusContinue
	invalid := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/declared",
		`{"workspaceId":"workspace-1","body":{}}`,
	)
	if invalid.Code != http.StatusBadGateway {
		t.Fatalf("invalid plugin status = %d, want 502, body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestActionHandlerTimeoutCancelsPluginCallAndLifecycleRevokesAccess(t *testing.T) {
	invoker := &recordingActionInvoker{}
	router, svc := newActionTestRouter(t, invoker)
	if _, err := svc.Install(t.Context(), actionPackage(t, "kandev-plugin-actions", "declared", "workspace", 128)); err != nil {
		t.Fatalf("Install: %v", err)
	}

	oldTimeout := pluginActionTimeout
	pluginActionTimeout = 5 * time.Millisecond
	t.Cleanup(func() { pluginActionTimeout = oldTimeout })
	canceled := false
	invoker.invoke = func(ctx context.Context, _ string, _ *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
		<-ctx.Done()
		canceled = errors.Is(ctx.Err(), context.DeadlineExceeded)
		return nil, ctx.Err()
	}
	timedOut := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/declared",
		`{"workspaceId":"workspace-1","body":{}}`,
	)
	if timedOut.Code != http.StatusGatewayTimeout || !canceled {
		t.Fatalf("timeout status/cancellation = %d/%t, want 504/true, body=%s", timedOut.Code, canceled, timedOut.Body.String())
	}

	if err := svc.Disable("kandev-plugin-actions"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	revoked := doAuthenticatedActionRequest(
		router,
		"/api/plugins/kandev-plugin-actions/actions/declared",
		`{"workspaceId":"workspace-1","body":{}}`,
	)
	if revoked.Code != http.StatusServiceUnavailable {
		t.Fatalf("revoked status = %d, want 503, body=%s", revoked.Code, revoked.Body.String())
	}
	if invoker.calls != 1 {
		t.Fatalf("action invocations = %d, want 1 after revoked request", invoker.calls)
	}
}

// TestWebhookStatusForResponse_ValidStatusesPassThrough pins that any
// syntactically valid HTTP status code a plugin returns is relayed
// unchanged.
func TestWebhookStatusForResponse_ValidStatusesPassThrough(t *testing.T) {
	for _, status := range []int32{100, 200, 204, 302, 404, 500, 599} {
		got, ok := webhookStatusForResponse(status)
		if !ok {
			t.Fatalf("webhookStatusForResponse(%d) ok = false, want true", status)
		}
		if got != int(status) {
			t.Fatalf("webhookStatusForResponse(%d) = %d, want %d", status, got, status)
		}
	}
}

// TestWebhookStatusForResponse_OutOfRangeStatusesAreRejected pins the fix
// for a plugin returning a status outside [100, 599]: gin's WriteHeader
// panics (-> 500) on such a code, so the webhook relay must reject it
// before ever calling WriteHeader.
func TestWebhookStatusForResponse_OutOfRangeStatusesAreRejected(t *testing.T) {
	for _, status := range []int32{0, -1, 99, 600, 1000} {
		if _, ok := webhookStatusForResponse(status); ok {
			t.Fatalf("webhookStatusForResponse(%d) ok = true, want false (out of [100,599])", status)
		}
	}
}

// TestWriteWebhookResponse_OutOfRangePluginStatusReturns502 exercises the
// exact code path the webhook handler uses to turn a plugin's
// WebhookResponse into the outbound HTTP response: given an out-of-range
// plugin status, it must write 502 instead of ever calling
// ctx.Writer.WriteHeader with the invalid code (gin panics -> bare 500 on
// an out-of-range WriteHeader call).
func TestWriteWebhookResponse_OutOfRangePluginStatusReturns502(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	ctrl := &Controller{svc: &Service{}, log: testLogger(t)}
	ctrl.writeWebhookResponse(ctx, &store.Record{}, &pluginsdk.WebhookResponse{Status: 0, Body: []byte("boom")})

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body=%s", rec.Code, rec.Body.String())
	}
}

// TestWriteWebhookResponse_ValidStatusRelaysHeadersAndBody proves the
// normal (in-range) path still relays the plugin's status, headers, and
// body verbatim.
func TestWriteWebhookResponse_ValidStatusRelaysHeadersAndBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	ctrl := &Controller{svc: &Service{}, log: testLogger(t)}
	ctrl.writeWebhookResponse(ctx, &store.Record{}, &pluginsdk.WebhookResponse{
		Status:  201,
		Headers: map[string]string{"X-Plugin": "slack"},
		Body:    []byte(`{"ok":true}`),
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if got := rec.Header().Get("X-Plugin"); got != "slack" {
		t.Fatalf("X-Plugin header = %q, want %q", got, "slack")
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Fatalf("body = %q, want %q", rec.Body.String(), `{"ok":true}`)
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
