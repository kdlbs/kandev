package docker

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
)

func networkDefaultRouter(t *testing.T, defaultNetwork string, identity *authn.Identity) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if identity != nil {
		router.Use(func(c *gin.Context) {
			authn.SetOnGin(c, *identity)
			c.Next()
		})
	}
	registerRoutes(router, func() containerAPI { return nil }, nil, nil, defaultNetwork, testLogger(t))
	return router
}

// The profile editor tells an operator which network applies when a profile
// names none. It cannot say that without the effective install-wide value.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-003.2
func TestNetworkDefaultReportsTheConfiguredValue(t *testing.T) {
	router := networkDefaultRouter(t, "install-bridge", &authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin})

	response := do(router, http.MethodGet, "/api/v1/docker/network-default", "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		DefaultNetwork string `json:"default_network"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.DefaultNetwork != "install-bridge" {
		t.Errorf("default_network = %q, want %q", body.DefaultNetwork, "install-bridge")
	}
}

// An unset value is reported as empty rather than omitted, so the editor can
// distinguish "no default configured" from a response it failed to read.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-003.2
func TestNetworkDefaultReportsAnUnsetValueAsEmpty(t *testing.T) {
	router := networkDefaultRouter(t, "", &authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin})

	response := do(router, http.MethodGet, "/api/v1/docker/network-default", "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := response.Body.String(); got != `{"default_network":""}` {
		t.Errorf("body = %s, want an explicit empty value", got)
	}
}

// It reports an install-wide operator setting, so it is admin-only like the
// other install-wide Docker route.
func TestNetworkDefaultIsAdminOnly(t *testing.T) {
	router := networkDefaultRouter(t, "install-bridge", member("user-a"))

	response := do(router, http.MethodGet, "/api/v1/docker/network-default", "")

	if response.Code == http.StatusOK {
		t.Fatalf("status = %d, want a denial for a non-admin", response.Code)
	}
}
