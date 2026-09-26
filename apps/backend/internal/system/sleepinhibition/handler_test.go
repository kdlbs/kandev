package sleepinhibition

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

func TestHandlerRejectsMalformedPatchAndReturnsReconciledResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := NewService(NewStore(&memoryRawStore{}), emptySessionReader{}, &fakeInhibitor{platform: PlatformOther}, bus.NewMemoryEventBus(nil), nil)
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		authn.SetOnGin(ctx, authn.Identity{UserID: "admin", Role: authn.RoleAdmin})
		ctx.Next()
	})
	g := router.Group("/api")
	admin := g.Group("", authn.RequireAdmin())
	RegisterRoutes(g, admin, service)

	bad := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/sleep-inhibition", nil)
	router.ServeHTTP(bad, request)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("malformed patch status = %d, want 400", bad.Code)
	}

	good := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/sleep-inhibition", newJSONBuffer(`{"enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(good, request)
	if good.Code != http.StatusOK {
		t.Fatalf("valid patch status = %d, body=%s", good.Code, good.Body.String())
	}
	if got := good.Body.String(); !containsAll(got, `"enabled":true`, `"active":false`, `"unsupported_platform"`) {
		t.Fatalf("reconciled response = %s", got)
	}
}

func TestHandlerAllowsAuthenticatedRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := NewService(NewStore(&memoryRawStore{}), emptySessionReader{}, &fakeInhibitor{platform: PlatformLinux, supported: true}, bus.NewMemoryEventBus(nil), nil)
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		authn.SetOnGin(ctx, authn.Identity{UserID: "member", Role: authn.RoleMember})
		ctx.Next()
	})
	g := router.Group("/api")
	admin := g.Group("", authn.RequireAdmin())
	RegisterRoutes(g, admin, service)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/sleep-inhibition", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("member GET status = %d, body=%s", response.Code, response.Body.String())
	}
}

type emptySessionReader struct{}

func (emptySessionReader) ListActiveTaskSessions(context.Context) ([]*models.TaskSession, error) {
	return nil, nil
}

func newJSONBuffer(value string) *bytes.Buffer { return bytes.NewBufferString(value) }

func containsAll(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(value, fragment) {
			return false
		}
	}
	return true
}
