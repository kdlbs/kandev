package runtime

import (
	"net/http"
	"strings"
	"testing"
)

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-004.3
func TestRuntimeHandler_CreateTaskRejectsRemainingUnsupportedFields(t *testing.T) {
	for _, field := range []string{"priority", "workspace_group_id", "default_child_workspace", "default_child_ordering"} {
		t.Run(field, func(t *testing.T) {
			h := newRuntimeHandlerHarness(t, Capabilities{CanCreateTasks: true})

			resp := h.request(t, http.MethodPost, "/runtime/tasks", map[string]interface{}{
				"title": "Runtime task", field: "value",
			})

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), field) {
				t.Fatalf("body %s does not name %s", resp.Body.String(), field)
			}
			if h.tasks.calls != 0 {
				t.Fatalf("task creator called for unsupported %s", field)
			}
		})
	}
}

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.1
func TestRuntimeHandler_CreateTaskAcceptsBlockersAndWorkspaceMode(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanCreateTasks: true})

	resp := h.request(t, http.MethodPost, "/runtime/tasks", map[string]interface{}{
		"title": "Successor", "blocked_by": []string{"task-1"}, "workspace_mode": "new_workspace",
	})

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	if len(h.tasks.relations) != 1 {
		t.Fatalf("relations recorded = %d, want 1", len(h.tasks.relations))
	}
	got := h.tasks.relations[0]
	if strings.Join(got.BlockedBy, ",") != "task-1" || got.WorkspaceMode != "new_workspace" {
		t.Fatalf("relations = %#v", got)
	}
}

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.4
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-003.7
func TestRuntimeHandler_CreateTaskRejectsMalformedRelations(t *testing.T) {
	cases := map[string]map[string]interface{}{
		"empty blocker":     {"blocked_by": []string{""}},
		"shared group mode": {"workspace_mode": "shared_group"},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			h := newRuntimeHandlerHarness(t, Capabilities{CanCreateTasks: true})
			payload := map[string]interface{}{"title": "Join"}
			for k, v := range extra {
				payload[k] = v
			}

			resp := h.request(t, http.MethodPost, "/runtime/tasks", payload)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
			}
			if h.tasks.calls != 0 {
				t.Fatalf("task creator called for malformed relations")
			}
			assertDeniedRunEvent(t, h.runEvents, "create_task", "task", "")
		})
	}
}

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.3
func TestRuntimeHandler_CreateTaskForbidsOutOfScopeBlocker(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanCreateTasks: true})

	resp := h.request(t, http.MethodPost, "/runtime/tasks", map[string]interface{}{
		"title": "Join", "blocked_by": []string{"unrelated-task"},
	})

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "unrelated-task") {
		t.Fatalf("denial body names the blocker: %s", resp.Body.String())
	}
	if h.tasks.calls != 0 {
		t.Fatalf("task creator called after blocker denial")
	}
	assertDeniedRunEvent(t, h.runEvents, "create_task", "task", "")
}
