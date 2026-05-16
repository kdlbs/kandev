package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupInstanceGuardRouter(expectedID string) *gin.Engine {
	r := gin.New()
	r.Use(instanceIDGuard(expectedID))
	r.GET("/api/v1/data", func(c *gin.Context) { c.JSON(200, gin.H{"data": "secret"}) })
	return r
}

// TestInstanceIDGuard_EmptyExpectedID_NoOp covers the legacy / test wiring
// path where InstanceConfig.InstanceID is unset. The guard must let every
// request through regardless of X-Instance-ID so callers that predate this
// middleware keep working.
func TestInstanceIDGuard_EmptyExpectedID_NoOp(t *testing.T) {
	r := setupInstanceGuardRouter("")

	tests := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"matching header", "anything"},
		{"mismatched header", "different"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/api/v1/data", nil)
			if tt.header != "" {
				req.Header.Set(InstanceIDHeader, tt.header)
			}
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200 (no-op when expectedID empty), got %d", w.Code)
			}
		})
	}
}

// TestInstanceIDGuard_MissingHeader_Passes covers the backward-compat
// case: the agent subprocess's local MCP loopback, raw curl probes, and
// older clients don't send X-Instance-ID. Missing header must pass —
// only an *explicit mismatch* is a stale-client signal.
func TestInstanceIDGuard_MissingHeader_Passes(t *testing.T) {
	r := setupInstanceGuardRouter("instance-abc")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/data", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (missing header is allowed), got %d", w.Code)
	}
}

// TestInstanceIDGuard_MatchingHeader_Passes is the normal path —
// kandev's agentctl Client stamps the executionID it owns and gets
// through to its own instance's handlers.
func TestInstanceIDGuard_MatchingHeader_Passes(t *testing.T) {
	r := setupInstanceGuardRouter("instance-abc")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/data", nil)
	req.Header.Set(InstanceIDHeader, "instance-abc")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for matching header, got %d", w.Code)
	}
}

// TestInstanceIDGuard_MismatchedHeader_404 is the bug-fix path: a
// stale client whose port was recycled to a different instance must
// be rejected with 404 instead of accidentally driving the new
// instance's process Manager.
func TestInstanceIDGuard_MismatchedHeader_404(t *testing.T) {
	r := setupInstanceGuardRouter("instance-abc")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/data", nil)
	req.Header.Set(InstanceIDHeader, "instance-xyz")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on mismatch, got %d", w.Code)
	}
}

// TestInstanceIDGuard_TrimsWhitespace defends against a client that
// accidentally appends a newline or spaces to the header value (a
// past source of "why doesn't my header match" bugs in middleware
// that uses raw Header.Get).
func TestInstanceIDGuard_TrimsWhitespace(t *testing.T) {
	r := setupInstanceGuardRouter("instance-abc")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/data", nil)
	req.Header.Set(InstanceIDHeader, "  instance-abc  ")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after trim, got %d", w.Code)
	}
}
