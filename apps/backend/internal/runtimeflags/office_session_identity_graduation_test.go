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
	for _, relPath := range []string{
		"apps/backend/internal/profiles/profiles.yaml",
		"docs/public/configuration.md",
		"docs/public/operations.md",
	} {
		content, err := os.ReadFile(filepath.Join(repoRoot, relPath))
		if err != nil {
			t.Fatalf("read %s: %v", relPath, err)
		}
		assertNoUniqueIndexClaim(t, relPath, string(content))
	}
}

func assertNoUniqueIndexClaim(t *testing.T, surface, content string) {
	t.Helper()
	if strings.Contains(strings.ToLower(content), "unique index") ||
		strings.Contains(strings.ToLower(content), "unique-index") {
		t.Fatalf("%s still claims a unique-index precondition for features.officeSessionIdentity", surface)
	}
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
