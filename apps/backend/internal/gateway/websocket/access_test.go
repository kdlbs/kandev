package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// connectionAuthResult captures what requireConnectionAuth did to one request:
// whether the chain continued, the identity the downstream handler saw, and the
// HTTP response.
type connectionAuthResult struct {
	recorder   *httptest.ResponseRecorder
	nextCalled bool
	identity   authn.Identity
	hasID      bool
}

// runConnectionAuth drives requireConnectionAuth through a real gin router so
// both halves of the contract are observable: c.Next() reaching the downstream
// handler, and AbortWithStatusJSON stopping the chain before it.
func runConnectionAuth(t *testing.T, policy AuthPolicy, query string, preset *authn.Identity) connectionAuthResult {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	gateway := NewGateway(log)
	gateway.SetAuthPolicy(policy)

	result := connectionAuthResult{recorder: httptest.NewRecorder()}
	router := gin.New()
	if preset != nil {
		identity := *preset
		router.Use(func(c *gin.Context) {
			authn.SetOnGin(c, identity)
			c.Next()
		})
	}
	router.GET("/ws", gateway.requireConnectionAuth(), func(c *gin.Context) {
		result.nextCalled = true
		result.identity, result.hasID = authn.FromGin(c)
		c.Status(http.StatusOK)
	})
	router.ServeHTTP(result.recorder, httptest.NewRequest(http.MethodGet, "/ws"+query, nil))
	return result
}

// TestRequireConnectionAuth covers the 401 gate in front of WebSocket upgrades
// and the VS Code / port proxy routes.
//
// Must-not-regress: the enforced-and-anonymous row. Inverting the enforcement
// check (`policy.Enforced()` instead of `!policy.Enforced()`) turns this
// middleware into a total auth bypass precisely when auth IS enforced, and
// nothing else in the suite notices.
func TestRequireConnectionAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const patToken = "kandev_pat_valid"
	sessionIdentity := authn.Identity{UserID: "user-session", Role: authn.RoleMember, SessionID: "sess-1"}
	tokenIdentity := authn.Identity{UserID: "user-pat", Role: authn.RoleMember, TokenID: "tok-1"}

	enforced := func() bool { return true }
	// resolvedToken is shared by the ResolveToken stubs below and reset at the
	// top of every subtest, so these subtests must stay sequential — do not add
	// t.Parallel() here without giving each case its own capture.
	var resolvedToken string
	resolveValid := func(_ context.Context, token string) (authn.Identity, bool) {
		resolvedToken = token
		return tokenIdentity, true
	}
	resolveReject := func(_ context.Context, token string) (authn.Identity, bool) {
		resolvedToken = token
		return authn.Identity{}, false
	}

	cases := []struct {
		name       string
		policy     AuthPolicy
		query      string
		preset     *authn.Identity
		wantStatus int
		wantNext   bool
		wantUserID string
		wantToken  string
	}{
		{
			name:       "no enforcement hook passes through",
			policy:     AuthPolicy{},
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "enforcement disabled passes through",
			policy:     AuthPolicy{Enforced: func() bool { return false }, ResolveToken: resolveValid},
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "enforced without identity or token is rejected",
			policy:     AuthPolicy{Enforced: enforced, ResolveToken: resolveValid},
			wantStatus: http.StatusUnauthorized,
			wantNext:   false,
		},
		{
			name:       "enforced with an already-resolved identity passes through",
			policy:     AuthPolicy{Enforced: enforced},
			preset:     &sessionIdentity,
			wantStatus: http.StatusOK,
			wantNext:   true,
			wantUserID: sessionIdentity.UserID,
		},
		{
			// Branch order matters: an established identity must win over a
			// query credential. Resolving ?token= first would let any caller
			// overwrite the identity the HTTP auth middleware already
			// authenticated — wantToken "" pins that ResolveToken never runs.
			name:       "enforced with an identity present ignores the query token",
			policy:     AuthPolicy{Enforced: enforced, ResolveToken: resolveValid},
			query:      "?token=" + patToken,
			preset:     &sessionIdentity,
			wantStatus: http.StatusOK,
			wantNext:   true,
			wantUserID: sessionIdentity.UserID,
			wantToken:  "",
		},
		{
			name:       "enforced with a valid query token passes through and sets the identity",
			policy:     AuthPolicy{Enforced: enforced, ResolveToken: resolveValid},
			query:      "?token=" + patToken,
			wantStatus: http.StatusOK,
			wantNext:   true,
			wantUserID: tokenIdentity.UserID,
			wantToken:  patToken,
		},
		{
			name:       "enforced with a rejected query token is rejected",
			policy:     AuthPolicy{Enforced: enforced, ResolveToken: resolveReject},
			query:      "?token=" + patToken,
			wantStatus: http.StatusUnauthorized,
			wantNext:   false,
			wantToken:  patToken,
		},
		{
			name:       "enforced with a query token but no resolver is rejected",
			policy:     AuthPolicy{Enforced: enforced},
			query:      "?token=" + patToken,
			wantStatus: http.StatusUnauthorized,
			wantNext:   false,
		},
		{
			name:       "enforced with an empty query token is rejected",
			policy:     AuthPolicy{Enforced: enforced, ResolveToken: resolveValid},
			query:      "?token=",
			wantStatus: http.StatusUnauthorized,
			wantNext:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolvedToken = ""
			got := runConnectionAuth(t, tc.policy, tc.query, tc.preset)

			if got.recorder.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", got.recorder.Code, tc.wantStatus, got.recorder.Body.String())
			}
			if got.nextCalled != tc.wantNext {
				t.Errorf("downstream handler called = %v, want %v", got.nextCalled, tc.wantNext)
			}
			if resolvedToken != tc.wantToken {
				t.Errorf("ResolveToken received %q, want %q", resolvedToken, tc.wantToken)
			}
			if tc.wantStatus == http.StatusUnauthorized {
				assertAuthRequiredBody(t, got.recorder)
				return
			}
			if tc.wantUserID != "" {
				if !got.hasID {
					t.Fatalf("downstream handler saw no identity, want user %q", tc.wantUserID)
				}
				if got.identity.UserID != tc.wantUserID {
					t.Errorf("identity user = %q, want %q", got.identity.UserID, tc.wantUserID)
				}
			}
		})
	}
}

func assertAuthRequiredBody(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("401 body is not JSON (%q): %v", rec.Body.String(), err)
	}
	if body["error"] != "authentication required" {
		t.Errorf("401 body error = %q, want %q", body["error"], "authentication required")
	}
}

// TestRequireConnectionAuthAcceptsPortProxyCapability covers the subtree
// capability credential minted after the preview document authenticates: it is
// accepted as a query parameter (appended to rewritten asset URLs, used by
// credential-less fetches like the manifest) and as a path-scoped cookie
// (cookie-sending subresource fetches), and only for the exact session:port
// subtree it was minted for.
func TestRequireConnectionAuthAcceptsPortProxyCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := newGatewayForTest(t)
	gateway.SetPortProxy(nil) // constructs the handler; capability logic needs no manager
	gateway.SetAuthPolicy(AuthPolicy{
		Enforced:     func() bool { return true },
		ResolveToken: func(context.Context, string) (authn.Identity, bool) { return authn.Identity{}, false },
	})
	identity := authn.Identity{UserID: "owner-1", Role: authn.RoleAdmin}
	capability := gateway.PortProxyHandler.issueCapability("sess-cap", 5173, identity)

	type outcome struct {
		status   int
		identity authn.Identity
		hasID    bool
	}
	run := func(path string, cookie *http.Cookie) outcome {
		t.Helper()
		router := gin.New()
		var got outcome
		router.Any("/port-proxy/:sessionId/:port/*path", gateway.requireConnectionAuth(), func(c *gin.Context) {
			got.identity, got.hasID = authn.FromGin(c)
			c.Status(http.StatusOK)
		})
		router.Any("/vscode/:sessionId/*path", gateway.requireConnectionAuth(), func(c *gin.Context) {
			got.identity, got.hasID = authn.FromGin(c)
			c.Status(http.StatusOK)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		router.ServeHTTP(rec, req)
		got.status = rec.Code
		return got
	}

	// Query parameter form: accepted, identity restored.
	if got := run("/port-proxy/sess-cap/5173/manifest.webmanifest?"+proxyCapabilityQueryParam+"="+capability, nil); got.status != http.StatusOK {
		t.Fatalf("capability query status = %d, want %d", got.status, http.StatusOK)
	} else if !got.hasID || got.identity.UserID != "owner-1" {
		t.Fatalf("capability query identity = %+v (has=%v), want owner-1", got.identity, got.hasID)
	}

	// Cookie form: accepted.
	cookie := &http.Cookie{Name: proxyCapabilityCookieName, Value: capability, Path: "/port-proxy/sess-cap/5173"}
	if got := run("/port-proxy/sess-cap/5173/src/main.tsx", cookie); got.status != http.StatusOK {
		t.Fatalf("capability cookie status = %d, want %d", got.status, http.StatusOK)
	}

	// The same capability on a different subtree is rejected.
	if got := run("/port-proxy/other-session/5173/x?"+proxyCapabilityQueryParam+"="+capability, nil); got.status != http.StatusUnauthorized {
		t.Fatalf("capability on wrong session status = %d, want %d", got.status, http.StatusUnauthorized)
	}
	if got := run("/port-proxy/sess-cap/5174/x?"+proxyCapabilityQueryParam+"="+capability, nil); got.status != http.StatusUnauthorized {
		t.Fatalf("capability on wrong port status = %d, want %d", got.status, http.StatusUnauthorized)
	}

	// A capability on a non-port-proxy route is never consulted.
	if got := run("/vscode/sess-cap/x?"+proxyCapabilityQueryParam+"="+capability, nil); got.status != http.StatusUnauthorized {
		t.Fatalf("capability on vscode route status = %d, want %d", got.status, http.StatusUnauthorized)
	}

	// No credential at all is still rejected.
	if got := run("/port-proxy/sess-cap/5173/x", nil); got.status != http.StatusUnauthorized {
		t.Fatalf("no credential status = %d, want %d", got.status, http.StatusUnauthorized)
	}
}

// A request may carry multiple kandev_cap values (the app's own or encoded
// duplicates); a valid one must win even when an invalid value comes first.
func TestRequireConnectionAuthAcceptsAnyValidCapabilityAmongDuplicates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := newGatewayForTest(t)
	gateway.SetPortProxy(nil)
	gateway.SetAuthPolicy(AuthPolicy{
		Enforced:     func() bool { return true },
		ResolveToken: func(context.Context, string) (authn.Identity, bool) { return authn.Identity{}, false },
	})
	capability := gateway.PortProxyHandler.issueCapability("sess-cap", 5173,
		authn.Identity{UserID: "owner-1", Role: authn.RoleAdmin})

	router := gin.New()
	var got struct {
		identity authn.Identity
		hasID    bool
	}
	router.Any("/port-proxy/:sessionId/:port/*path", gateway.requireConnectionAuth(), func(c *gin.Context) {
		got.identity, got.hasID = authn.FromGin(c)
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/port-proxy/sess-cap/5173/x?"+proxyCapabilityQueryParam+"=garbage&"+proxyCapabilityQueryParam+"="+capability, nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (valid duplicate must win)", rec.Code, http.StatusOK)
	}
	if !got.hasID || got.identity.UserID != "owner-1" {
		t.Fatalf("identity = %+v (has=%v), want owner-1", got.identity, got.hasID)
	}
}

// A restored capability identity must pass the same live-account gate as
// cookie/PAT auth: a user disabled mid-preview cannot keep the sliding-mint
// capability alive.
func TestRequireConnectionAuthCapabilityRequiresActiveUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := newGatewayForTest(t)
	gateway.SetPortProxy(nil)
	identity := authn.Identity{UserID: "owner-1", Role: authn.RoleAdmin}
	capability := gateway.PortProxyHandler.issueCapability("sess-cap", 5173, identity)

	run := func(active func(context.Context, string) bool) int {
		gateway.SetAuthPolicy(AuthPolicy{
			Enforced:     func() bool { return true },
			ResolveToken: func(context.Context, string) (authn.Identity, bool) { return authn.Identity{}, false },
			ActiveUser:   active,
		})
		router := gin.New()
		router.Any("/port-proxy/:sessionId/:port/*path", gateway.requireConnectionAuth(), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/port-proxy/sess-cap/5173/x?"+proxyCapabilityQueryParam+"="+capability, nil)
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := run(func(context.Context, string) bool { return true }); got != http.StatusOK {
		t.Fatalf("active user status = %d, want %d", got, http.StatusOK)
	}
	if got := run(func(context.Context, string) bool { return false }); got != http.StatusUnauthorized {
		t.Fatalf("disabled user status = %d, want %d (capability must not outlive the account)", got, http.StatusUnauthorized)
	}
	// Nil ActiveUser preserves zero-policy behavior.
	if got := run(nil); got != http.StatusOK {
		t.Fatalf("nil ActiveUser status = %d, want %d", got, http.StatusOK)
	}
}

// Duplicate kandev_port_proxy cookies: a stale value sent first (more
// specific path) must not shadow a valid subtree cookie — any cookie value
// that validates authorizes the request.
func TestRequireConnectionAuthAcceptsAnyValidCapabilityCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := newGatewayForTest(t)
	gateway.SetPortProxy(nil)
	gateway.SetAuthPolicy(AuthPolicy{
		Enforced:     func() bool { return true },
		ResolveToken: func(context.Context, string) (authn.Identity, bool) { return authn.Identity{}, false },
	})
	capability := gateway.PortProxyHandler.issueCapability("sess-cap", 5173,
		authn.Identity{UserID: "owner-1", Role: authn.RoleAdmin})

	router := gin.New()
	var got struct {
		identity authn.Identity
		hasID    bool
	}
	router.Any("/port-proxy/:sessionId/:port/*path", gateway.requireConnectionAuth(), func(c *gin.Context) {
		got.identity, got.hasID = authn.FromGin(c)
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/port-proxy/sess-cap/5173/x", nil)
	req.AddCookie(&http.Cookie{Name: proxyCapabilityCookieName, Value: "stale-not-valid", Path: "/port-proxy/sess-cap/5173"})
	req.AddCookie(&http.Cookie{Name: proxyCapabilityCookieName, Value: capability, Path: "/port-proxy/sess-cap/5173"})
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (stale cookie must not shadow the valid one)", rec.Code, http.StatusOK)
	}
	if !got.hasID || got.identity.UserID != "owner-1" {
		t.Fatalf("identity = %+v (has=%v), want owner-1", got.identity, got.hasID)
	}
}

// TestRequireConnectionAuthIgnoresCapabilityWhenHandlerUnwired pins that the
// capability gate is inert when the port proxy is not wired (SetupRoutes
// registers no port-proxy routes then, and requireConnectionAuth must not
// reach into a nil handler).
func TestRequireConnectionAuthIgnoresCapabilityWhenHandlerUnwired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := NewGateway(newNopTestLogger(t))
	gateway.SetAuthPolicy(AuthPolicy{
		Enforced:     func() bool { return true },
		ResolveToken: func(context.Context, string) (authn.Identity, bool) { return authn.Identity{}, false },
	})

	router := gin.New()
	var called bool
	router.GET("/ws", gateway.requireConnectionAuth(), func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ws?token=anything", nil)
	req.AddCookie(&http.Cookie{Name: proxyCapabilityCookieName, Value: "whatever"})
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if called {
		t.Fatal("downstream handler ran without a valid credential")
	}
}

func newNopTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

func newAccessTestHub(t *testing.T) *Hub {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	hub := NewHub(ws.NewDispatcher(), log)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(ctx)
	return hub
}

func registerAccessClient(t *testing.T, hub *Hub, id string, identity authn.Identity) *Client {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	client := NewClient(id, identity, nil, hub, log)
	hub.Register(client)
	waitForClientCount(t, hub, id)
	return client
}

// waitForClientCount waits until the hub has processed the register message
// for the given client (Register goes through the Run loop channel).
func waitForClientCount(t *testing.T, hub *Hub, clientID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		hub.mu.RLock()
		found := false
		for client := range hub.clients {
			if client.ID == clientID {
				found = true
				break
			}
		}
		hub.mu.RUnlock()
		if found {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("client %s never registered", clientID)
}

func receivedActions(client *Client) []string {
	actions := []string{}
	for {
		select {
		case raw := <-client.controlSend:
			var msg ws.Message
			if err := json.Unmarshal(raw, &msg); err == nil {
				actions = append(actions, msg.Action)
			}
		case raw := <-client.send:
			var msg ws.Message
			if err := json.Unmarshal(raw, &msg); err == nil {
				actions = append(actions, msg.Action)
			}
		default:
			return actions
		}
	}
}

func TestBroadcastToWorkspaceRoutesByOwner(t *testing.T) {
	hub := newAccessTestHub(t)
	hub.setAuthPolicy(AuthPolicy{
		WorkspaceOwner: func(_ context.Context, workspaceID string) (string, error) {
			switch workspaceID {
			case "ws-a":
				return "user-a", nil
			case "ws-unowned":
				return "", nil
			}
			return "", errors.New("unknown workspace")
		},
	})

	clientA := registerAccessClient(t, hub, "a", authn.Identity{UserID: "user-a", Role: authn.RoleMember})
	clientB := registerAccessClient(t, hub, "b", authn.Identity{UserID: "user-b", Role: authn.RoleMember})
	clientSynthetic := registerAccessClient(t, hub, "s", authn.Identity{UserID: "default-user", Role: authn.RoleAdmin, Synthetic: true})

	notify := func(action string) *ws.Message {
		msg, err := ws.NewNotification(action, map[string]interface{}{"workspace_id": "ws-a"})
		if err != nil {
			t.Fatal(err)
		}
		return msg
	}

	// Owned workspace: owner + synthetic receive, the other user does not.
	hub.BroadcastToWorkspace("ws-a", notify("task.created"))
	if got := receivedActions(clientA); len(got) != 1 {
		t.Fatalf("owner received %v, want 1 message", got)
	}
	if got := receivedActions(clientB); len(got) != 0 {
		t.Fatalf("foreign user received %v, want none — WS LEAK", got)
	}
	if got := receivedActions(clientSynthetic); len(got) != 1 {
		t.Fatalf("synthetic client received %v, want 1 message", got)
	}

	// Unowned workspace falls back to broadcast-to-all (async via Run loop).
	hub.BroadcastToWorkspace("ws-unowned", notify("task.updated"))
	waitForMessage(t, clientB)

	// Resolver errors also fall back to broadcast (never drop events).
	hub.BroadcastToWorkspace("ws-mystery", notify("task.updated"))
	waitForMessage(t, clientB)

	// Empty workspace ID = instance-wide event = everyone.
	hub.BroadcastToWorkspace("", notify("executor.created"))
	waitForMessage(t, clientA)
}

// TestBroadcastToWorkspaceOrDropFailsClosed verifies office-scoped broadcasts
// are dropped when the hub cannot route by workspace.
func TestBroadcastToWorkspaceOrDropFailsClosed(t *testing.T) {
	hub := newAccessTestHub(t)
	hub.setAuthPolicy(AuthPolicy{
		Enforced: func() bool { return true },
		WorkspaceOwner: func(_ context.Context, workspaceID string) (string, error) {
			if workspaceID == "ws-a" {
				return "user-a", nil
			}
			return "", errors.New("unknown workspace")
		},
	})
	clientA := registerAccessClient(t, hub, "a", authn.Identity{UserID: "user-a", Role: authn.RoleMember})
	clientB := registerAccessClient(t, hub, "b", authn.Identity{UserID: "user-b", Role: authn.RoleMember})

	notify := func(action string) *ws.Message {
		msg, err := ws.NewNotification(action, map[string]interface{}{"workspace_id": "ws-a"})
		if err != nil {
			t.Fatal(err)
		}
		return msg
	}

	// Resolvable workspace: only the owner receives it, the foreign user
	// never does.
	hub.BroadcastToWorkspaceOrDrop("ws-a", notify("agent.profile.updated"))
	if got := receivedActions(clientA); len(got) != 1 {
		t.Fatalf("owner received %v, want 1 message", got)
	}
	if got := receivedActions(clientB); len(got) != 0 {
		t.Fatalf("foreign user received %v, want none — WS LEAK", got)
	}

	// Unresolvable workspace: DROPPED under enforced auth — never the
	// global fallback that BroadcastToWorkspace applies.
	hub.BroadcastToWorkspaceOrDrop("ws-mystery", notify("agent.profile.updated"))
	if got := receivedActions(clientB); len(got) != 0 {
		t.Fatalf("unresolvable workspace leaked globally: %v", got)
	}

	// Empty workspace ID under enforced auth: DROPPED.
	hub.BroadcastToWorkspaceOrDrop("", notify("agent.profile.updated"))
	if got := receivedActions(clientB); len(got) != 0 {
		t.Fatalf("empty workspace leaked globally: %v", got)
	}
}

func waitForMessage(t *testing.T, client *Client) {
	t.Helper()
	select {
	case <-client.send:
	case <-time.After(2 * time.Second):
		t.Fatalf("client %s did not receive expected broadcast", client.ID)
	}
}

func TestSubscriptionChecksDenyForeignTopics(t *testing.T) {
	hub := newAccessTestHub(t)
	denied := errors.New("denied")
	hub.setAuthPolicy(AuthPolicy{
		Subscriptions: SubscriptionAccessPolicy{
			Task: func(ctx context.Context, taskID string) error {
				identity, _ := authn.IdentityFromContext(ctx)
				if taskID == "task-b" && identity.UserID != "user-b" {
					return denied
				}
				return nil
			},
			Session: func(ctx context.Context, sessionID string) error {
				identity, _ := authn.IdentityFromContext(ctx)
				if sessionID == "sess-b" && identity.UserID != "user-b" {
					return denied
				}
				return nil
			},
			Run: func(ctx context.Context, runID string) error {
				identity, _ := authn.IdentityFromContext(ctx)
				if runID == "run-b" && identity.UserID != "user-b" {
					return denied
				}
				return nil
			},
		},
	})
	clientA := registerAccessClient(t, hub, "a", authn.Identity{UserID: "user-a", Role: authn.RoleMember})

	subscribe := func(action string, payload map[string]interface{}) *ws.Message {
		raw, _ := json.Marshal(payload)
		return &ws.Message{ID: "1", Type: ws.MessageTypeRequest, Action: action, Payload: raw}
	}

	clientA.handleSubscribe(subscribe(ws.ActionTaskSubscribe, map[string]interface{}{"task_id": "task-b"}))
	assertErrorResponse(t, clientA, "task subscribe")
	if len(hub.getSubscribersLocked(hub.taskSubscribers, "task-b")) != 0 {
		t.Fatal("denied task subscription must not register")
	}

	clientA.handleSessionSubscribe(subscribe(ws.ActionSessionSubscribe, map[string]interface{}{"session_id": "sess-b"}))
	assertErrorResponse(t, clientA, "session subscribe")
	if len(hub.getSubscribersLocked(hub.sessionSubscribers, "sess-b")) != 0 {
		t.Fatal("denied session subscription must not register")
	}

	clientA.handleRunSubscribe(subscribe(ws.ActionRunSubscribe, map[string]interface{}{"run_id": "run-b"}))
	assertErrorResponse(t, clientA, "run subscribe")
	if len(hub.getSubscribersLocked(hub.runSubscribers, "run-b")) != 0 {
		t.Fatal("denied run subscription must not register")
	}

	// Own topics still work.
	clientA.handleSubscribe(subscribe(ws.ActionTaskSubscribe, map[string]interface{}{"task_id": "task-a"}))
	assertSuccessResponse(t, clientA, "own task subscribe")
	if len(hub.getSubscribersLocked(hub.taskSubscribers, "task-a")) != 1 {
		t.Fatal("allowed task subscription must register")
	}

	clientA.handleRunSubscribe(subscribe(ws.ActionRunSubscribe, map[string]interface{}{"run_id": "run-a"}))
	assertSuccessResponse(t, clientA, "own run subscribe")
	if len(hub.getSubscribersLocked(hub.runSubscribers, "run-a")) != 1 {
		t.Fatal("allowed run subscription must register")
	}
}

// TestRunSubscribeDeniesEmptyWorkspaceResolution pins WO-02 round 2's
// FINDING R1-1 fix at the gateway wiring level: a Run hook denial for a run
// that resolves to an empty workspace (the runSubscriptionCheck case, an
// ordinary non-Office agent profile) must refuse the subscribe request and
// leave it unregistered in hub.runSubscribers — the same contract already
// pinned for a foreign-owned run by TestSubscriptionChecksDenyForeignTopics,
// exercised here for the empty-workspace input specifically.
func TestRunSubscribeDeniesEmptyWorkspaceResolution(t *testing.T) {
	hub := newAccessTestHub(t)
	errEmptyWorkspace := errors.New("workspace not found")
	hub.setAuthPolicy(AuthPolicy{
		Subscriptions: SubscriptionAccessPolicy{
			Run: func(_ context.Context, runID string) error {
				if runID == "run-unscoped" {
					return errEmptyWorkspace
				}
				return nil
			},
		},
	})
	client := registerAccessClient(t, hub, "a", authn.Identity{UserID: "user-a", Role: authn.RoleMember})

	subscribe := func(runID string) *ws.Message {
		raw, _ := json.Marshal(map[string]interface{}{"run_id": runID})
		return &ws.Message{ID: "1", Type: ws.MessageTypeRequest, Action: ws.ActionRunSubscribe, Payload: raw}
	}

	client.handleRunSubscribe(subscribe("run-unscoped"))
	assertErrorResponse(t, client, "run subscribe with empty-workspace resolution")
	if len(hub.getSubscribersLocked(hub.runSubscribers, "run-unscoped")) != 0 {
		t.Fatal("denied run subscription (empty workspace) must not register")
	}
}

// TestRunSubscribeNoExistenceLeak pins the no-existence-leak property: an
// unknown run id and a foreign run id must be byte-identical on the wire.
// handleRunSubscribe maps every hook error to one fixed (code, message)
// pair rather than forwarding err.Error(), so a caller cannot distinguish
// "you don't own this" from "this doesn't exist" by probing.
func TestRunSubscribeNoExistenceLeak(t *testing.T) {
	hub := newAccessTestHub(t)
	errUnknown := errors.New("sql: no rows in result set")
	errForeign := errors.New("run belongs to another workspace")
	hub.setAuthPolicy(AuthPolicy{
		Subscriptions: SubscriptionAccessPolicy{
			Run: func(_ context.Context, runID string) error {
				if runID == "run-unknown" {
					return errUnknown
				}
				return errForeign
			},
		},
	})
	client := registerAccessClient(t, hub, "a", authn.Identity{UserID: "user-a", Role: authn.RoleMember})

	subscribe := func(id, runID string) *ws.Message {
		raw, _ := json.Marshal(map[string]interface{}{"run_id": runID})
		return &ws.Message{ID: id, Type: ws.MessageTypeRequest, Action: ws.ActionRunSubscribe, Payload: raw}
	}

	client.handleRunSubscribe(subscribe("1", "run-unknown"))
	unknownFrame := readErrorFrame(t, client, "unknown run")

	client.handleRunSubscribe(subscribe("2", "run-foreign"))
	foreignFrame := readErrorFrame(t, client, "foreign run")

	if unknownFrame.code != foreignFrame.code || unknownFrame.message != foreignFrame.message {
		t.Fatalf("unknown run response (%q, %q) must match foreign run response (%q, %q)",
			unknownFrame.code, unknownFrame.message, foreignFrame.code, foreignFrame.message)
	}
}

type errorFrame struct {
	code    string
	message string
}

// readErrorFrame reads one frame off the client's queues and requires it to
// be an error, decoding the (code, message) payload for comparison.
func readErrorFrame(t *testing.T, client *Client, label string) errorFrame {
	t.Helper()
	var raw []byte
	select {
	case raw = <-client.controlSend:
	case raw = <-client.send:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: no response frame", label)
	}
	var msg ws.Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("%s: bad frame: %v", label, err)
	}
	if msg.Type != ws.MessageTypeError {
		t.Fatalf("%s: got %s frame, want error", label, msg.Type)
	}
	var payload ws.ErrorPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("%s: bad error payload: %v", label, err)
	}
	return errorFrame{code: payload.Code, message: payload.Message}
}

func TestUserSubscribeScopedToOwnUser(t *testing.T) {
	hub := newAccessTestHub(t)
	client := registerAccessClient(t, hub, "a", authn.Identity{UserID: "user-a", Role: authn.RoleMember})

	raw, _ := json.Marshal(map[string]interface{}{"user_id": "user-b"})
	client.handleUserSubscribe(&ws.Message{ID: "1", Type: ws.MessageTypeRequest, Action: ws.ActionUserSubscribe, Payload: raw})
	assertErrorResponse(t, client, "foreign user subscribe")

	// Empty user_id defaults to the client's own user.
	raw, _ = json.Marshal(map[string]interface{}{})
	client.handleUserSubscribe(&ws.Message{ID: "2", Type: ws.MessageTypeRequest, Action: ws.ActionUserSubscribe, Payload: raw})
	assertSuccessResponse(t, client, "own user subscribe")
	if len(hub.getSubscribersLocked(hub.userSubscribers, "user-a")) != 1 {
		t.Fatal("own-user subscription must land on the client's user topic")
	}
}

func assertErrorResponse(t *testing.T, client *Client, label string) {
	t.Helper()
	select {
	case raw := <-client.controlSend:
		var msg ws.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("%s: bad frame: %v", label, err)
		}
		if msg.Type != ws.MessageTypeError {
			t.Fatalf("%s: got %s frame, want error", label, msg.Type)
		}
	case raw := <-client.send:
		var msg ws.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("%s: bad frame: %v", label, err)
		}
		if msg.Type != ws.MessageTypeError {
			t.Fatalf("%s: got %s frame, want error", label, msg.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: no response frame", label)
	}
}

func assertSuccessResponse(t *testing.T, client *Client, label string) {
	t.Helper()
	select {
	case raw := <-client.controlSend:
		var msg ws.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("%s: bad frame: %v", label, err)
		}
		if msg.Type == ws.MessageTypeError {
			t.Fatalf("%s: got error frame: %s", label, string(raw))
		}
	case raw := <-client.send:
		var msg ws.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("%s: bad frame: %v", label, err)
		}
		if msg.Type == ws.MessageTypeError {
			t.Fatalf("%s: got error frame: %s", label, string(raw))
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: no response frame", label)
	}
}

// Regression: workspace.* event payloads carry the workspace ID under "id"
// (the payload IS the workspace DTO), not "workspace_id". The first
// implementation missed this and broadcast admin workspace updates to every
// user — caught by the auth E2E segregation spec.
func TestExtractWorkspaceIDFieldShapes(t *testing.T) {
	if got := extractWorkspaceID(map[string]interface{}{"workspace_id": "ws-1"}); got != "ws-1" {
		t.Fatalf("workspace_id key: %q", got)
	}
	if got := extractWorkspaceID(map[string]interface{}{"id": "ws-1"}); got != "" {
		t.Fatalf("bare id must NOT be treated as workspace context for generic events: %q", got)
	}
	if got := extractStringField(map[string]interface{}{"id": "ws-1"}, "id"); got != "ws-1" {
		t.Fatalf("extractStringField: %q", got)
	}
	if got := extractWorkspaceID("not-a-map"); got != "" {
		t.Fatalf("non-map payload: %q", got)
	}
	// Profile events wrap the profile under "profile". The wrapper is a map
	// after a JSON round-trip (remote event bus) and a struct on the
	// in-process bus; both must resolve the nested workspace ID so office
	// profile events never fall back to the global broadcast.
	if got := extractWorkspaceID(map[string]interface{}{
		"profile": map[string]interface{}{"workspace_id": "ws-nested-map"},
	}); got != "ws-nested-map" {
		t.Fatalf("map-nested profile workspace_id: %q", got)
	}
	if got := extractWorkspaceID(map[string]interface{}{
		"profile": &dto.AgentProfileDTO{WorkspaceID: "ws-nested-struct"},
	}); got != "ws-nested-struct" {
		t.Fatalf("struct-nested profile workspace_id: %q", got)
	}
}
