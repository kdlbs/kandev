package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
)

func assertGatewayAuthUnavailable(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (body=%s)", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("503 body is not JSON (%q): %v", rec.Body.String(), err)
	}
	if body["error"] != "authentication temporarily unavailable" {
		t.Fatalf("503 error = %q", body["error"])
	}
	if body["code"] != "persistence_unavailable" {
		t.Fatalf("503 code = %q", body["code"])
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Fatalf("503 must not challenge, WWW-Authenticate = %q", got)
	}
}

func TestQueryTokenStoreFailureBeforeUpgrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var resolved string
	policy := AuthPolicy{
		Enforced: func() bool { return true },
		ResolveToken: func(_ context.Context, token string) (authn.Identity, bool, error) {
			resolved = token
			return authn.Identity{}, false, errors.New("credential store is unavailable")
		},
	}
	got := runConnectionAuth(t, policy, "?token=kandev_pat_fixture", nil)
	if got.nextCalled {
		t.Fatal("downstream upgrade handler ran after a failed query-token lookup")
	}
	if resolved != "kandev_pat_fixture" {
		t.Fatalf("resolved token = %q", resolved)
	}
	assertGatewayAuthUnavailable(t, got.recorder)
}

func TestProxyTokenLookupFailureDoesNotForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := newGatewayForTest(t)
	gateway.SetPortProxy(nil)
	identity := authn.Identity{UserID: "owner-1", Role: authn.RoleAdmin}
	capability := gateway.PortProxyHandler.issueCapability("sess-proxy", 5173, identity)
	var resolved string
	gateway.SetAuthPolicy(AuthPolicy{
		Enforced: func() bool { return true },
		ResolveToken: func(_ context.Context, token string) (authn.Identity, bool, error) {
			resolved = token
			return authn.Identity{}, false, errors.New("credential store is unavailable")
		},
	})

	var nextCalled bool
	router := gin.New()
	router.GET("/port-proxy/:sessionID/:port/*path", gateway.requireConnectionAuth(), func(c *gin.Context) {
		nextCalled = true
		c.Status(http.StatusOK)
	})
	target := "/port-proxy/sess-proxy/5173/app.js?kandev_cap=" + url.QueryEscape(capability) +
		"&token=kandev_pat_fixture"
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if nextCalled {
		t.Fatal("proxy request was forwarded after its query token could not be verified")
	}
	if resolved != "kandev_pat_fixture" {
		t.Fatalf("resolved token = %q", resolved)
	}
	assertGatewayAuthUnavailable(t, rec)
}
