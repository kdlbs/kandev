package store

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func TestQuickChatTabOrderSettingsRoundTrip(t *testing.T) {
	want := map[string][]string{
		"workspace-a": {"conversation:one", "terminal:one"},
		"workspace-b": {"conversation:two"},
	}
	raw, err := marshalUserSettingsPayload(&models.UserSettings{
		QuickChatTabOrderByWorkspace: want,
	})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if _, ok := payload["quick_chat_tab_order_by_workspace"]; !ok {
		t.Fatal("serialized settings omitted quick_chat_tab_order_by_workspace")
	}

	got, err := scanUserSettings(settingsScanner{raw: string(raw)}, DefaultUserID)
	if err != nil {
		t.Fatalf("scan settings: %v", err)
	}
	if len(got.QuickChatTabOrderByWorkspace) != len(want) {
		t.Fatalf("workspace order count = %d, want %d", len(got.QuickChatTabOrderByWorkspace), len(want))
	}
	for workspaceID, refs := range want {
		if got.QuickChatTabOrderByWorkspace[workspaceID] == nil {
			t.Fatalf("workspace %q order is missing", workspaceID)
		}
		if len(got.QuickChatTabOrderByWorkspace[workspaceID]) != len(refs) {
			t.Fatalf("workspace %q refs = %#v, want %#v", workspaceID, got.QuickChatTabOrderByWorkspace[workspaceID], refs)
		}
		for index, ref := range refs {
			if got.QuickChatTabOrderByWorkspace[workspaceID][index] != ref {
				t.Fatalf("workspace %q ref[%d] = %q, want %q", workspaceID, index, got.QuickChatTabOrderByWorkspace[workspaceID][index], ref)
			}
		}
	}
}

func TestDefaultUserSettingsQuickChatTabOrderIsEmpty(t *testing.T) {
	settings := defaultUserSettings(DefaultUserID)
	if settings.QuickChatTabOrderByWorkspace == nil {
		t.Fatal("default quick-chat tab order map is nil")
	}
	if len(settings.QuickChatTabOrderByWorkspace) != 0 {
		t.Fatalf("default quick-chat tab order = %#v, want empty", settings.QuickChatTabOrderByWorkspace)
	}
}
