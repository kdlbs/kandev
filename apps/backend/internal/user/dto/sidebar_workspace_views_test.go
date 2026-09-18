package dto

import (
	"encoding/json"
	"github.com/kandev/kandev/internal/user/models"
	"testing"
)

func TestSidebarWorkspaceDTOProjection(t *testing.T) {
	got := FromUserSettings(&models.UserSettings{SidebarViewsByWorkspace: map[string]models.SidebarWorkspaceState{"a": {Views: []models.SidebarView{{ID: "one", Name: "A"}}, ActiveViewID: "one"}}})
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["sidebar_views_by_workspace"]) == "" {
		t.Fatal("workspace view projection missing")
	}
}
