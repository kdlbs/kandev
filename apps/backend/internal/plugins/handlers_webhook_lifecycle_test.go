package plugins

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type blockingWebhookInvoker struct {
	entered  chan struct{}
	proceed  chan struct{}
	response *pluginsdk.WebhookResponse
}

type recordingWebhookInvoker struct {
	calls    int
	response *pluginsdk.WebhookResponse
	err      error
}

func (i *recordingWebhookInvoker) InvokeWebhook(
	_ context.Context, _ string, _ *pluginsdk.WebhookRequest,
) (*pluginsdk.WebhookResponse, error) {
	i.calls++
	if i.response != nil || i.err != nil {
		return i.response, i.err
	}
	return &pluginsdk.WebhookResponse{Status: http.StatusOK}, nil
}

type gatedRequestBody struct {
	entered chan struct{}
	proceed chan struct{}
	once    sync.Once
	data    []byte
	offset  int
}

func (b *gatedRequestBody) Read(p []byte) (int, error) {
	b.once.Do(func() {
		close(b.entered)
		<-b.proceed
	})
	if b.offset == len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.offset:])
	b.offset += n
	return n, nil
}

func (*gatedRequestBody) Close() error { return nil }

func (i *blockingWebhookInvoker) InvokeWebhook(
	_ context.Context, _ string, _ *pluginsdk.WebhookRequest,
) (*pluginsdk.WebhookResponse, error) {
	close(i.entered)
	<-i.proceed
	return i.response, nil
}

type blockingAuthLoginBridge struct {
	entered chan struct{}
	proceed chan struct{}
}

func (b *blockingAuthLoginBridge) SessionCookieName() string { return "kandev_session" }

func (b *blockingAuthLoginBridge) LoginExternal(
	c *gin.Context, _, _, _, _ string,
) error {
	close(b.entered)
	<-b.proceed
	c.SetCookie("kandev_session", "minted-by-generation-a", 3600, "/", "", false, true)
	return nil
}

func lifecycleWebhookPackage(t *testing.T, id, version, access string, authCapability bool) *bytes.Buffer {
	t.Helper()
	capabilities := ""
	if authCapability {
		capabilities = "capabilities:\n  auth: true\n"
	}
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 2
version: %q
display_name: Lifecycle Plugin
%swebhooks:
  - key: callback
    access: %s
runtime:
  type: binary
  executables:
    %s-%s: server/plugin
`, id, version, capabilities, access, runtime.GOOS, runtime.GOARCH)
	var buf bytes.Buffer
	if err := pkgtartest.WritePackage(&buf, map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	return &buf
}

func observedWebhookLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	zapLogger := zap.New(core)
	wrapped, err := logger.NewFromZap(zapLogger)
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	t.Cleanup(func() { _ = zapLogger.Sync() })
	return wrapped, logs
}

func TestWebhookFailureOriginLogsSafeFields(t *testing.T) {
	for _, test := range []struct {
		name       string
		origin     string
		class      string
		response   *pluginsdk.WebhookResponse
		invokeErr  error
		disable    bool
		wantStatus int
		wantBody   string
	}{
		{
			name:       "host lifecycle",
			origin:     "host_lifecycle",
			class:      "host_error",
			disable:    true,
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `"error":"plugins: plugin \"kandev-plugin-lifecycle-log\" lifecycle generation is no longer active"`,
		},
		{
			name:       "host RPC",
			origin:     "host_rpc",
			class:      "host_error",
			invokeErr:  fmt.Errorf("rpc failed with rpc-secret-sentinel"),
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `"error":"rpc failed with rpc-secret-sentinel"`,
		},
		{
			name:       "plugin response",
			origin:     "plugin_response",
			response:   &pluginsdk.WebhookResponse{Status: http.StatusServiceUnavailable, Body: []byte("plugin-response-secret-sentinel")},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "plugin-response-secret-sentinel",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			const id = "kandev-plugin-lifecycle-log"
			svc, _, _ := newTestService(t)
			if _, err := svc.Install(t.Context(), lifecycleWebhookPackage(t, id, "1.0.0", "public", false)); err != nil {
				t.Fatalf("Install: %v", err)
			}
			if test.disable {
				if err := svc.Disable(id); err != nil {
					t.Fatalf("Disable: %v", err)
				}
			}

			log, logs := observedWebhookLogger(t)
			invoker := &recordingWebhookInvoker{response: test.response, err: test.invokeErr}
			ctrl := &Controller{svc: svc, log: log, webhookInvoker: invoker}
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/api/plugins/:id/webhooks/:key", ctrl.webhook)

			requestBody := "request-body-secret-sentinel"
			request := httptest.NewRequest(http.MethodPost,
				"/api/plugins/"+id+"/webhooks/callback?token=query-secret-sentinel",
				strings.NewReader(requestBody),
			)
			request.Header.Set("X-Request-Secret", "header-secret-sentinel")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), test.wantBody) {
				t.Fatalf("body = %q, want it to contain %q", recorder.Body.String(), test.wantBody)
			}
			if test.disable && invoker.calls != 0 {
				t.Fatalf("invoker calls = %d, want 0 after lifecycle lease failure", invoker.calls)
			}
			if !test.disable && invoker.calls != 1 {
				t.Fatalf("invoker calls = %d, want 1", invoker.calls)
			}

			entries := logs.FilterMessage("plugin webhook failed").All()
			if len(entries) != 1 {
				t.Fatalf("webhook failure logs = %d, want 1", len(entries))
			}
			fields := entries[0].ContextMap()
			if got := fields["plugin_id"]; got != id {
				t.Fatalf("plugin_id = %v, want %q", got, id)
			}
			if got := fmt.Sprint(fields["status"]); got != fmt.Sprint(test.wantStatus) {
				t.Fatalf("status field = %v, want %d", got, test.wantStatus)
			}
			if got := fields["origin"]; got != test.origin {
				t.Fatalf("origin = %v, want %q", got, test.origin)
			}
			if test.class == "" {
				if _, ok := fields["error_class"]; ok {
					t.Fatalf("plugin response was labeled as a host error: %#v", fields["error_class"])
				}
			} else if got := fields["error_class"]; got != test.class {
				t.Fatalf("error_class = %v, want %q", got, test.class)
			}
			logged := fmt.Sprint(fields)
			for _, secret := range []string{
				"request-body-secret-sentinel", "query-secret-sentinel", "header-secret-sentinel",
				"rpc-secret-sentinel", "plugin-response-secret-sentinel",
			} {
				if strings.Contains(logged, secret) {
					t.Fatalf("webhook diagnostic exposed %q: %s", secret, logged)
				}
			}
			if _, ok := fields["error"]; ok {
				t.Fatalf("webhook diagnostic contained a raw error field: %s", logged)
			}
		})
	}
}

func TestWebhookFailureLogsWaitForAuthorizationAndDeclaration(t *testing.T) {
	const id = "kandev-plugin-private-log"
	svc, _, _ := newTestService(t)
	if _, err := svc.Install(t.Context(), lifecycleWebhookPackage(t, id, "1.0.0", "authenticated", false)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	log, logs := observedWebhookLogger(t)
	ctrl := &Controller{svc: svc, log: log, webhookInvoker: &recordingWebhookInvoker{}}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/plugins/:id/webhooks/:key", ctrl.webhook)

	unauthorized := doRequest(router, http.MethodPost, "/api/plugins/"+id+"/webhooks/callback", "{}", nil)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401", unauthorized.Code)
	}
	undeclared := doAuthedRequest(router, http.MethodPost, "/api/plugins/"+id+"/webhooks/unknown", "{}", nil)
	if undeclared.Code != http.StatusNotFound {
		t.Fatalf("undeclared status = %d, want 404", undeclared.Code)
	}
	if entries := logs.FilterMessage("plugin webhook failed").All(); len(entries) != 0 {
		t.Fatalf("pre-dispatch requests emitted webhook failure diagnostics: %+v", entries)
	}
}

func TestWebhookDispatchLeaseCoversRPCAndAuthLoginResponse(t *testing.T) {
	const id = "kandev-plugin-generation"
	svc, _, _ := newTestService(t)
	if _, err := svc.Install(t.Context(), lifecycleWebhookPackage(t, id, "1.0.0", "public", true)); err != nil {
		t.Fatalf("Install generation A: %v", err)
	}

	invoker := &blockingWebhookInvoker{
		entered: make(chan struct{}),
		proceed: make(chan struct{}),
		response: &pluginsdk.WebhookResponse{Status: http.StatusFound, Headers: map[string]string{
			authLoginHeader: authAssertionHeader(),
		}},
	}
	bridge := &blockingAuthLoginBridge{entered: make(chan struct{}), proceed: make(chan struct{})}
	svc.SetAuthLoginBridge(bridge)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := &Controller{svc: svc, log: testLogger(t), webhookInvoker: invoker}
	router.GET("/api/plugins/:id/webhooks/:key", ctrl.webhook)

	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response <- doRequest(router, http.MethodGet, "/api/plugins/"+id+"/webhooks/callback", "", nil)
	}()
	select {
	case <-invoker.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("generation A webhook did not enter the plugin RPC")
	}

	replacement := lifecycleWebhookPackage(t, id, "2.0.0", "authenticated", false)
	upgrade := make(chan error, 1)
	go func() {
		_, err := svc.Install(t.Context(), replacement)
		upgrade <- err
	}()
	select {
	case err := <-upgrade:
		t.Fatalf("generation B installed while generation A RPC was active: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(invoker.proceed)
	select {
	case <-bridge.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("generation A response did not reach the auth login bridge")
	}
	select {
	case err := <-upgrade:
		t.Fatalf("generation B installed while generation A auth response was active: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(bridge.proceed)
	select {
	case rec := <-response:
		if rec.Code != http.StatusFound {
			t.Fatalf("webhook status = %d, want 302, body=%s", rec.Code, rec.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("generation A webhook did not finish")
	}
	select {
	case err := <-upgrade:
		if err != nil {
			t.Fatalf("Install generation B: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("generation B remained blocked after the response completed")
	}
	current, err := svc.Get(id)
	if err != nil || current.Version != "2.0.0" || current.Capabilities.Auth {
		t.Fatalf("current generation = %+v, err=%v, want non-auth generation 2.0.0", current, err)
	}
}

func TestWebhookGenerationChangeAfterAuthorizationDoesNotInvokeReplacement(t *testing.T) {
	const id = "kandev-plugin-generation-race"
	svc, _, _ := newTestService(t)
	if _, err := svc.Install(t.Context(), lifecycleWebhookPackage(t, id, "1.0.0", "public", false)); err != nil {
		t.Fatalf("Install public generation A: %v", err)
	}

	invoker := &recordingWebhookInvoker{}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := &Controller{svc: svc, log: testLogger(t), webhookInvoker: invoker}
	router.POST("/api/plugins/:id/webhooks/:key", ctrl.webhook)
	body := &gatedRequestBody{
		entered: make(chan struct{}),
		proceed: make(chan struct{}),
		data:    []byte("{}"),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/plugins/"+id+"/webhooks/callback", nil)
	req.Body = body
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req)
		close(done)
	}()
	select {
	case <-body.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("generation A request did not reach body read after authorization")
	}

	if _, err := svc.Install(t.Context(), lifecycleWebhookPackage(t, id, "2.0.0", "authenticated", false)); err != nil {
		t.Fatalf("Install authenticated generation B: %v", err)
	}
	close(body.proceed)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("generation A request did not finish after body release")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 for stale generation, body=%s", rec.Code, rec.Body.String())
	}
	if invoker.calls != 0 {
		t.Fatalf("replacement invocations = %d, want 0", invoker.calls)
	}
}
