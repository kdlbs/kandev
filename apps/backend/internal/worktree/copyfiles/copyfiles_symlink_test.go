package copyfiles

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseSpecs(t *testing.T) {
	t.Parallel()
	got := ParseSpecs(".env, .env.local:symlink, config/{local,dev}.yml")
	want := []PatternSpec{
		{Pattern: ".env"},
		{Pattern: ".env.local", Symlink: true},
		{Pattern: "config/{local,dev}.yml"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSpecs = %#v, want %#v", got, want)
	}
}

func TestValidateSpec(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"", ".env", ".env, .env.local:symlink", "config/{local,dev}.yml", ".env:"} {
		if err := ValidateSpec(ok); err != nil {
			t.Errorf("ValidateSpec(%q) unexpected error: %v", ok, err)
		}
	}
	for _, bad := range []string{".env:hardlink", ".env, foo:move"} {
		if err := ValidateSpec(bad); err == nil {
			t.Errorf("ValidateSpec(%q) = nil, want error", bad)
		}
	}
}

func TestCopy_SymlinkMode_File(t *testing.T) {
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
	if len(copied) != 1 || copied[0] != ".env.local" {
		t.Fatalf("copied = %v, want [.env.local]", copied)
	}
}

// Plan is the remote path; symlink entries must fall back to a byte copy there
// since a link back to the host repo can't apply on a remote executor.
func TestCopy_SymlinkMode_IgnoredInPlanMode(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, ".env.local"), "SECRET=1", 0o644)

	entries, warnings, err := Plan(context.Background(), src, Parse(".env.local:symlink"), nil)
	if err != nil {
		t.Fatalf("Plan err: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(entries) != 1 || entries[0].RelPath != ".env.local" || string(entries[0].Content) != "SECRET=1" {
		t.Fatalf("expected a copied entry with content, got %#v", entries)
	}
}
