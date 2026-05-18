package service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestListDirectory_ListsImmediateSubdirsOnly(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	// File should not appear in listing.
	if err := os.WriteFile(filepath.Join(root, "ignore-me.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	// Hidden directory should be excluded.
	if err := os.Mkdir(filepath.Join(root, ".hidden"), 0o755); err != nil {
		t.Fatalf("mkdir hidden: %v", err)
	}
	// Nested dir should NOT appear (immediate children only).
	if err := os.MkdirAll(filepath.Join(root, "alpha", "deep"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	svc := &Service{}
	got, err := svc.ListDirectory(context.Background(), root)
	if err != nil {
		t.Fatalf("ListDirectory: %v", err)
	}

	want := []string{"alpha", "beta", "gamma"}
	if len(got.Entries) != len(want) {
		t.Fatalf("entries: got %d, want %d (%+v)", len(got.Entries), len(want), got.Entries)
	}
	for i, e := range got.Entries {
		if e.Name != want[i] {
			t.Errorf("entry[%d].Name = %q; want %q", i, e.Name, want[i])
		}
	}
	// A t.TempDir is not the filesystem root, so the parent should be set.
	if got.Parent == "" {
		t.Errorf("expected parent to be set for nested path, got empty")
	}
}

func TestListDirectory_DefaultsToHome(t *testing.T) {
	svc := &Service{}
	got, err := svc.ListDirectory(context.Background(), "")
	if err != nil {
		t.Fatalf("ListDirectory: %v", err)
	}
	home, _ := os.UserHomeDir()
	if got.Path != filepath.Clean(home) {
		t.Errorf("got Path = %q; want home %q", got.Path, home)
	}
}

func TestListDirectory_RejectsNonDirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	svc := &Service{}
	_, err := svc.ListDirectory(context.Background(), file)
	if err == nil {
		t.Fatalf("expected error for non-directory path, got nil")
	}
}

func TestListDirectory_BrowsesOutsideHome(t *testing.T) {
	// The picker deliberately lets users browse any directory the kandev
	// process has read access to, not just $HOME or the discoveryRoots.
	// Pick a canonical "outside $HOME but accessible" path per platform.
	target := "/tmp"
	if runtime.GOOS == "windows" {
		target = os.Getenv("SystemDrive") + `\` // e.g. "C:\"
		if target == `\` {
			target = `C:\`
		}
	}
	svc := &Service{}
	got, err := svc.ListDirectory(context.Background(), target)
	if err != nil {
		t.Fatalf("ListDirectory(%q): %v", target, err)
	}
	if got.Path != filepath.Clean(target) {
		t.Errorf("got Path = %q; want %q", got.Path, filepath.Clean(target))
	}
}

func TestSplitAbsForRoot(t *testing.T) {
	// Each case spells out the exact OS it applies to so the test runs the
	// right branches no matter where the suite executes. filepath.VolumeName
	// only returns non-empty on Windows, so the Windows-shape cases are
	// behaviour-noisy on Unix and vice versa — we filter accordingly.
	cases := []struct {
		name     string
		os       string // "windows", "unix", or "" for both
		in       string
		wantRoot string
		wantRel  string
	}{
		{name: "unix-typical", os: "unix", in: "/home/user", wantRoot: "/", wantRel: "home/user"},
		{name: "unix-root", os: "unix", in: "/", wantRoot: "/", wantRel: "."},
		{name: "windows-drive-typical", os: "windows", in: `C:\Users\carlo`, wantRoot: `C:\`, wantRel: `Users\carlo`},
		{name: "windows-drive-root", os: "windows", in: `C:\`, wantRoot: `C:\`, wantRel: "."},
		{name: "windows-unc", os: "windows", in: `\\server\share\sub`, wantRoot: `\\server\share\`, wantRel: "sub"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.os == "windows" && runtime.GOOS != "windows" {
				t.Skip("windows-only path shape")
			}
			if tc.os == "unix" && runtime.GOOS == "windows" {
				t.Skip("unix-only path shape")
			}
			gotRoot, gotRel := splitAbsForRoot(tc.in)
			if gotRoot != tc.wantRoot {
				t.Errorf("rootPath = %q; want %q", gotRoot, tc.wantRoot)
			}
			if gotRel != tc.wantRel {
				t.Errorf("rel = %q; want %q", gotRel, tc.wantRel)
			}
		})
	}
}

func TestListDirectory_ParentEmptyAtFilesystemRoot(t *testing.T) {
	target := "/"
	if runtime.GOOS == "windows" {
		target = os.Getenv("SystemDrive") + `\`
		if target == `\` {
			target = `C:\`
		}
	}
	svc := &Service{}
	got, err := svc.ListDirectory(context.Background(), target)
	if err != nil {
		t.Fatalf("ListDirectory(%q): %v", target, err)
	}
	if got.Parent != "" {
		t.Errorf("expected empty parent at %q, got %q", target, got.Parent)
	}
}
