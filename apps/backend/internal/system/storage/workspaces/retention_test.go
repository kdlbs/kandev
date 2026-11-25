package workspaces

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestQuarantineRechecksRetentionAfterCandidateScan(t *testing.T) {
	provider, root, store := newProviderFixture(t, Inventory{Complete: true}, nil)
	owner := OwnershipMarker{TaskID: "retained", WorkspaceID: "workspace", TaskDirName: "retained_abc", LayoutVersion: LayoutVersionSemantic}
	directory := createOwnedCandidate(t, root, "retained_abc", owner)
	file := filepath.Join(directory, "keep.txt")
	if err := os.WriteFile(file, []byte("keep this content"), 0o600); err != nil {
		t.Fatal(err)
	}
	candidateBeforeRetention := candidate{path: directory, owner: owner}
	provider.config.Inventory = fakeInventorySource{inventory: Inventory{Complete: true, WorktreePaths: []string{filepath.Join(directory, "repo")}}}
	if _, err := provider.quarantine(context.Background(), candidateBeforeRetention); !errors.Is(err, ErrActiveWorktree) {
		t.Fatalf("quarantine error = %v", err)
	}
	contents, err := os.ReadFile(file)
	if err != nil || string(contents) != "keep this content" {
		t.Fatalf("retained file changed: %q, %v", contents, err)
	}
	if len(store.entries) != 0 {
		t.Fatal("retained directory entered quarantine")
	}
}
