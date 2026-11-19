package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
)

func TestFilePreviewPolicyHandlers(t *testing.T) {
	router, svc := newTestRouterWithIdentity(t, authn.Identity{UserID: "alice", SessionID: "browser-session", Role: authn.RoleMember})
	svc.SetFilePreviewWorkspaceAuthorizer(func(ctx context.Context, workspace string) error {
		if workspace != "workspace" {
			return fmt.Errorf("forbidden")
		}
		return nil
	})
	installed, err := svc.Install(t.Context(), capabilityApprovalTestPackage(t, "kandev-plugin-preview-policy"))
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/plugins/" + installed.ID + "/file-preview-policy?workspace_id=workspace"
	initial := doRequest(router, http.MethodGet, endpoint, "", nil)
	if initial.Code != 200 {
		t.Fatalf("get %d: %s", initial.Code, initial.Body.String())
	}
	var policy filePreviewPolicy
	if err := json.Unmarshal(initial.Body.Bytes(), &policy); err != nil {
		t.Fatal(err)
	}
	if policy.Mode != "ask" {
		t.Fatal("unsafe initial mode")
	}
	changed := doRequest(router, http.MethodPut, endpoint, `{"mode":"allow","expected_revision":0}`, jsonHeaders())
	if changed.Code != 200 {
		t.Fatalf("set %d: %s", changed.Code, changed.Body.String())
	}
	stale := doRequest(router, http.MethodPut, endpoint, `{"mode":"deny","expected_revision":0}`, jsonHeaders())
	if stale.Code != 409 {
		t.Fatalf("stale status %d", stale.Code)
	}
	foreign := doRequest(router, http.MethodGet, "/api/plugins/"+installed.ID+"/file-preview-policy?workspace_id=foreign", "", nil)
	if foreign.Code != 403 {
		t.Fatalf("foreign workspace status %d", foreign.Code)
	}
}
