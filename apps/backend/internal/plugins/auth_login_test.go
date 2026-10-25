package plugins

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

var errTestBridge = errors.New("bridge failed")

// fakeBridge records the LoginExternal call and optionally sets a session
// cookie / returns an error, standing in for the real auth-backed bridge.
type fakeBridge struct {
	called                            bool
	provider, subject, email, display string
	err                               error
	setCookie                         bool
}

func (b *fakeBridge) SessionCookieName() string { return "kandev_session" }

func (b *fakeBridge) LoginExternal(c *gin.Context, provider, subject, email, displayName string) error {
	b.called = true
	b.provider, b.subject, b.email, b.display = provider, subject, email, displayName
	if b.err != nil {
		return b.err
	}
	if b.setCookie {
		c.SetCookie("kandev_session", "minted-token", 3600, "/", "", false, true)
	}
	return nil
}

func authAssertionHeader() string {
	return `{"provider":"oidc:iss","subject":"sub-1","email":"a@b.dev","display_name":"Alice"}`
}

func newTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/api/plugins/p/webhooks/cb", nil)
	return c, rec
}

func authCapableRecord() *store.Record {
	return &store.Record{Manifest: manifest.Manifest{
		ID:           "p",
		Capabilities: manifest.Capabilities{Auth: true},
	}}
}

func TestTakeAuthLoginDirective(t *testing.T) {
	headers := map[string]string{
		"Location":            "/",
		"x-kandev-auth-login": "payload", // case-insensitive match
	}
	raw, present, ambiguous := takeAuthLoginDirective(headers)
	if !present || ambiguous || raw != "payload" {
		t.Fatalf("directive = %q present=%v ambiguous=%v", raw, present, ambiguous)
	}
	if _, stillThere := headers["x-kandev-auth-login"]; stillThere {
		t.Fatal("directive header was not stripped")
	}
	if _, present, _ := takeAuthLoginDirective(map[string]string{"Location": "/"}); present {
		t.Fatal("no directive should report absent")
	}
}

func TestTakeAuthLoginDirectiveMultipleVariantsAmbiguous(t *testing.T) {
	// Two case variants are distinct Go map keys; picking one is nondeterministic
	// and would leak/relay the other. Must be reported ambiguous with BOTH stripped.
	headers := map[string]string{
		"X-Kandev-Auth-Login": "a",
		"x-kandev-auth-login": "b",
		"Location":            "/",
	}
	_, present, ambiguous := takeAuthLoginDirective(headers)
	if !present || !ambiguous {
		t.Fatalf("present=%v ambiguous=%v, want both true", present, ambiguous)
	}
	for k := range headers {
		if k != "Location" {
			t.Fatalf("directive variant %q not stripped", k)
		}
	}
}

func TestWriteWebhookResponseAuthLoginSuccess(t *testing.T) {
	bridge := &fakeBridge{setCookie: true}
	svc := &Service{}
	svc.SetAuthLoginBridge(bridge)
	ctrl := &Controller{svc: svc, log: testLogger(t)}

	c, rec := newTestContext(t)
	resp := &pluginsdk.WebhookResponse{
		Status: 302,
		Headers: map[string]string{
			"Location":      "/app",
			authLoginHeader: authAssertionHeader(),
			"Set-Cookie":    "evil=1", // plugin-set cookies must be dropped
		},
		Body: []byte("redirecting"),
	}
	ctrl.writeWebhookResponse(c, authCapableRecord(), resp)

	if rec.Code != 302 {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if !bridge.called || bridge.provider != "oidc:iss" || bridge.subject != "sub-1" ||
		bridge.email != "a@b.dev" || bridge.display != "Alice" {
		t.Fatalf("bridge not invoked with parsed assertion: %+v", bridge)
	}
	if rec.Header().Get("Location") != "/app" {
		t.Fatalf("Location not relayed: %q", rec.Header().Get("Location"))
	}
	if rec.Header().Get(authLoginHeader) != "" {
		t.Fatal("auth directive header leaked to the browser")
	}
	setCookies := strings.Join(rec.Header().Values("Set-Cookie"), "; ")
	if !strings.Contains(setCookies, "kandev_session=minted-token") {
		t.Fatalf("host session cookie missing: %q", setCookies)
	}
	if strings.Contains(setCookies, "evil=1") {
		t.Fatalf("plugin-set cookie was relayed: %q", setCookies)
	}
}

func TestWriteWebhookResponseAmbiguousDirectiveRejected(t *testing.T) {
	bridge := &fakeBridge{}
	svc := &Service{}
	svc.SetAuthLoginBridge(bridge)
	ctrl := &Controller{svc: svc, log: testLogger(t)}

	c, rec := newTestContext(t)
	resp := &pluginsdk.WebhookResponse{
		Status: 302,
		Headers: map[string]string{
			"X-Kandev-Auth-Login": authAssertionHeader(),
			"x-kandev-auth-login": `{"provider":"evil","subject":"x","email":"e@x.dev"}`,
		},
	}
	ctrl.writeWebhookResponse(c, authCapableRecord(), resp)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 for ambiguous directive", rec.Code)
	}
	if bridge.called {
		t.Fatal("bridge must not be called on an ambiguous directive")
	}
	if rec.Header().Get(authLoginHeader) != "" {
		t.Fatal("a directive variant leaked to the browser")
	}
}

func TestWriteWebhookResponseAuthLoginRequiresCapability(t *testing.T) {
	bridge := &fakeBridge{}
	svc := &Service{}
	svc.SetAuthLoginBridge(bridge)
	ctrl := &Controller{svc: svc, log: testLogger(t)}

	c, rec := newTestContext(t)
	record := &store.Record{Manifest: manifest.Manifest{ID: "p"}} // Capabilities.Auth == false
	resp := &pluginsdk.WebhookResponse{
		Status:  302,
		Headers: map[string]string{authLoginHeader: authAssertionHeader()},
	}
	ctrl.writeWebhookResponse(c, record, resp)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if bridge.called {
		t.Fatal("bridge must not be called without the auth capability")
	}
}

func TestWriteWebhookResponseAuthLoginNoBridge(t *testing.T) {
	ctrl := &Controller{svc: &Service{}, log: testLogger(t)} // no bridge wired (auth off)

	c, rec := newTestContext(t)
	resp := &pluginsdk.WebhookResponse{
		Status:  302,
		Headers: map[string]string{authLoginHeader: authAssertionHeader()},
	}
	ctrl.writeWebhookResponse(c, authCapableRecord(), resp)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 when auth is not enabled", rec.Code)
	}
}

func TestWriteWebhookResponseBridgeError(t *testing.T) {
	bridge := &fakeBridge{err: errTestBridge}
	svc := &Service{}
	svc.SetAuthLoginBridge(bridge)
	ctrl := &Controller{svc: svc, log: testLogger(t)}

	c, rec := newTestContext(t)
	resp := &pluginsdk.WebhookResponse{
		Status:  302,
		Headers: map[string]string{authLoginHeader: authAssertionHeader()},
	}
	ctrl.writeWebhookResponse(c, authCapableRecord(), resp)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 on bridge error", rec.Code)
	}
	if !bridge.called {
		t.Fatal("bridge should have been called")
	}
	// The raw bridge/store error must not be disclosed to the (public) caller.
	if strings.Contains(rec.Body.String(), errTestBridge.Error()) {
		t.Fatalf("raw error leaked to caller: %s", rec.Body.String())
	}
}

func TestWriteWebhookResponseNoDirectiveRelayedVerbatim(t *testing.T) {
	ctrl := &Controller{svc: &Service{}, log: testLogger(t)}

	c, rec := newTestContext(t)
	resp := &pluginsdk.WebhookResponse{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    []byte(`{"ok":true}`),
	}
	ctrl.writeWebhookResponse(c, authCapableRecord(), resp)

	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" || rec.Body.String() != `{"ok":true}` {
		t.Fatalf("plain webhook not relayed: code=%d ct=%q body=%q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}
