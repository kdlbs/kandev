package clarification

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
)

// registeredPaths collects every route path RegisterRoutes wired onto router,
// independent of HTTP method.
func registeredPaths(router *gin.Engine) map[string]bool {
	paths := map[string]bool{}
	for _, route := range router.Routes() {
		paths[route.Path] = true
	}
	return paths
}

// TestRegisterRoutes_InboxGatedByFlag guards the Needs-you Inbox routes
// against shipping reachable in a profile where the feature flag is off: the
// original /api/v1/clarification routes must stay unconditional (every
// agent's clarification-request/respond flow does not carry the flag), while
// the four /api/v1/clarification-inbox routes must only be registered when
// needsYouInboxEnabled is true.
func TestRegisterRoutes_InboxGatedByFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("flag off: inbox routes absent, original routes present", func(t *testing.T) {
		router := gin.New()
		RegisterRoutes(router, nil, nil, nil, nil, nil, nil, logger.Default(), nil, nil, false)
		paths := registeredPaths(router)

		if !paths["/api/v1/clarification/request"] {
			t.Error("/api/v1/clarification/request missing: the original route must stay unconditional")
		}
		for _, path := range []string{
			"/api/v1/clarification-inbox",
			"/api/v1/clarification-inbox/hidden",
			"/api/v1/clarification-inbox/sidecar/:pendingID",
		} {
			if paths[path] {
				t.Errorf("%s registered with the flag off: the Inbox API must not be reachable in a profile where features.needsYouInbox is false", path)
			}
		}
	})

	t.Run("flag on: inbox routes present alongside the original routes", func(t *testing.T) {
		router := gin.New()
		RegisterRoutes(router, nil, nil, nil, nil, nil, nil, logger.Default(), nil, nil, true)
		paths := registeredPaths(router)

		for _, path := range []string{
			"/api/v1/clarification/request",
			"/api/v1/clarification-inbox",
			"/api/v1/clarification-inbox/hidden",
			"/api/v1/clarification-inbox/sidecar/:pendingID",
		} {
			if !paths[path] {
				t.Errorf("%s missing with the flag on", path)
			}
		}
	})
}
