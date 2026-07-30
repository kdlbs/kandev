package improvekandev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateBundleDirWritesOwnerMarker(t *testing.T) {
	dir, err := createBundleDir("user-1")
	if err != nil {
		t.Fatalf("createBundleDir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if !strings.HasPrefix(filepath.Base(dir), bundlePrefix) {
		t.Errorf("bundle dir base %q must start with %q", filepath.Base(dir), bundlePrefix)
	}

	info, err := os.Stat(filepath.Join(dir, ownerMarkerName))
	if err != nil {
		t.Fatalf("owner marker: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("owner marker mode = %o, want 600", info.Mode().Perm())
	}
	if _, err := validateBundleDir(dir, "user-1"); err != nil {
		t.Fatalf("validate owner: %v", err)
	}
	if _, err := validateBundleDir(dir, "user-2"); err == nil {
		t.Fatal("different owner unexpectedly accepted")
	}
}

func TestValidateBundleDir_AcceptsValid(t *testing.T) {
	dir, err := createBundleDir("user-1")
	if err != nil {
		t.Fatalf("createBundleDir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	resolved, err := validateBundleDir(dir, "user-1")
	if err != nil {
		t.Errorf("validateBundleDir(%q): %v", dir, err)
	}
	if !strings.HasPrefix(filepath.Base(resolved), bundlePrefix) {
		t.Errorf("resolved base %q must start with %q", filepath.Base(resolved), bundlePrefix)
	}
}

func TestValidateBundleDir_RejectsBad(t *testing.T) {
	cases := []struct {
		name string
		dir  string
	}{
		{"empty", ""},
		{"home", "/etc"},
		{"wrong_prefix", filepath.Join(os.TempDir(), "not-kandev")},
		{"missing", filepath.Join(os.TempDir(), "kandev-improve-doesnotexist")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateBundleDir(tc.dir, "user-1"); err == nil {
				t.Errorf("expected error for %q", tc.dir)
			}
		})
	}
}

func TestCleanupStaleBundles_RemovesOnlyStale(t *testing.T) {
	root := t.TempDir()

	stale := filepath.Join(root, bundlePrefix+"old")
	if err := os.Mkdir(stale, 0o755); err != nil {
		t.Fatalf("mkdir stale: %v", err)
	}
	old := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("chtimes stale: %v", err)
	}

	fresh := filepath.Join(root, bundlePrefix+"new")
	if err := os.Mkdir(fresh, 0o755); err != nil {
		t.Fatalf("mkdir fresh: %v", err)
	}

	other := filepath.Join(root, "unrelated-dir")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatalf("mkdir other: %v", err)
	}
	if err := os.Chtimes(other, old, old); err != nil {
		t.Fatalf("chtimes other: %v", err)
	}

	cleanupStaleBundlesIn(root, 24*time.Hour, nil)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale bundle %q should have been removed: %v", stale, err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh bundle %q should have been preserved: %v", fresh, err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("non-bundle dir %q should have been preserved: %v", other, err)
	}
}
