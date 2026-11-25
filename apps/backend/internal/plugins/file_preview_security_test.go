package plugins

import (
	"context"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
)

func TestFilePreviewPolicyRejectsAPITokenMutation(t *testing.T) {
	router, svc := newTestRouterWithIdentity(t, authn.Identity{UserID: "alice", Role: authn.RoleAdmin, TokenID: "token"})
	svc.SetFilePreviewWorkspaceAuthorizer(func(context.Context, string) error { return nil })
	response := doRequest(router, http.MethodPut, "/api/plugins/viewer/file-preview-policy?workspace_id=workspace", `{"mode":"allow","expected_revision":0}`, jsonHeaders())
	if response.Code != http.StatusForbidden {
		t.Fatalf("API token policy mutation: %d", response.Code)
	}
}
func TestFilePreviewPolicyAuditIsBounded(t *testing.T) {
	policies := newFilePreviewPolicies(t.TempDir())
	scope := filePreviewScope{UserID: "user", WorkspaceID: "workspace", InstallationID: "install", Digest: "digest"}
	for revision := uint64(0); revision < 40; revision++ {
		if _, err := policies.set(scope, "ask", revision); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := policies.load()
	if err != nil {
		t.Fatal(err)
	}
	key, _ := scope.key()
	if len(rows[key].Events) != 32 || rows[key].Events[0].Revision != 9 || rows[key].Events[31].Revision != 40 {
		t.Fatalf("audit bound: %+v", rows[key].Events)
	}
}
