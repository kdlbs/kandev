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

	"github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// testBootToken is the per-boot operator token the test routers register with;
// doRequest / doMultipartInstall attach it in the X-Kandev-Boot-Token header so
// the guarded state-changing routes accept them. Tests that assert the guard
// itself build their own requests without the header.
const testBootToken = "test-boot-token"

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
	RegisterRoutes(router, svc, nil, testLogger(t), testBootToken)
	return router, svc
}

func doRequest(router *gin.Engine, method, path string, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(httpmw.BootTokenHeader, testBootToken)
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
	req.Header.Set(httpmw.BootTokenHeader, testBootToken)
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

func TestWebhookHandlerUnknownPluginReturns404(t *testing.T) {
	router, _ := newTestRouter(t)
	rec := doRequest(router, http.MethodPost, "/api/plugins/missing/webhooks/key1", "{}", nil)
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

	rec := doRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-slack/webhooks/undeclared-key", "{}", nil)
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

	oversized := strings.Repeat("a", maxWebhookBodyBytes+1)
	rec := doRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-slack/webhooks/key1", oversized, nil)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413, body=%s", rec.Code, rec.Body.String())
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

	rec := doRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-slack/webhooks/key1", "{}", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body=%s", rec.Code, rec.Body.String())
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

	writeWebhookResponse(ctx, &pluginsdk.WebhookResponse{Status: 0, Body: []byte("boom")})

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

	writeWebhookResponse(ctx, &pluginsdk.WebhookResponse{
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

// doMultipartInstallNoToken posts a multipart install WITHOUT the operator
// boot-token header — the shape of a cross-origin CSRF drive-by or an
// unauthenticated LAN caller.
func doMultipartInstallNoToken(t *testing.T, router *gin.Engine, pkg *bytes.Buffer) *httptest.ResponseRecorder {
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

// TestInstallHandlerMultipartRejectsMissingBootToken is the C1 regression: a
// cross-origin multipart POST /api/plugins/install with no operator token must
// be rejected with 403 AND must not execute the install (no plugin spawned).
func TestInstallHandlerMultipartRejectsMissingBootToken(t *testing.T) {
	router, svc := newTestRouter(t)

	rec := doMultipartInstallNoToken(t, router, testPackage(t, "kandev-plugin-slack", "1.0.0", false))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
	if len(svc.List()) != 0 {
		t.Fatalf("install executed despite missing token: %d plugins registered", len(svc.List()))
	}
	if _, err := svc.Get("kandev-plugin-slack"); err == nil {
		t.Fatal("plugin was installed despite missing operator token")
	}
}

// TestStateChangingRoutesRejectMissingBootToken pins that every gated
// state-changing route rejects a tokenless request with 403.
func TestStateChangingRoutesRejectMissingBootToken(t *testing.T) {
	router, _ := newTestRouter(t)

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/plugins/install", `{"url":"http://x"}`},
		{http.MethodPost, "/api/plugins/sync", ""},
		{http.MethodPatch, "/api/plugins/some-id", `{"config":{}}`},
		{http.MethodDelete, "/api/plugins/some-id", ""},
		{http.MethodPost, "/api/plugins/some-id/enable", ""},
		{http.MethodPost, "/api/plugins/some-id/disable", ""},
		{http.MethodPost, "/api/plugins/marketplace/refresh", ""},
		{http.MethodPost, "/api/plugins/marketplace/sources", `{"name":"x","url":"http://x"}`},
		{http.MethodPatch, "/api/plugins/marketplace/sources/sid", `{"enabled":false}`},
		{http.MethodDelete, "/api/plugins/marketplace/sources/sid", ""},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403 (body=%s)", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

// TestReadAndWebhookRoutesAllowedWithoutBootToken pins that the ungated
// surface — read-only management GETs, browser-native asset loads, and the
// external webhook relay — is NOT locked behind the operator token (a 403 here
// would break plugin UI loading and inbound webhooks).
func TestReadAndWebhookRoutesAllowedWithoutBootToken(t *testing.T) {
	router, _ := newTestRouter(t)

	// GET /api/plugins (list) must not require the token.
	list := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, list)
	if listRec.Code == http.StatusForbidden {
		t.Fatalf("GET /api/plugins wrongly gated by operator token")
	}

	// Webhook relay must reach the handler (404 unknown plugin), not 403.
	wh := httptest.NewRequest(http.MethodPost, "/api/plugins/missing/webhooks/key1", strings.NewReader("{}"))
	whRec := httptest.NewRecorder()
	router.ServeHTTP(whRec, wh)
	if whRec.Code == http.StatusForbidden {
		t.Fatalf("webhook relay wrongly gated by operator token; got 403")
	}
	if whRec.Code != http.StatusNotFound {
		t.Fatalf("webhook relay status = %d, want 404 (unknown plugin)", whRec.Code)
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
