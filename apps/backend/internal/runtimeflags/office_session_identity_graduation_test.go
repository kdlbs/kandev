package runtimeflags

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestOfficeSessionIdentityDoesNotClaimUniqueIndexPrecondition pins
// AC-OFFICE-IDENTITY-GRADUATION-003.7: no operator-visible description of
// features.officeSessionIdentity may state that an unshipped
// (task_id, agent_profile_id) unique index is a precondition for enabling
// it. That precondition was dropped in favor of selection-only safety
// (REQ-OFFICE-IDENTITY-GRADUATION-003); a reintroduced claim would tell
// operators to wait on work that will never ship.
func TestOfficeSessionIdentityDoesNotClaimUniqueIndexPrecondition(t *testing.T) {
	def, ok := DefinitionByKey("features.officeSessionIdentity")
	if !ok {
		t.Fatal("features.officeSessionIdentity definition missing")
	}
	assertNoUniqueIndexClaim(t, "runtime flag registry RiskDescription", def.RiskDescription)

	repoRoot := officeSessionIdentityRepoRoot(t)
	for _, relPath := range []struct {
		path    string
		locator string
	}{
		{path: "apps/backend/internal/profiles/profiles.yaml", locator: "KANDEV_FEATURES_OFFICE_SESSION_IDENTITY"},
		{path: "docs/public/configuration.md", locator: "features.officeSessionIdentity"},
		{path: "docs/public/operations.md", locator: "Office session identity"},
	} {
		content, err := os.ReadFile(filepath.Join(repoRoot, relPath.path))
		if err != nil {
			t.Fatalf("read %s: %v", relPath.path, err)
		}
		assertNoUniqueIndexClaimInSection(t, relPath.path, string(content), relPath.locator)
	}
}

func assertNoUniqueIndexClaim(t *testing.T, surface, content string) {
	t.Helper()
	if strings.Contains(strings.ToLower(content), "unique index") ||
		strings.Contains(strings.ToLower(content), "unique-index") {
		t.Fatalf("%s still claims a unique-index precondition for features.officeSessionIdentity", surface)
	}
}

func assertNoUniqueIndexClaimInSection(t *testing.T, surface, content, locator string) {
	t.Helper()
	lowerContent := strings.ToLower(content)
	index := strings.Index(lowerContent, strings.ToLower(locator))
	if index < 0 {
		return
	}
	start := max(0, index-100)
	end := min(len(content), index+600)
	assertNoUniqueIndexClaim(t, surface, content[start:end])
}

func officeSessionIdentityRepoRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// sourceFile: apps/backend/internal/runtimeflags/<this file> -> repo root
	// is four directories up.
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../../../.."))
}
