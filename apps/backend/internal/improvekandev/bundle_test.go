package improvekandev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
