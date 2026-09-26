package dockerremote

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
)

// TestRoutesRequireAdmin pins the trust boundary for both routes.
//
// The test route dials a host the caller names and a saved profile grants
// effective root on that host; the build route runs arbitrary Dockerfile
// instructions with the remote daemon's authority. Neither is an ordinary
// member operation.
func TestRoutesRequireAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	routes := []struct {
		name string
		path string
	}{
		{name: "connection test", path: "/api/v1/remote-docker/test"},
		{name: "image build", path: "/api/v1/remote-docker/executors/exec-1/build"},
	}

	for _, route := range routes {
		t.Run(route.name+" without an identity", func(t *testing.T) {
			router := gin.New()
			RegisterRoutes(router, nil, nil)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, route.path, bytes.NewBufferString(`{}`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 for an unauthenticated caller", rec.Code)
			}
		})

		t.Run(route.name+" as a non-admin", func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				authn.SetOnGin(c, authn.Identity{UserID: "user-1", Role: authn.RoleMember})
				c.Next()
			})
			RegisterRoutes(router, nil, nil)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, route.path, bytes.NewBufferString(`{}`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 for a non-admin caller", rec.Code)
			}
		})
	}
}
