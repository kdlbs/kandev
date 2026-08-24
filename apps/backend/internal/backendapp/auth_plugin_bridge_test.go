package backendapp

import "testing"

// TestPluginSSOBridgeSessionCookieName pins the production end of the seam
// internal/plugins.Service.sessionCookieName() reads: the webhook relay strips
// Kandev's session cookie from the headers it forwards to a plugin subprocess,
// and stripSessionCookie skips that work entirely when the name is empty. A
// bridge returning "" would therefore hand every authenticated caller's session
// cookie to the plugin while every flattenHeaders unit test — which passes the
// name in directly — still passed.
func TestPluginSSOBridgeSessionCookieName(t *testing.T) {
	svc := newSSOTestAuthService(t)
	bridge := pluginSSOBridge{auth: svc}

	got := bridge.SessionCookieName()
	if got == "" {
		t.Fatal("SessionCookieName() is empty; the webhook relay would forward the caller's session cookie to the plugin subprocess")
	}
	if want := svc.CookieName(); got != want {
		t.Fatalf("SessionCookieName() = %q, want the auth service's cookie name %q", got, want)
	}
}
