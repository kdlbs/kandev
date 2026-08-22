package authz

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scopeConstNames maps each registered scope to the Go identifier that names
// it, so the enforcement sweep can look for real call sites rather than string
// literals.
var scopeConstNames = map[Scope]string{
	ScopeOrgMembersManage:  "ScopeOrgMembersManage",
	ScopeOrgSettingsManage: "ScopeOrgSettingsManage",
	ScopeOrgConfigManage:   "ScopeOrgConfigManage",
	ScopeWorkspaceRead:     "ScopeWorkspaceRead",
	ScopeWorkspaceManage:   "ScopeWorkspaceManage",
	ScopeTaskWrite:         "ScopeTaskWrite",
	ScopeSessionPrompt:     "ScopeSessionPrompt",
	ScopeSessionControl:    "ScopeSessionControl",
	ScopeSessionExec:       "ScopeSessionExec",
	ScopeRepositoryManage:  "ScopeRepositoryManage",
	ScopeSecretManage:      "ScopeSecretManage",
	ScopeMemberManage:      "ScopeMemberManage",
}

// TestEveryRegisteredScopeIsNamed fails when a scope is added to the registry
// without a matching entry here, which would let it slip past the enforcement
// sweep below unnoticed.
func TestEveryRegisteredScopeIsNamed(t *testing.T) {
	for _, def := range Registry() {
		if _, ok := scopeConstNames[def.Scope]; !ok {
			t.Errorf("scope %q is registered but has no constant name entry in scopeConstNames", def.Scope)
		}
	}
	if len(scopeConstNames) != len(registry) {
		t.Errorf("scopeConstNames has %d entries, registry has %d", len(scopeConstNames), len(registry))
	}
}

// TestEveryRegisteredScopeIsEnforced sweeps the backend for a use of each
// scope constant outside this package. A scope nobody enforces is a permission
// that silently does not exist, which is worse than no scope at all: it reads
// as protection in the registry and in the API response.
func TestEveryRegisteredScopeIsEnforced(t *testing.T) {
	root := backendRoot(t)
	sources := goSourcesOutsideAuthz(t, root)
	if len(sources) == 0 {
		t.Fatal("found no Go sources outside internal/authz; the sweep would pass vacuously")
	}

	for scope, constName := range scopeConstNames {
		needle := "authz." + constName
		if !anyFileContains(t, sources, needle) {
			t.Errorf("scope %q (authz.%s) has no enforcement call site outside internal/authz", scope, constName)
		}
	}
}

// TestScopeIdentifiersAreStable pins the wire values. They appear in API
// responses and are compared with ==, so a rename is a breaking change and
// must be a deliberate one.
func TestScopeIdentifiersAreStable(t *testing.T) {
	want := []string{
		"org.config.manage", "org.members.manage", "org.settings.manage",
		"member.manage", "repository.manage", "secret.manage",
		"session.control", "session.exec", "session.prompt",
		"task.write", "workspace.manage", "workspace.read",
	}
	got := make([]string, 0, len(registry))
	for _, def := range Registry() {
		got = append(got, string(def.Scope))
	}
	if len(got) != len(want) {
		t.Fatalf("registry has %d scopes, pinned list has %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("scope %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func backendRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// .../apps/backend/internal/authz -> .../apps/backend
	root := filepath.Dir(filepath.Dir(wd))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("expected go.mod at %s: %v", root, err)
	}
	return root
}

func goSourcesOutsideAuthz(t *testing.T, root string) []string {
	t.Helper()
	authzDir := filepath.Join(root, "internal", "authz")
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == "bin" || path == authzDir {
				return filepath.SkipDir
			}
			return nil
		}
		// Production call sites only. A scope referenced solely from a test
		// would otherwise look enforced while guarding nothing.
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return files
}

func anyFileContains(t *testing.T, files []string, needle string) bool {
	t.Helper()
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if strings.Contains(string(data), needle) {
			return true
		}
	}
	return false
}
