package copyfiles

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestParseSpecs(t *testing.T) {
	t.Parallel()
	got := ParseSpecs(".env, .env.local:symlink, config/{local,dev}.yml, config:dev, .env:, literal::symlink")
	want := []PatternSpec{
		{Pattern: ".env"},
		{Pattern: ".env.local", Symlink: true},
		{Pattern: "config/{local,dev}.yml"},
		{Pattern: "config:dev"},
		{Pattern: ".env:"},
		{Pattern: "literal:symlink"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSpecs = %#v, want %#v", got, want)
	}
}

func TestValidateSpec(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{
		"", ".env", ".env, .env.local:symlink", "config/{local,dev}.yml",
		".env:", "config:dev", ".env:hardlink", "literal::symlink",
	} {
		if err := ValidateSpec(ok); err != nil {
			t.Errorf("ValidateSpec(%q) unexpected error: %v", ok, err)
		}
	}
	for _, bad := range []string{":symlink", ".env, :symlink"} {
		if err := ValidateSpec(bad); err == nil {
			t.Errorf("ValidateSpec(%q) = nil, want error", bad)
		}
	}
}

func TestParseSpecs_DuplicateAndOverlappingPatternsUseFirstEntry(t *testing.T) {
	t.Parallel()

	got := ParseSpecs(".env:symlink, .env, .env:symlink, .*")
	want := []PatternSpec{
		{Pattern: ".env", Symlink: true},
		{Pattern: ".*"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSpecs = %#v, want %#v", got, want)
	}
}

func TestCopy_SymlinkMode_File(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation often requires privilege on Windows")
	}
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, ".env.local"), "SECRET=1", 0o644)

	copied, warnings, err := Copy(context.Background(), src, dst,
		[]PatternSpec{{Pattern: ".env.local", Symlink: true}}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	info, err := os.Lstat(filepath.Join(dst, ".env.local"))
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected a symlink, got mode %s", info.Mode())
	}
	// The link resolves to the source content.
	got, err := os.ReadFile(filepath.Join(dst, ".env.local"))
	if err != nil {
		t.Fatalf("read through symlink: %v", err)
	}
	if string(got) != "SECRET=1" {
		t.Fatalf("symlink content = %q", got)
	}
	writeFile(t, filepath.Join(src, ".env.local"), "SECRET=updated", 0o644)
	if got := readFile(t, filepath.Join(dst, ".env.local")); got != "SECRET=updated" {
		t.Fatalf("updated symlink content = %q, want live source content", got)
	}
	if len(copied) != 1 || copied[0] != ".env.local" {
		t.Fatalf("copied = %v, want [.env.local]", copied)
	}
}

func TestCopy_DuplicateAndOverlappingPatternsUseFirstEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation often requires privilege on Windows")
	}

	for _, tc := range []struct {
		name        string
		spec        string
		wantSymlink bool
	}{
		{name: "symlink before copy overlap", spec: ".env:symlink, .*", wantSymlink: true},
		{name: "copy before symlink duplicate", spec: ".*, .env:symlink", wantSymlink: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := t.TempDir()
			dst := t.TempDir()
			writeFile(t, filepath.Join(src, ".env"), "SECRET=1", 0o644)

			_, warnings, err := Copy(context.Background(), src, dst, ParseSpecs(tc.spec), nil)
			if err != nil {
				t.Fatalf("Copy err: %v", err)
			}
			if len(warnings) != 0 {
				t.Fatalf("unexpected warnings: %v", warnings)
			}
			info, err := os.Lstat(filepath.Join(dst, ".env"))
			if err != nil {
				t.Fatalf("lstat: %v", err)
			}
			if got := info.Mode()&os.ModeSymlink != 0; got != tc.wantSymlink {
				t.Fatalf("is symlink = %v, want %v", got, tc.wantSymlink)
			}
		})
	}
}

func TestCopy_EscapedSymlinkSuffixIsLiteralPath(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "config:symlink"), "literal", 0o644)

	_, warnings, err := Copy(context.Background(), src, dst, ParseSpecs("config::symlink"), nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if got := readFile(t, filepath.Join(dst, "config:symlink")); got != "literal" {
		t.Fatalf("content = %q, want literal", got)
	}
}

// Plan is the remote path; symlink entries must fall back to a byte copy there
// since a link back to the host repo can't apply on a remote executor.
func TestCopy_SymlinkMode_IgnoredInPlanMode(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, ".env.local"), "SECRET=1", 0o644)
	writeFile(t, filepath.Join(src, "config:dev"), "MODE=dev", 0o644)
	writeFile(t, filepath.Join(src, ".env:"), "TRAILING=colon", 0o644)

	entries, warnings, err := Plan(context.Background(), src,
		Parse(".env.local:symlink, config:dev, .env:"), nil)
	if err != nil {
		t.Fatalf("Plan err: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	want := map[string]string{
		".env.local": "SECRET=1",
		"config:dev": "MODE=dev",
		".env:":      "TRAILING=colon",
	}
	if len(entries) != len(want) {
		t.Fatalf("entries = %#v, want %d entries", entries, len(want))
	}
	for _, entry := range entries {
		if string(entry.Content) != want[entry.RelPath] {
			t.Fatalf("entry %q content = %q, want %q", entry.RelPath, entry.Content, want[entry.RelPath])
		}
	}
}

// A symlink entry whose destination parent is itself a symlink pointing outside
// the worktree must be rejected before MkdirAll/os.Symlink follow it out.
func TestCopy_SymlinkMode_RejectsSymlinkedParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation often requires privilege on Windows")
	}
	src := t.TempDir()
	dst := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(src, "config", ".env"), "SECRET=1", 0o644)
	// The worktree already has a symlinked `config` ancestor pointing outside.
	if err := os.Symlink(outside, filepath.Join(dst, "config")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, warnings, err := Copy(context.Background(), src, dst,
		[]PatternSpec{{Pattern: "config/.env", Symlink: true}}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 rejection warning, got %v", warnings)
	}
	// Nothing was written through the symlinked parent into the outside dir.
	if _, statErr := os.Lstat(filepath.Join(outside, ".env")); !os.IsNotExist(statErr) {
		t.Fatalf("symlink escaped through symlinked parent: %v", statErr)
	}
}
