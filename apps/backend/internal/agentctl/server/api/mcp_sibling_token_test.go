package api

import (
	"net/http"
	"strconv"
	"testing"
)

// Each MCP route of a live instance answers 401 to another instance's token
// and to no token, and does not answer 401 to its own.
func TestMCPRoutesRejectSiblingAndMissingToken(t *testing.T) {
	cs, mgr := tokenFixture(t)
	for id, tok := range map[string]string{"inst-a": "tok-a", "inst-b": "tok-b"} {
		if rec := createTokenInstance(t, cs, id, tok); rec.Code != http.StatusCreated {
			t.Fatalf("create %s: %d", id, rec.Code)
		}
		t.Cleanup(func() { _ = mgr.StopInstance(t.Context(), id) })
	}
	a, _ := mgr.GetInstance("inst-a")
	base := "http://127.0.0.1:" + strconv.Itoa(a.Port)

	status := func(method, path, auth string) int {
		req, err := http.NewRequestWithContext(t.Context(), method, base+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/sse"}, {http.MethodPost, "/message"}, {http.MethodPost, "/mcp"},
	} {
		t.Run(route.method+route.path, func(t *testing.T) {
			if got := status(route.method, route.path, ""); got != http.StatusUnauthorized {
				t.Fatalf("no token: %d", got)
			}
			if got := status(route.method, route.path, "Bearer tok-b"); got != http.StatusUnauthorized {
				t.Fatalf("sibling token: %d", got)
			}
			if got := status(route.method, route.path, "Bearer tok-a"); got == http.StatusUnauthorized {
				t.Fatalf("own token rejected: %d", got)
			}
		})
	}
}

// An instance created without a token serves its MCP routes unguarded until a
// revoke, after which they answer 503.
func TestLegacyInstanceServesMCPRoutesUntilRevoked(t *testing.T) {
	cs, mgr := tokenFixture(t)
	if rec := createTokenInstance(t, cs, "legacy", ""); rec.Code != http.StatusCreated {
		t.Fatalf("create: %d", rec.Code)
	}
	t.Cleanup(func() { _ = mgr.StopInstance(t.Context(), "legacy") })
	inst, _ := mgr.GetInstance("legacy")
	base := "http://127.0.0.1:" + strconv.Itoa(inst.Port)
	status := func(method, path string) int {
		req, err := http.NewRequestWithContext(t.Context(), method, base+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	routes := []struct{ method, path string }{
		{http.MethodGet, "/sse"}, {http.MethodPost, "/message"}, {http.MethodPost, "/mcp"},
	}
	for _, r := range routes {
		if got := status(r.method, r.path); got == http.StatusUnauthorized || got == http.StatusServiceUnavailable {
			t.Fatalf("legacy %s %s refused: %d", r.method, r.path, got)
		}
	}
	if rec := controlDo(cs, http.MethodPost, "/api/v1/instances/legacy/mcp-token/revoke", ""); rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d", rec.Code)
	}
	for _, r := range routes {
		if got := status(r.method, r.path); got != http.StatusServiceUnavailable {
			t.Fatalf("revoked %s %s: %d, want 503", r.method, r.path, got)
		}
	}
}
