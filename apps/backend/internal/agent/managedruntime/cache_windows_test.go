//go:build windows

package managedruntime

import (
	"os"
	"path/filepath"
	"testing"
)

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.3
func TestRemoveNpxExecutionTreeTreatsMissingWindowsChildAsAbsent(t *testing.T) {
	cacheRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(cacheRoot, "_npx"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := RemoveNpxExecutionTree(cacheRoot, "managed-acp@1.2.3"); err != nil {
		t.Fatalf("absent execution tree should be idempotent: %v", err)
	}
}
