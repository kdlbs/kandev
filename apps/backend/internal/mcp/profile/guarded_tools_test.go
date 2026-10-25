package profile

import "testing"

func TestGuardedToolClasses(t *testing.T) {
	want := map[string]GuardedToolClass{
		"list_workspaces": ClassRead, "list_agents": ClassRead, "get_task_document": ClassRead,
		"step_complete": ClassSelf, "write_task_document": ClassSelf, "show_rich_output": ClassSelf,
		"create_task": ClassChildWrite, "move_task": ClassChildWrite, "message_task": ClassChildWrite,
		"audit_task_transfer_attempt": ClassInternal,
		"transfer_task":               ClassDenied, "archive_task": ClassDenied, "delete_task": ClassDenied,
		"invoke_plugin_tool": ClassDenied, "resolve_agent_permission": ClassDenied,
	}
	for name, class := range want {
		got, ok := GuardedToolByName(name)
		if !ok || got.Class != class {
			t.Errorf("%s: got %v ok=%v want %s", name, got.Class, ok, class)
		}
	}
	for _, name := range []string{"create_agent_profile", "update_settings", "nope", "mcp.create_task", "create_task_kandev"} {
		if _, ok := GuardedToolByName(name); ok && name != "" {
			if got, _ := GuardedToolByName(name); got.Class != ClassDenied {
				t.Errorf("%s must not be classified usable", name)
			}
		}
	}
}

func TestGuardedToolByActionStripsOnlyTheMCPPrefix(t *testing.T) {
	if got, ok := GuardedToolByAction("mcp.create_task"); !ok || got.Class != ClassChildWrite {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
	if _, ok := GuardedToolByAction("create_task"); ok {
		t.Fatal("bare action without the prefix is not an action")
	}
	if _, ok := GuardedToolByAction("mcp.mcp.create_task"); ok {
		t.Fatal("only one prefix is stripped")
	}
}

func TestGuardedToolAttributionColumns(t *testing.T) {
	cases := map[string][]string{
		"create_task":           {"source_task_id", "source_session_id"},
		"spawn_session":         {"sender_task_id", "sender_session_id"},
		"message_task":          {"sender_task_id", "sender_session_id"},
		"stop_task":             {"sender_task_id"},
		"move_task":             {"sender_session_id"},
		"add_workspace_sources": {"caller_task_id", "caller_session_id"},
		"list_related_tasks":    {"caller_task_id"},
		"update_task":           nil,
	}
	for name, attr := range cases {
		got, _ := GuardedToolByName(name)
		if len(got.Attribution) != len(attr) {
			t.Errorf("%s: got %v want %v", name, got.Attribution, attr)
		}
	}
}
