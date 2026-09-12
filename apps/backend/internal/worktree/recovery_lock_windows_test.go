//go:build windows

package worktree

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryOperationLockPinsClaimPathAgainstReplacement(t *testing.T) {
	claimPath := filepath.Join(t.TempDir(), "recovery.claim")
	first, err := acquireRecoveryOperation(claimPath)
	if err != nil {
		t.Fatalf("first recovery claim: %v", err)
	}
	defer func() { _ = first.Close() }()

	if err := os.Remove(claimPath); err == nil {
		t.Fatal("Remove succeeded while recovery claim was held")
	}
	if err := os.Rename(claimPath, claimPath+".replaced"); err == nil {
		t.Fatal("Rename succeeded while recovery claim was held")
	}

	second, err := acquireRecoveryOperation(claimPath)
	if second != nil {
		_ = second.Close()
		t.Fatal("replacement recovery claim unexpectedly acquired the lock")
	}
	if !errors.Is(err, errRecoveryOperationClaimed) {
		t.Fatalf("second recovery claim error = %v, want errRecoveryOperationClaimed", err)
	}
}
