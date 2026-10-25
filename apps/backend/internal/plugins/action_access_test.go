package plugins

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func adminActionPackage(t *testing.T) *bytes.Buffer {
	t.Helper()
	manifestYAML := fmt.Sprintf(`
id: kandev-plugin-admin-action
api_version: 1
version: "1.0.0"
display_name: Admin Action
min_kandev_version: "0.91.1"
actions:
  - key: connection.set
    scope: workspace
    access: admin
    max_body_bytes: 128
runtime:
  type: binary
  executables:
    %s-%s: server/plugin
`, runtime.GOOS, runtime.GOARCH)
	var buf bytes.Buffer
	if err := pkgtartest.WritePackage(&buf, map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	return &buf
}

func doActionRequestAsRole(router http.Handler, role authn.Role) *httptest.ResponseRecorder {
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/plugins/kandev-plugin-admin-action/actions/connection.set",
		strings.NewReader(`{"workspaceId":"workspace-1","body":{}}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{
		UserID: "user-1",
		Role:   role,
	}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestActionHandlerRequiresAdminForAdminAccess(t *testing.T) {
	invoker := &recordingActionInvoker{
		response: &pluginsdk.PluginActionResponse{Body: []byte(`{"ok":true}`)},
	}
	router, service := newActionTestRouter(t, invoker)
	if _, err := service.Install(t.Context(), adminActionPackage(t)); err != nil {
		t.Fatalf("Install: %v", err)
	}

	member := doActionRequestAsRole(router, authn.RoleMember)
	if member.Code != http.StatusForbidden {
		t.Fatalf("member status = %d, want 403, body=%s", member.Code, member.Body.String())
	}
	if invoker.calls != 0 {
		t.Fatalf("member action invocations = %d, want 0", invoker.calls)
	}

	admin := doActionRequestAsRole(router, authn.RoleAdmin)
	if admin.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want 200, body=%s", admin.Code, admin.Body.String())
	}
	if invoker.calls != 1 {
		t.Fatalf("admin action invocations = %d, want 1", invoker.calls)
	}
}
