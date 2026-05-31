package agents

import (
	"slices"
	"testing"
)

func TestOpenCodeACPRemoteAuth(t *testing.T) {
	auth := NewOpenCodeACP().RemoteAuth()
	if auth == nil {
		t.Fatal("RemoteAuth() returned nil; expected files-based auth method")
	}
	if len(auth.Methods) != 1 {
		t.Fatalf("Methods len = %d, want 1", len(auth.Methods))
	}
	m := auth.Methods[0]
	if m.Type != "files" {
		t.Errorf("Type = %q, want %q", m.Type, "files")
	}
	if m.TargetRelDir != ".local/share/opencode" {
		t.Errorf("TargetRelDir = %q, want %q", m.TargetRelDir, ".local/share/opencode")
	}
	want := []string{".local/share/opencode/auth.json"}
	for _, os := range []string{"darwin", "linux"} {
		got := m.SourceFiles[os]
		if !slices.Equal(got, want) {
			t.Errorf("SourceFiles[%q] = %v, want %v", os, got, want)
		}
	}
}
