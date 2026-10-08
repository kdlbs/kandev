package backendapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/webapp"
)

func TestSettingsBootCarriesSavedWorkspaceIdlePolicy(t *testing.T) {
	params := bootStateParamsForTest(t)
	ctx := t.Context()
	workspaces, err := params.taskSvc.ListWorkspaces(ctx)
	if err != nil || len(workspaces) == 0 {
		t.Fatalf("list workspaces: %v", err)
	}
	workspaceID := workspaces[0].ID
	for _, enabled := range []bool{true, false} {
		timeout := 45
		if _, err := params.taskSvc.UpdateWorkspace(ctx, workspaceID, &taskservice.UpdateWorkspaceRequest{
			ACPIdleSuspensionEnabled: &enabled,
			ACPIdleTimeoutMinutes:    &timeout,
		}); err != nil {
			t.Fatalf("update policy: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/settings/workspaces/"+workspaceID+"/general", nil)
		req.AddCookie(&http.Cookie{Name: activeWorkspaceCookie, Value: workspaceID})
		state := bootInitialState(ctx, req, params, webapp.RouteClassification{
			Route: webapp.RouteSettings,
			Path:  req.URL.Path,
		})
		block, ok := state["workspaces"].(map[string]any)
		if !ok {
			t.Fatal("settings boot state has no workspaces block")
		}
		itemsJSON, err := json.Marshal(block["items"])
		if err != nil {
			t.Fatalf("marshal settings boot workspace items: %v", err)
		}
		var items []map[string]any
		if err := json.Unmarshal(itemsJSON, &items); err != nil {
			t.Fatalf("unmarshal settings boot workspace items: %v", err)
		}
		if len(items) == 0 {
			t.Fatalf("settings boot workspace items = %#v", block["items"])
		}
		if items[0]["id"] != workspaceID || items[0]["acp_idle_suspension_enabled"] != enabled || items[0]["acp_idle_timeout_minutes"] != float64(timeout) {
			t.Fatalf("boot workspace policy = %#v, want enabled %t timeout %d", items[0], enabled, timeout)
		}
	}
}
