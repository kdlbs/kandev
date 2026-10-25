package store

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func TestSidebarWorkspaceSettingsRoundTrip(t *testing.T) {
	raw := `{"sidebar_workspace_version":1,"sidebar_views_by_workspace":{"a":{"views":[{"id":"shared","name":"A","filters":[],"sort":{"key":"state","direction":"asc"},"group":"repository","collapsed_groups":["repo-a"]}],"active_view_id":"shared","draft":null},"b":{"views":[{"id":"shared","name":"B","filters":[],"sort":{"key":"title","direction":"desc"},"group":"none","collapsed_groups":[]}],"active_view_id":"shared","draft":null}}}`
	settings, err := scanUserSettings(settingsScanner{raw: raw}, DefaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := marshalUserSettingsPayload(settings)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if string(got["sidebar_workspace_version"]) != "1" {
		t.Fatalf("migration version lost: %s", got["sidebar_workspace_version"])
	}
	var entries map[string]struct {
		Views []struct {
			Name string `json:"name"`
		} `json:"views"`
	}
	if err := json.Unmarshal(got["sidebar_views_by_workspace"], &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries["a"].Views[0].Name != "A" || entries["b"].Views[0].Name != "B" {
		t.Fatalf("workspace views lost: %+v", entries)
	}
}

func TestSidebarWorkspaceSortAndGroupIndentRoundTrip(t *testing.T) {
	raw := `{"sidebar_workspace_version":1,"sidebar_views_by_workspace":{"a":{"views":[{"id":"chain","name":"Chain","filters":[],"sort":{"key":"running","direction":"desc","then_by":[{"key":"color","color":"red","direction":"desc"},{"key":"lastActivityAt","direction":"desc"}]},"group":"repository","group_indent":false,"collapsed_groups":[]}],"active_view_id":"chain","draft":{"base_view_id":"chain","filters":[],"sort":{"key":"title","direction":"asc","then_by":[{"key":"running","direction":"desc"}]},"group":"state","group_indent":false}}}}`
	settings, err := scanUserSettings(settingsScanner{raw: raw}, DefaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := marshalUserSettingsPayload(settings)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Workspaces map[string]struct {
			Views []models.SidebarView     `json:"views"`
			Draft *models.SidebarViewDraft `json:"draft"`
		} `json:"sidebar_views_by_workspace"`
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	view := got.Workspaces["a"].Views[0]
	if view.GroupIndent == nil || *view.GroupIndent {
		t.Fatalf("explicit false group indent was lost: %+v", view.GroupIndent)
	}
	if len(view.Sort.ThenBy) != 2 || view.Sort.ThenBy[0].Color != "red" {
		t.Fatalf("sort chain was lost: %+v", view.Sort)
	}
	draft := got.Workspaces["a"].Draft
	if draft == nil || draft.GroupIndent == nil || *draft.GroupIndent || len(draft.Sort.ThenBy) != 1 {
		t.Fatalf("draft settings were lost: %+v", draft)
	}
}
