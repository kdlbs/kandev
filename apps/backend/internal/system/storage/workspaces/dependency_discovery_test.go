package workspaces

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.11
func TestCleanupDependenciesKeepsUnclassifiedCheckout(t *testing.T) {
	provider, tasksRoot, _, _ := newDependencyProviderFixture(t)
	archived := createDependencyWorkspace(t, tasksRoot, "archived-task_abc", "archived-task")
	writeDependencyDirectory(t, filepath.Join(archived, "node_modules"), "remove")

	checkout := filepath.Join(tasksRoot, "unrelated-checkout")
	unknownDependencies := filepath.Join(checkout, "node_modules")
	writeDependencyDirectory(t, unknownDependencies, "preserve")
	for _, name := range []string{".git", "apps"} {
		if err := os.MkdirAll(filepath.Join(checkout, name), 0o755); err != nil {
			t.Fatalf("create checkout directory %s: %v", name, err)
		}
	}
	externalGuide := filepath.Join(t.TempDir(), "AGENTS.md")
	const externalPayload = "external target remains unchanged"
	if err := os.WriteFile(externalGuide, []byte(externalPayload), 0o600); err != nil {
		t.Fatalf("write external guide: %v", err)
	}
	link := filepath.Join(checkout, "CLAUDE.md")
	if err := os.Symlink(externalGuide, link); err != nil {
		t.Fatalf("symlink checkout guide: %v", err)
	}

	result, err := provider.CleanupDependencies(context.Background())
	if err != nil {
		t.Fatalf("CleanupDependencies: %v", err)
	}
	if result.Directories != 1 || result.Workspaces != 1 {
		t.Fatalf("cleanup result = %#v, want only archived workspace dependency cleanup", result)
	}
	assertDependencyRemoved(t, archived, "node_modules")
	if contents, err := os.ReadFile(filepath.Join(unknownDependencies, "payload.txt")); err != nil || string(contents) != "preserve" {
		t.Fatalf("unclassified dependencies changed: contents=%q err=%v", contents, err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("unclassified checkout symlink changed: %v", err)
	}
	if contents, err := os.ReadFile(externalGuide); err != nil || string(contents) != externalPayload {
		t.Fatalf("external guide changed: contents=%q err=%v", contents, err)
	}
	if !containsWarning(result.Warnings, "unclassified task directory kept: "+checkout) {
		t.Fatalf("cleanup warnings = %#v, want omission warning for %s", result.Warnings, checkout)
	}
}
