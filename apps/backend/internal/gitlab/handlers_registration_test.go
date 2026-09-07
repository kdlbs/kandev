package gitlab

import (
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

func newRouteRegistrationFixture(t *testing.T, client Client) (*Service, *logger.Logger) {
	t.Helper()
	log := newTestLogger(t)
	return NewService(DefaultHost, client, AuthMethodNone, nil, log), log
}

func TestRegisterRoutesMountsHTTPRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, log := newRouteRegistrationFixture(t, NewMockClient(DefaultHost))
	router := gin.New()

	RegisterRoutes(router, svc, log)

	want := map[string]string{
		"GET":    "/api/v1/gitlab/status",
		"POST":   "/api/v1/gitlab/watches/review",
		"DELETE": "/api/v1/gitlab/watches/review/:id",
		"PUT":    "/api/v1/gitlab/action-presets",
	}
	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for method, path := range want {
		if !routes[method+" "+path] {
			t.Errorf("route %s %s is not registered", method, path)
		}
	}
}

func TestRegisterMockRoutesIsNoOpForNonMockClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, log := newRouteRegistrationFixture(t, NewNoopClient(DefaultHost))
	router := gin.New()

	RegisterMockRoutes(router, svc, log)

	if got := len(router.Routes()); got != 0 {
		t.Errorf("mock routes = %d, want 0 for a non-mock client (%v)", got, router.Routes())
	}
}

func TestRegisterMockRoutesMountsControlEndpointsForMockClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, log := newRouteRegistrationFixture(t, NewMockClient(DefaultHost))
	router := gin.New()

	RegisterMockRoutes(router, svc, log)

	if len(router.Routes()) == 0 {
		t.Fatal("no mock control endpoints were registered for a MockClient")
	}
}
