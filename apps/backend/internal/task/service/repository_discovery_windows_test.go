package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplicitLocalRepositoryWindowsPathSemantics(t *testing.T) {
	base := t.TempDir()
	discoveryRoot := filepath.Join(base, "Discovery")
	if err := os.MkdirAll(discoveryRoot, 0o755); err != nil {
		t.Fatalf("create discovery root: %v", err)
	}

	repoPath := filepath.Join(base, "External", "Repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("create repository directory: %v", err)
	}
	cmd := exec.Command("git", "init", "-b", "main", ".")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("initialize repository: %v\n%s", err, output)
	}

	svc := newDiscoveryService(t, discoveryRoot)
	explicitPath := windowsPathVariant(repoPath, true) + "/"
	validation, err := svc.ValidateLocalRepositoryPath(context.Background(), explicitPath)
	if err != nil {
		t.Fatalf("validate explicit repository: %v", err)
	}
	if !validation.Exists || !validation.IsGitRepo || !validation.Allowed {
		t.Fatalf("expected explicit repository outside discovery roots to validate: %+v", validation)
	}
	if validation.DefaultBranch != "main" {
		t.Fatalf("expected main default branch, got %q", validation.DefaultBranch)
	}

	discovered, err := svc.DiscoverLocalRepositories(context.Background(), "")
	if err != nil {
		t.Fatalf("discover repositories: %v", err)
	}
	if len(discovered.Repositories) != 0 {
		t.Fatalf("explicit validation must not widen discovery roots: %+v", discovered.Repositories)
	}

	containedPath := filepath.Join(discoveryRoot, "Nested", "Repository")
	rootVariant := windowsPathVariant(discoveryRoot, true) + "/"
	if !isWithinRoot(containedPath, rootVariant) {
		t.Fatalf("expected mixed-case drive and separator variant to remain within %q", rootVariant)
	}
	if !isWithinRoot(windowsPathVariant(discoveryRoot, true)+"/", discoveryRoot) {
		t.Fatal("expected trailing separator and drive-letter casing to preserve path equality")
	}
	if isWithinRoot(filepath.Join(base, "Discovery-sibling"), discoveryRoot) {
		t.Fatal("sibling prefix must not be treated as contained")
	}
}

func windowsPathVariant(path string, lowercaseDrive bool) string {
	volume := filepath.VolumeName(path)
	if lowercaseDrive {
		volume = strings.ToLower(volume)
	}
	path = volume + strings.TrimPrefix(path, filepath.VolumeName(path))
	return strings.ReplaceAll(path, `\`, "/")
}
