package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestBearerTokenAuth_EmptyToken_ExposesCommandRoutes_PoC demonstrates the
// LOW-severity defense-in-depth gap: when AuthToken is empty the bearer-token
// middleware becomes a pass-through, so a request with NO Authorization header
// reaches command-execution routes such as /api/v1/processes/start (which runs
// `sh -lc <command>`), git, and /api/v1/shell/*.
//
// This is by design for standalone loopback mode (no nonce → no token), but is
// a real risk if the listener is ever bound beyond loopback. The fix does NOT
// change this middleware behavior (that would need a product decision); it
// enforces loopback-only binding when no token is configured, so this
// pass-through is only ever reachable from localhost. See the ListenHost guard
// (config package) and the open question noted in the task report.
func TestBearerTokenAuth_EmptyToken_ExposesCommandRoutes_PoC(t *testing.T) {
	r := gin.New()
	r.Use(bearerTokenAuth("" /* no token → auth disabled */))
	r.POST("/api/v1/processes/start", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "command executed"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/processes/start", nil)
	// Note: no Authorization header at all.
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PoC expected unauthenticated request to reach protected command route (200), got %d", w.Code)
	}
	t.Log("PoC: with empty AuthToken, an unauthenticated POST reaches /api/v1/processes/start")
}
