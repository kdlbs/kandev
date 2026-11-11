package backendapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.1, AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.2, AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.3
func TestManagedAttachmentCompatibilityConsumers(t *testing.T) {
	for _, route := range []string{"http-source", "mcp-source", "http-local-source", "mcp-local-source", "mcp-branch"} {
		for _, scenario := range []string{"scoped-legacy", "alias-empty", "alias-missing", "alias-valid", "space-missing", "space-valid", "newline-missing", "newline-valid"} {
			t.Run(route+"/"+scenario, func(t *testing.T) { managedAttachment(t, route, scenario) })
		}
	}
}

func managedAttachmentCloneBase(t *testing.T, scenario string) string {
	t.Helper()
	root := canonicalTempDir(t)
	if strings.HasPrefix(scenario, "space-") || strings.HasPrefix(scenario, "newline-") {
		probeManagedAttachmentPathName(t, root, managedAttachmentProviderName(scenario))
	}
	if !strings.HasPrefix(scenario, "alias-") {
		return root
	}
	physical := filepath.Join(root, "physical")
	if err := os.Mkdir(physical, 0755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(physical, alias); err != nil {
		t.Skipf("parent aliases unavailable: %v", err)
	}
	return filepath.Join(alias, "clones")
}

func managedAttachmentProviderName(scenario string) string {
	if strings.HasPrefix(scenario, "space-") {
		return "added \t"
	}
	if strings.HasPrefix(scenario, "newline-") {
		return "added \n"
	}
	return "added"
}

func verifyManagedAttachmentCompatibility(t *testing.T, scenario, saved, managed, origin, content string) {
	t.Helper()
	if !strings.HasPrefix(scenario, "alias-") && !strings.HasPrefix(scenario, "space-") && !strings.HasPrefix(scenario, "newline-") {
		return
	}
	if saved != managed {
		t.Fatalf("lexical clone path changed: saved=%q want=%q", saved, managed)
	}
	physical, err := filepath.EvalSymlinks(managed)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(origin, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(managed, "README.md"), filepath.Join(physical, "README.md"), content} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(want) {
			t.Fatalf("content at %q = %q: %v", path, got, err)
		}
	}
}

func probeManagedAttachmentPathName(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.Mkdir(path, 0755); err != nil {
		t.Skipf("filesystem cannot create exact whitespace name: %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if info.Name() != name {
		t.Skip("filesystem normalizes trailing whitespace")
	}
}
