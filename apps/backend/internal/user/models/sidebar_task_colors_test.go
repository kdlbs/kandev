package models

import (
	"strings"
	"testing"
)

func TestValidateSidebarTaskColorPatchAcceptsColorsAndTombstones(t *testing.T) {
	red := "red"
	patch := SidebarTaskColorPatch{
		Colors: map[string]*string{
			"task-red":     &red,
			"task-cleared": nil,
		},
		IfMissing: true,
	}

	if err := ValidateSidebarTaskColorPatch(patch); err != nil {
		t.Fatalf("ValidateSidebarTaskColorPatch: %v", err)
	}
}

func TestValidateSidebarTaskColorPatchRejectsInvalidEntriesAndOversizedPatch(t *testing.T) {
	blue := "blue"
	tests := []struct {
		name  string
		patch SidebarTaskColorPatch
	}{
		{
			name: "unsupported color",
			patch: SidebarTaskColorPatch{Colors: map[string]*string{
				"task-1": func() *string { value := "gray"; return &value }(),
			}},
		},
		{
			name:  "empty task id",
			patch: SidebarTaskColorPatch{Colors: map[string]*string{"": &blue}},
		},
		{
			name: "task id exceeds byte limit",
			patch: SidebarTaskColorPatch{Colors: map[string]*string{
				strings.Repeat("a", MaxSidebarTaskColorTaskIDBytes+1): &blue,
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSidebarTaskColorPatch(tt.patch); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}

	tooMany := make(map[string]*string, MaxSidebarTaskColorPatchEntries+1)
	for index := 0; index <= MaxSidebarTaskColorPatchEntries; index++ {
		tooMany["task-"+string(rune('a'+index%26))+string(rune(index/26))] = &blue
	}
	if err := ValidateSidebarTaskColorPatch(SidebarTaskColorPatch{Colors: tooMany}); err == nil {
		t.Fatal("expected oversized patch validation error")
	}
}

func TestValidateSidebarTaskColorsRejectsOversizedStoredMap(t *testing.T) {
	red := "red"
	colors := make(map[string]*string, MaxSidebarTaskColors)
	for index := 0; index < MaxSidebarTaskColors; index++ {
		colors[strings.Repeat("x", MaxSidebarTaskColorTaskIDBytes-8)+string(rune(index))] = &red
	}

	if err := ValidateSidebarTaskColors(colors); err == nil {
		t.Fatal("expected encoded stored-map size validation error")
	}
}

func TestCloneSidebarTaskColorsDoesNotAliasPointers(t *testing.T) {
	red := "red"
	original := map[string]*string{"task-1": &red, "task-cleared": nil}
	clone := CloneSidebarTaskColors(original)

	if clone["task-1"] == original["task-1"] {
		t.Fatal("clone aliased the color pointer")
	}
	*clone["task-1"] = "blue"
	if *original["task-1"] != "red" {
		t.Fatalf("original color = %q, want red", *original["task-1"])
	}
	if _, ok := clone["task-cleared"]; !ok || clone["task-cleared"] != nil {
		t.Fatalf("clone tombstone = %#v, want present nil", clone["task-cleared"])
	}
}
