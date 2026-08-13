package plugintools

import "testing"

func TestExposedNameIsReadableAndProviderSafe(t *testing.T) {
	got := ExposedName("task-tags.v1", "add_tag")
	if got != "kandev_task_tags_v1_add_tag" {
		t.Fatalf("ExposedName() = %q", got)
	}
	if len(got) > 64 {
		t.Fatalf("ExposedName() length = %d, want <= 64", len(got))
	}
}

func TestExposedNameUsesHashOnlyForLongPluginIDs(t *testing.T) {
	got := ExposedName("a-very-long-plugin-id-that-exceeds-the-readable-name-budget", "run")
	if len(got) > 64 {
		t.Fatalf("ExposedName() length = %d, want <= 64", len(got))
	}
	if got == "kandev_a_very_long_plugin_id_that_exceeds_the_readable_name_budget_run" {
		t.Fatal("long exposed name was not bounded")
	}
	if got[len(got)-9] != '_' {
		t.Fatalf("long exposed name = %q, want hash suffix", got)
	}
}

func TestNormalizeSortsToolsAndSurfaces(t *testing.T) {
	got := Normalize(Snapshot{Tools: []Definition{
		{ExposedName: "b", Surfaces: []string{SurfaceOffice, SurfaceKanban}},
		{ExposedName: "a", Surfaces: []string{SurfaceOffice}},
	}})
	if got.Tools[0].ExposedName != "a" || got.Tools[1].ExposedName != "b" {
		t.Fatalf("tools not sorted: %+v", got.Tools)
	}
	if got.Tools[1].Surfaces[0] != SurfaceKanban {
		t.Fatalf("surfaces not sorted: %+v", got.Tools[1].Surfaces)
	}
}
