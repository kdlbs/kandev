package store

import (
	"encoding/json"
	"testing"
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
