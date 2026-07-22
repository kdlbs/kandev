package worktree

import (
	"errors"
	"testing"
)

func TestRemoveDirWithRetries_ReturnsPortableFilesystemError(t *testing.T) {
	mgr := &Manager{logger: newTestLogger()}
	wantErr := errors.New("filesystem removal failed")
	removeCalls := 0

	err := mgr.removeDirWithRetries("worktree", 3, 0, func(string) error {
		removeCalls++
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("removeDirWithRetries() error = %v, want wrapped filesystem error", err)
	}
	if removeCalls != 3 {
		t.Fatalf("remove function calls = %d, want 3", removeCalls)
	}
}
