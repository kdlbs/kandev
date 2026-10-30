package plugins

import (
	"net/http"
	"testing"
)

func TestThemeRoutesAdminAndInventory(t *testing.T) {
	svc, _, _ := newTestService(t)
	member := registerMemberPluginRoutes(t, svc)
	for _, path := range []string{"/api/plugins/themes/prepare", "/api/plugins/themes/commit"} {
		response := doRequest(member, http.MethodPost, path, `{}`, nil)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s: %d", path, response.Code)
		}
	}
	admin := registerAdminPluginRoutes(t, svc)
	response := doRequest(admin, http.MethodGet, "/api/plugins/themes", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("inventory: %d %s", response.Code, response.Body.String())
	}
}
