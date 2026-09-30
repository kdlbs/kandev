package models

import (
	"reflect"
	"testing"
)

func TestUserSettingsCarriesWorkspaceSidebarLayouts(t *testing.T) {
	field, ok := reflect.TypeOf(UserSettings{}).FieldByName("SidebarLayoutsByWorkspace")
	if !ok {
		t.Fatal("UserSettings must expose SidebarLayoutsByWorkspace")
	}
	if field.Type.String() != "map[string]models.SidebarLayout" {
		t.Fatalf("SidebarLayoutsByWorkspace type = %v, want map[string]models.SidebarLayout", field.Type)
	}
}
