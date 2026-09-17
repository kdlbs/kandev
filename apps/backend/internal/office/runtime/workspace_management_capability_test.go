package runtime

import (
	"net/http"
	"testing"
)

func TestWorkspaceManagementRequiresTaskCapability(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{})
	response := h.request(t, http.MethodPost, "/runtime/tasks/task-1/manage", map[string]string{"action": "start"})
	if response.Code != http.StatusForbidden {
		t.Fatalf("unprivileged management status=%d", response.Code)
	}
}
