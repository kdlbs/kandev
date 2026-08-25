// handlers_webhook_auth_test.go covers the plugin webhook relay's auth gate
// (AC1-AC5, AC9 of the "require auth for plugin webhooks unless the manifest
// declares them public" plan). It lives in its own file per
// apps/backend/AGENTS.md's revive 800-effective-line file cap:
// handlers_test.go is already close to that limit.
//
// Test technique: PluginRuntime.Get returns a concrete *pluginsdk.RemotePlugin
// (see internal/plugins/types.go), which fakeRuntime (service_test.go) can
// only stand in for as a nil pointer when "running" — calling HandleWebhook
// on it would panic (nil pointer dereference), so these tests never leave a
// plugin running. Instead they reuse the same technique
// TestWebhookHandlerNotRunningReturns503 (handlers_test.go) already
// establishes: with the plugin installed but not running, a request that
// passes the auth gate reaches Service.InvokeWebhook and gets a clean 503
// "plugin ... is not running" — proof the gate let it through to the relay
// path without needing a live subprocess. A request the gate blocks never
// gets that far and returns 401 instead. 401 vs 503 is therefore the
// discriminator these tests assert on, exactly matching "HandleWebhook is
// never invoked" (AC1) / "reaches the subprocess" (AC2, AC3, AC4) without
// risking the nil-pointer panic.
package plugins

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	goruntime "runtime"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
)

// installedWebhookPlugin installs a plugin declaring exactly one webhook
// (key, with the given access policy), then disables it so the runtime never
// reports it running — see the file doc comment for why.
func installedWebhookPlugin(t *testing.T, svc *Service, id, key string, public bool) {
	t.Helper()
	platformKey := goruntime.GOOS + "-" + goruntime.GOARCH
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 2
version: "1.0.0"
display_name: Test Plugin
webhooks:
  - key: %s
    method: POST
    access: %s
runtime:
  type: binary
  executables:
    %s: server/plugin
`, id, key, webhookAccessForTest(public), platformKey)
	var buf bytes.Buffer
	files := map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}
	if err := pkgtartest.WritePackage(&buf, files); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	if _, err := svc.Install(t.Context(), &buf); err != nil {
		t.Fatalf("Install(%q): %v", id, err)
	}
	if err := svc.Disable(id); err != nil {
		t.Fatalf("Disable(%q): %v", id, err)
	}
}

func webhookAccessForTest(public bool) string {
	if public {
		return "public"
	}
	return "authenticated"
}

// reachedRelay reports whether a webhook response proves the request reached
// Service.InvokeWebhook (503 "not running") as opposed to being blocked by
// the auth gate (401).
func reachedRelay(code int) bool { return code == http.StatusServiceUnavailable }

// TestWebhookAuthGate_AnonymousNonPublicBlocked pins AC1: an anonymous
// caller against a webhook without public: true gets 401 with
// WWW-Authenticate: Bearer, and never reaches the relay.
func TestWebhookAuthGate_AnonymousNonPublicBlocked(t *testing.T) {
	router, svc := newTestRouter(t)
	installedWebhookPlugin(t, svc, "kandev-plugin-private", "key1", false)

	rec := doRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-private/webhooks/key1", "{}", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Fatalf("WWW-Authenticate = %q, want %q", got, "Bearer")
	}
	if reachedRelay(rec.Code) {
		t.Fatal("request reached the relay path; HandleWebhook must never be invoked for an anonymous, non-public webhook")
	}
}

// TestWebhookAuthGate_AnonymousPublicReachesRelay pins AC2: an anonymous
// caller against a webhook declaring public: true passes the gate and
// reaches the relay (503 "not running", not 401 "authentication required").
func TestWebhookAuthGate_AnonymousPublicReachesRelay(t *testing.T) {
	router, svc := newTestRouter(t)
	installedWebhookPlugin(t, svc, "kandev-plugin-public", "key1", true)

	rec := doRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-public/webhooks/key1", "{}", nil)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("public webhook was blocked by the auth gate: %d body=%s", rec.Code, rec.Body.String())
	}
	if !reachedRelay(rec.Code) {
		t.Fatalf("status = %d, want 503 (reached relay), body=%s", rec.Code, rec.Body.String())
	}
}

// TestWebhookAuthGate_AuthenticatedNonPublicReachesRelay pins AC3: a caller
// carrying a real request identity (session/PAT, modeled here via the same
// context-injection doAuthedRequest uses) reaches the relay for a non-public
// webhook.
func TestWebhookAuthGate_AuthenticatedNonPublicReachesRelay(t *testing.T) {
	router, svc := newTestRouter(t)
	installedWebhookPlugin(t, svc, "kandev-plugin-private2", "key1", false)

	rec := doAuthedRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-private2/webhooks/key1", "{}", nil)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("authenticated caller was blocked by the auth gate: %d body=%s", rec.Code, rec.Body.String())
	}
	if !reachedRelay(rec.Code) {
		t.Fatalf("status = %d, want 503 (reached relay), body=%s", rec.Code, rec.Body.String())
	}
}

// browserSignals is the pair of headers the session CSRF gate reads. A zero
// value models a request carrying neither: a non-browser client replaying a
// session cookie, or a browser predating Fetch Metadata.
type browserSignals struct {
	origin       string
	secFetchSite string
}

func doWebhookIdentityRequest(
	router http.Handler, method, path string, identity authn.Identity, signals browserSignals,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	if signals.origin != "" {
		req.Header.Set("Origin", signals.origin)
	}
	if signals.secFetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", signals.secFetchSite)
	}
	req = req.WithContext(authn.WithIdentity(req.Context(), identity))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestWebhookAuthGate_SessionCSRFSignals is the table over
// (method, Origin, Sec-Fetch-Site, identity) for the session CSRF gate.
//
// The gate exists because a session identity is carried by an ambient cookie,
// so a cross-site page can make the browser attach it. It must therefore
// admit a genuine same-origin request and refuse a cross-site one. The
// discriminating signal is *origin*, never the verb: a plugin's GET webhook
// may have side effects and the host cannot verify that it does not, so
// exempting safe methods would reopen the hole this gate closes.
//
// Sec-Fetch-Site is the signal for the case Origin cannot answer. Browsers do
// not send Origin on a same-origin GET/HEAD (Fetch, "append a request Origin
// header": Origin is attached to cross-origin requests and to same-origin
// requests whose method is neither GET nor HEAD), so an absent Origin is the
// *normal* state of the SPA polling its own plugin webhook, not evidence of a
// cross-origin caller.
func TestWebhookAuthGate_SessionCSRFSignals(t *testing.T) {
	router, svc := newTestRouter(t)
	installedWebhookPlugin(t, svc, "kandev-plugin-session-origin", "key1", false)
	session := authn.Identity{UserID: "user-1", Role: authn.RoleMember, SessionID: "session-1"}
	pat := authn.Identity{UserID: "user-1", Role: authn.RoleMember, TokenID: "token-1"}
	path := "/api/plugins/kandev-plugin-session-origin/webhooks/key1"

	const (
		blocked = http.StatusForbidden
		relayed = http.StatusServiceUnavailable // reached the relay; see reachedRelay
	)

	for _, tc := range []struct {
		name     string
		method   string
		identity authn.Identity
		signals  browserSignals
		want     int
	}{
		// The case broken in production: a plugin panel polling its own
		// webhook same-origin with GET. No Origin is sent, and before this
		// gate learned to read Sec-Fetch-Site every such poll was refused 403
		// on any auth-enabled instance.
		{
			name: "same-origin GET without Origin", method: http.MethodGet, identity: session,
			signals: browserSignals{secFetchSite: "same-origin"}, want: relayed,
		},
		{
			name: "same-origin POST without Origin", method: http.MethodPost, identity: session,
			signals: browserSignals{secFetchSite: "same-origin"}, want: relayed,
		},
		// Header values are lowercase per the Fetch Metadata spec; matching
		// case-insensitively costs nothing and cannot admit a new value.
		{
			name: "same-origin spelled with mixed case", method: http.MethodGet, identity: session,
			signals: browserSignals{secFetchSite: "Same-Origin"}, want: relayed,
		},
		// Sec-Fetch-Site: none is a user-initiated request with no initiator
		// (address bar, bookmark). A cross-site page cannot cause a request
		// to be labelled none, so it is not a CSRF vector.
		{
			name: "user-initiated navigation", method: http.MethodGet, identity: session,
			signals: browserSignals{secFetchSite: "none"}, want: relayed,
		},
		// A cross-site top-level GET navigation sends no Origin, and the
		// SameSite=Lax session cookie *is* attached to it — this is the one
		// ambient-credential vector left on this route, and the gate refuses
		// it on the fetch-metadata signal alone.
		{
			name: "cross-site without Origin", method: http.MethodGet, identity: session,
			signals: browserSignals{secFetchSite: "cross-site"}, want: blocked,
		},
		{
			name: "cross-site POST without Origin", method: http.MethodPost, identity: session,
			signals: browserSignals{secFetchSite: "cross-site"}, want: blocked,
		},
		// same-site (a sibling subdomain, or the same host on another port)
		// is refused too: SameSite=Lax does not stop it, and no legitimate
		// caller reaches this route as same-site without also sending Origin
		// (see the split-origin case below).
		{
			name: "same-site without Origin", method: http.MethodGet, identity: session,
			signals: browserSignals{secFetchSite: "same-site"}, want: blocked,
		},
		// Origin present keeps deciding on its own: a cross-origin fetch
		// always carries Origin, and httpmw.AllowedOrigin is the single
		// shared origin trust policy.
		{
			name: "foreign Origin", method: http.MethodPost, identity: session,
			signals: browserSignals{origin: "https://attacker.example"}, want: blocked,
		},
		{
			name: "foreign Origin claiming same-origin", method: http.MethodGet, identity: session,
			signals: browserSignals{origin: "https://attacker.example", secFetchSite: "same-origin"},
			want:    blocked,
		},
		{
			name: "accepted Origin", method: http.MethodPost, identity: session,
			signals: browserSignals{origin: "https://example.com"}, want: relayed,
		},
		// The split-origin deployment host-api.ts documents: the SPA on one
		// port fetching the backend on another. That is cross-origin, so the
		// browser sends Origin (which AllowedOrigin accepts — it ignores
		// ports) and labels it same-site. Deciding on Origin first is what
		// keeps this working; demanding same-origin whenever the header is
		// present would break every split-origin and desktop install.
		{
			name: "accepted Origin on another port", method: http.MethodGet, identity: session,
			signals: browserSignals{origin: "https://example.com:1420", secFetchSite: "same-site"},
			want:    relayed,
		},
		// Neither signal: refused. An ambient session cookie with no origin
		// evidence is indistinguishable from a cross-site top-level GET
		// navigation by a browser predating Fetch Metadata, which
		// SameSite=Lax does attach the cookie to. A deliberate non-browser
		// caller uses a PAT, which is not gated at all (next case).
		{
			name: "no browser signals at all", method: http.MethodGet, identity: session,
			signals: browserSignals{}, want: blocked,
		},
		// The gate is scoped to ambient-credential identities. A PAT is sent
		// deliberately by its holder, so a cross-site page cannot borrow it
		// and the fetch-metadata signal is irrelevant.
		{
			name: "PAT identity is not gated", method: http.MethodGet, identity: pat,
			signals: browserSignals{secFetchSite: "cross-site"}, want: relayed,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doWebhookIdentityRequest(router, tc.method, path, tc.identity, tc.signals)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestWebhookAuthGate_NonBrowserIdentitiesDoNotRequireOrigin(t *testing.T) {
	router, svc := newTestRouter(t)
	installedWebhookPlugin(t, svc, "kandev-plugin-non-browser", "key1", false)
	path := "/api/plugins/kandev-plugin-non-browser/webhooks/key1"
	identities := []authn.Identity{
		{UserID: "user-1", Role: authn.RoleMember, TokenID: "token-1"},
		{UserID: "default", Role: authn.RoleAdmin, Synthetic: true},
	}
	for _, identity := range identities {
		rec := doWebhookIdentityRequest(router, http.MethodPost, path, identity, browserSignals{})
		if !reachedRelay(rec.Code) {
			t.Fatalf("identity %+v: status = %d, want 503, body=%s", identity, rec.Code, rec.Body.String())
		}
	}
}

// TestWebhookAuthGate_AuthDisabledReachesRelayRegardlessOfPublic pins AC4:
// with auth disabled, httpmw.Middleware injects a synthetic identity onto
// every request, so both public and non-public webhooks reach the relay —
// this is the no-regression pin for the default (auth-off) configuration.
// This handler test wires no auth middleware at all (see newTestRouter), so
// "auth disabled" is modeled the same way httpmw.Middleware's disabled path
// behaves: a request identity is always present. doAuthedRequest supplies
// that identity; a plain doRequest (used above for the anonymous AC1/AC2
// cases) supplies none, matching what httpmw.Middleware does only once auth
// is enabled.
func TestWebhookAuthGate_AuthDisabledReachesRelayRegardlessOfPublic(t *testing.T) {
	router, svc := newTestRouter(t)
	installedWebhookPlugin(t, svc, "kandev-plugin-priv3", "key1", false)
	installedWebhookPlugin(t, svc, "kandev-plugin-pub3", "key1", true)

	for _, id := range []string{"kandev-plugin-priv3", "kandev-plugin-pub3"} {
		rec := doAuthedRequest(router, http.MethodPost, "/api/plugins/"+id+"/webhooks/key1", "{}", nil)
		if !reachedRelay(rec.Code) {
			t.Fatalf("%s: status = %d, want 503 (reached relay), body=%s", id, rec.Code, rec.Body.String())
		}
	}
}

// TestWebhookAuthGate_AnonymousUnknownPluginReturns401 pins AC5's first case:
// an anonymous caller against an unknown plugin ID gets 401, not 404 — a
// distinct status here would let an anonymous caller enumerate installed
// plugins.
func TestWebhookAuthGate_AnonymousUnknownPluginReturns401(t *testing.T) {
	router, _ := newTestRouter(t)
	rec := doRequest(router, http.MethodPost, "/api/plugins/does-not-exist/webhooks/key1", "{}", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body.String())
	}
}

// TestWebhookAuthGate_AnonymousUndeclaredKeyReturns401 pins AC5's second
// case: an anonymous caller against an installed plugin but an undeclared
// webhook key gets 401, not 404.
func TestWebhookAuthGate_AnonymousUndeclaredKeyReturns401(t *testing.T) {
	router, svc := newTestRouter(t)
	installedWebhookPlugin(t, svc, "kandev-plugin-declared", "key1", false)

	rec := doRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-declared/webhooks/undeclared", "{}", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body.String())
	}
}

// TestWebhookAuthGate_AnonymousNonPublicKeyReturns401 pins AC5's third case:
// an anonymous caller against a declared-but-not-public key gets 401, the
// same status as the unknown-plugin and undeclared-key cases above, so an
// anonymous caller cannot distinguish "no such plugin," "no such webhook,"
// and "webhook exists but requires auth."
func TestWebhookAuthGate_AnonymousNonPublicKeyReturns401(t *testing.T) {
	router, svc := newTestRouter(t)
	installedWebhookPlugin(t, svc, "kandev-plugin-declared2", "key1", false)

	rec := doRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-declared2/webhooks/key1", "{}", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body.String())
	}
}

// --- AC9: credential stripping ---

// TestFlattenHeaders_StripsKandevSessionCookie pins AC9: the Kandev session
// cookie is dropped from the Cookie header before relaying to a plugin
// subprocess, while a non-Kandev cookie in the same header survives.
func TestFlattenHeaders_StripsKandevSessionCookie(t *testing.T) {
	h := http.Header{}
	h.Set("Cookie", "kandev_session=secret-token; other=keep-me")

	out := flattenHeaders(h, "kandev_session", true)
	if got := out["Cookie"]; got != "other=keep-me" {
		t.Fatalf("Cookie = %q, want only the surviving non-Kandev cookie", got)
	}
}

// TestFlattenHeaders_OmitsCookieHeaderWhenNothingSurvives pins that the
// Cookie header is dropped entirely (not sent as an empty string) once the
// only cookie present was the Kandev session cookie.
func TestFlattenHeaders_OmitsCookieHeaderWhenNothingSurvives(t *testing.T) {
	h := http.Header{}
	h.Set("Cookie", "kandev_session=secret-token")

	out := flattenHeaders(h, "kandev_session", true)
	if _, present := out["Cookie"]; present {
		t.Fatalf("Cookie header present = %v, want omitted entirely", out["Cookie"])
	}
}

// TestFlattenHeaders_StripsPATBearerButKeepsOtherBearers pins AC9: a
// kandev_pat_* Authorization bearer is dropped, while a non-PAT bearer (a
// provider's own token, which a public webhook may need to verify) survives.
func TestFlattenHeaders_StripsPATBearerButKeepsOtherBearers(t *testing.T) {
	pat := http.Header{}
	pat.Set("Authorization", "Bearer kandev_pat_abc123_secret")
	if out := flattenHeaders(pat, "", true); out["Authorization"] != "" {
		t.Fatalf("Authorization = %q, want stripped PAT bearer", out["Authorization"])
	}

	provider := http.Header{}
	provider.Set("Authorization", "Bearer provider-issued-token")
	if out := flattenHeaders(provider, "", true); out["Authorization"] != "Bearer provider-issued-token" {
		t.Fatalf("Authorization = %q, want the non-PAT bearer relayed unchanged", out["Authorization"])
	}
}

// TestFlattenHeaders_StripsPATBearerCaseInsensitively pins that the bearer
// scheme is matched case-insensitively (RFC 9110): a caller sending
// "bearer kandev_pat_..." is not authenticated by httpmw.BearerToken, so on a
// public webhook the request is relayed — the PAT must still be stripped
// rather than handed to the plugin subprocess.
func TestFlattenHeaders_StripsPATBearerCaseInsensitively(t *testing.T) {
	for _, scheme := range []string{"bearer", "BEARER", "BeArEr"} {
		h := http.Header{}
		h.Set("Authorization", scheme+" kandev_pat_abc123_secret")
		if out := flattenHeaders(h, "", true); out["Authorization"] != "" {
			t.Fatalf("scheme %q: Authorization = %q, want stripped PAT bearer", scheme, out["Authorization"])
		}
	}

	// A bare PAT with no scheme at all is stripped too.
	bare := http.Header{}
	bare.Set("Authorization", "kandev_pat_abc123_secret")
	if out := flattenHeaders(bare, "", true); out["Authorization"] != "" {
		t.Fatalf("Authorization = %q, want stripped bare PAT", out["Authorization"])
	}
}

// TestFlattenHeaders_StripsSessionCookieFromRepeatedHeaders proves that a client
// cannot hide the Kandev session by splitting cookies over repeated fields.
func TestFlattenHeaders_StripsSessionCookieFromRepeatedHeaders(t *testing.T) {
	h := http.Header{}
	h.Add("Cookie", "other=keep-me")
	h.Add("Cookie", "kandev_session=secret-token")

	out := flattenHeaders(h, "kandev_session", true)
	if got := out["Cookie"]; got != "other=keep-me" {
		t.Fatalf("Cookie = %q, want only the non-Kandev cookie", got)
	}
}

// TestFlattenHeaders_StripsPATFromRepeatedAuthorizationHeaders proves that a
// Kandev PAT cannot be hidden behind a provider bearer in another field.
func TestFlattenHeaders_StripsPATFromRepeatedAuthorizationHeaders(t *testing.T) {
	h := http.Header{}
	h.Add("Authorization", "Bearer provider-issued-token")
	h.Add("Authorization", "Bearer kandev_pat_abc123_secret")

	if out := flattenHeaders(h, "", true); out["Authorization"] != "" {
		t.Fatalf("Authorization = %q, want omitted when any field carries a PAT", out["Authorization"])
	}
}

// TestServiceSessionCookieName_ReflectsWiredBridge pins the seam the
// flattenHeaders tests above cannot reach: they pass the cookie name in
// directly, so they stay green no matter what Service.sessionCookieName()
// actually resolves to at runtime. Because stripSessionCookie no-ops on an
// empty name, a wired bridge reporting "" would silently forward every
// authenticated caller's kandev_session cookie to the plugin subprocess.
func TestServiceSessionCookieName_ReflectsWiredBridge(t *testing.T) {
	_, svc := newTestRouter(t)

	if got := svc.sessionCookieName(); got != "" {
		t.Fatalf("sessionCookieName() = %q with no bridge wired, want %q", got, "")
	}

	svc.SetAuthLoginBridge(&fakeBridge{})
	if got := svc.sessionCookieName(); got != "kandev_session" {
		t.Fatalf("sessionCookieName() = %q after wiring the bridge, want the bridge's own name", got)
	}
}

// TestFlattenHeaders_NoSessionCookieNameSkipsStripping pins that an empty
// sessionCookieName (no auth bridge wired — auth disabled entirely, so no
// session cookie is ever minted) relays the Cookie header unchanged.
func TestFlattenHeaders_NoSessionCookieNameSkipsStripping(t *testing.T) {
	h := http.Header{}
	h.Set("Cookie", "a=1; b=2")

	out := flattenHeaders(h, "", true)
	if out["Cookie"] != "a=1; b=2" {
		t.Fatalf("Cookie = %q, want unchanged when no session cookie name is configured", out["Cookie"])
	}
}

func TestFlattenHeaders_AuthenticatedWebhookDropsAmbientCredentials(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer reverse-proxy-token")
	h.Set("Cookie", "CF_Authorization=ambient; _oauth2_proxy=ambient")
	h.Set("Content-Type", "application/json")

	out := flattenHeaders(h, "kandev_session", false)
	if _, present := out["Authorization"]; present {
		t.Fatalf("Authorization = %q, want omitted for authenticated webhook", out["Authorization"])
	}
	if _, present := out["Cookie"]; present {
		t.Fatalf("Cookie = %q, want omitted for authenticated webhook", out["Cookie"])
	}
	if got := out["Content-Type"]; got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}
