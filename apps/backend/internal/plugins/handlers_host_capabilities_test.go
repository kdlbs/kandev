package plugins

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestHostCapabilitiesReadIsAdminOnlyAndListsTheRegistry(t *testing.T) {
	svc, _, _ := newTestService(t)

	rec := doRequest(registerMemberPluginRoutes(t, svc), http.MethodGet, "/api/plugins/host-capabilities", "", nil)
	requireMemberForbidden(t, rec.Code, rec.Body.String())

	rec = doRequest(registerAdminPluginRoutes(t, svc), http.MethodGet, "/api/plugins/host-capabilities", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Capabilities []struct {
			Name         string `json:"name"`
			MinVersion   int    `json:"min_version"`
			MaxVersion   int    `json:"max_version"`
			IntroducedIn string `json:"introduced_in"`
			Deprecated   bool   `json:"deprecated"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Capabilities) != 17 {
		t.Fatalf("capabilities = %d, want the 17 registry names", len(body.Capabilities))
	}
	first := body.Capabilities[0]
	if first.Name == "" || first.MinVersion != 1 || first.MaxVersion != 1 || first.IntroducedIn == "" || first.Deprecated {
		t.Fatalf("first row = %+v", first)
	}
}
