//go:build windows

package service

import (
	"context"
	"strings"
	"testing"
)

func TestListDirectoryWindowsVirtualRootListsLogicalDrives(t *testing.T) {
	got, err := (&Service{}).ListDirectory(context.Background(), "/")
	if err != nil {
		t.Fatalf("ListDirectory virtual root: %v", err)
	}
	if got.Path != "/" {
		t.Errorf("Path = %q, want virtual root /", got.Path)
	}
	if got.Parent != "" {
		t.Errorf("Parent = %q, want empty", got.Parent)
	}
	if got.Choosable {
		t.Error("Windows virtual root must not be choosable")
	}
	if len(got.Entries) == 0 {
		t.Fatal("virtual root returned no logical drives")
	}
	for _, entry := range got.Entries {
		if !strings.HasSuffix(entry.Path, `:\`) {
			t.Errorf("drive path = %q, want native absolute drive root", entry.Path)
		}
	}
}
