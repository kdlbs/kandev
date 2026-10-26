package instance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func guardFixture(t *testing.T, token string) (http.Handler, *Instance, *atomic.Int32, *config.SessionMCPToken) {
	t.Helper()
	inst := &Instance{ID: "inst-a"}
	holder := config.NewSessionMCPToken(token)
	var served atomic.Int32
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	h := newMCPGuard(holder, newTestLogger(t))(activityMiddleware(inst)(next))
	return h, inst, &served, holder
}

func doReq(h http.Handler, method, path string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Add(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMCPGuard_RefusesAndAcceptsOnAllPaths(t *testing.T) {
	h, inst, served, _ := guardFixture(t, "tok-a")
	for _, path := range []string{"/mcp", "/sse", "/message", "/mcp/", "/message/x"} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			cases := map[string][]string{
				"missing":   nil,
				"empty":     {"Authorization", ""},
				"scheme":    {"Authorization", "Basic tok-a"},
				"two":       {"Authorization", "Bearer tok-a", "Authorization", "Bearer tok-a"},
				"noToken":   {"Authorization", "Bearer "},
				"wrong":     {"Authorization", "Bearer tok-b"},
				"controlOK": {"Authorization", "Bearer control-credential"},
			}
			for name, hdr := range cases {
				rec := doReq(h, method, path, hdr...)
				if rec.Code != http.StatusUnauthorized {
					t.Fatalf("%s %s %s: status %d, want 401", method, path, name, rec.Code)
				}
			}
			if rec := doReq(h, method, path, "Authorization", "Bearer tok-a"); rec.Code != http.StatusOK {
				t.Fatalf("%s %s valid token: status %d", method, path, rec.Code)
			}
		}
	}
	if served.Load() == 0 {
		t.Fatal("valid requests never reached the handler")
	}
	_ = inst
}

func TestMCPGuard_RefusalMovesNoActivity(t *testing.T) {
	h, inst, served, _ := guardFixture(t, "tok-a")
	before := inst.LastActivity()
	rec := doReq(h, http.MethodPost, "/message", "Authorization", "Bearer nope")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
	if served.Load() != 0 {
		t.Fatal("refused request reached the handler")
	}
	if !inst.LastActivity().Equal(before) || inst.inflightRequests.Load() != 0 {
		t.Fatal("refusal changed activity accounting")
	}
	if body := rec.Body.String(); containsAny(body, "tok-a", "nope") {
		t.Fatalf("refusal body leaks token detail: %s", body)
	}
}

func TestMCPGuard_LegacyInstanceServesMCPRoutesUnguarded(t *testing.T) {
	h, _, served, _ := guardFixture(t, "")
	for _, path := range []string{"/mcp", "/sse", "/message"} {
		for _, hdr := range [][]string{nil, {"Authorization", "Bearer anything"}} {
			if rec := doReq(h, http.MethodGet, path, hdr...); rec.Code != http.StatusOK {
				t.Fatalf("legacy %s %v: status %d, want 200", path, hdr, rec.Code)
			}
		}
	}
	if served.Load() != 6 {
		t.Fatalf("legacy requests dispatched %d times, want 6", served.Load())
	}
}

func TestMCPGuard_RevokedLegacyInstanceAnswers503(t *testing.T) {
	h, _, served, holder := guardFixture(t, "")
	holder.Revoke()
	rec := doReq(h, http.MethodGet, "/sse", "Authorization", "Bearer anything")
	if rec.Code != http.StatusServiceUnavailable || !containsAny(rec.Body.String(), "mcp_token_unconfigured") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	if served.Load() != 0 {
		t.Fatal("revoked instance dispatched")
	}
}

func TestMCPGuard_RevokeTurnsAcceptedTokenInto503(t *testing.T) {
	h, _, _, holder := guardFixture(t, "tok-a")
	if rec := doReq(h, http.MethodGet, "/mcp", "Authorization", "Bearer tok-a"); rec.Code != http.StatusOK {
		t.Fatalf("pre-revoke %d", rec.Code)
	}
	holder.Revoke()
	if rec := doReq(h, http.MethodGet, "/mcp", "Authorization", "Bearer tok-a"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("post-revoke %d", rec.Code)
	}
}

func TestMCPGuard_NonMCPPathsAndVariantsPassThrough(t *testing.T) {
	h, _, served, _ := guardFixture(t, "tok-a")
	for _, path := range []string{"/health", "/api/v1/status", "/MCP", "/mcpx", "/sse.", "/identity"} {
		if rec := doReq(h, http.MethodGet, path); rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
	if served.Load() != 6 {
		t.Fatalf("served %d", served.Load())
	}
}

func TestMCPGuard_ConcurrentMixedRequests(t *testing.T) {
	h, _, _, _ := guardFixture(t, "tok-a")
	var wg sync.WaitGroup
	var bad atomic.Int32
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if doReq(h, http.MethodGet, "/mcp", "Authorization", "Bearer tok-a").Code != http.StatusOK {
				bad.Add(1)
			}
		}()
		go func() {
			defer wg.Done()
			if doReq(h, http.MethodGet, "/mcp", "Authorization", "Bearer x").Code != http.StatusUnauthorized {
				bad.Add(1)
			}
		}()
	}
	wg.Wait()
	if bad.Load() != 0 {
		t.Fatalf("%d wrong outcomes", bad.Load())
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
