//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestForceRemoveDirUnix_CanceledContextDoesNotRunRm(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := forceRemoveDirUnix(ctx, dir)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("forceRemoveDirUnix() error = %v, want context cancellation", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("marker removed by canceled fallback: %v", err)
	}
}
