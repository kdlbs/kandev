package settingscatalog

import "testing"

func TestSidebarPresentationPreferencesAreCallerWritable(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	domain, _ := registry.Domain("user_settings")
	for _, key := range []string{"sidebar_fast_actions_enabled", "sidebar_new_task_style"} {
		found := false
		for _, field := range domain.Fields {
			if field.Key != "user_settings."+key {
				continue
			}
			found = true
			if !field.Writable || field.Authority != "user.self" || field.SettingsHref != "/settings/preferences/layouts?tab=sidebar" {
				t.Fatalf("sidebar field = %+v", field)
			}
			if key == "sidebar_new_task_style" && field.Schema["enum"] == nil {
				t.Fatalf("style values = %v", field.Schema)
			}
		}
		if !found {
			t.Errorf("missing preference %s", key)
		}
	}
}
