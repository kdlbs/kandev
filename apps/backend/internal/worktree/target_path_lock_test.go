package worktree

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquireWorktreeTargetPath_SerializesNormalizedAliasesAndHonorsCancellation(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "shared")
	alias := filepath.Join(base, "nested", "..", "shared")
	firstKey, err := normalizedWorktreeTargetPath(target)
	if err != nil {
		t.Fatalf("normalize target: %v", err)
	}
	aliasKey, err := normalizedWorktreeTargetPath(alias)
	if err != nil {
		t.Fatalf("normalize alias: %v", err)
	}
	if firstKey != aliasKey {
		t.Fatalf("normalized keys differ: %q != %q", firstKey, aliasKey)
	}

	releaseFirst, err := acquireWorktreeTargetPath(context.Background(), target)
	if err != nil {
		t.Fatalf("acquire first target lock: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	result := make(chan error)
	go func() {
		close(started)
		release, acquireErr := acquireWorktreeTargetPath(ctx, alias)
		if acquireErr == nil {
			release()
		}
		result <- acquireErr
	}()

	<-started
	cancel()
	if acquireErr := <-result; !errors.Is(acquireErr, context.Canceled) {
		t.Fatalf("competing acquire error = %v, want context.Canceled", acquireErr)
	}
	releaseFirst()

	releaseAfterCancel, err := acquireWorktreeTargetPath(context.Background(), alias)
	if err != nil {
		t.Fatalf("target lock leaked after cancellation: %v", err)
	}
	releaseAfterCancel()
}

func TestNormalizedWorktreeTargetPathForOS_WindowsCaseAliases(t *testing.T) {
	base := t.TempDir()
	lowercasePath := filepath.Join(base, "shared", "worktree")
	uppercasePath := filepath.Join(base, "SHARED", "WORKTREE")

	lowercaseKey, err := normalizedWorktreeTargetPathForOS(lowercasePath, "windows")
	if err != nil {
		t.Fatalf("normalize lowercase Windows path: %v", err)
	}
	uppercaseKey, err := normalizedWorktreeTargetPathForOS(uppercasePath, "windows")
	if err != nil {
		t.Fatalf("normalize uppercase Windows path: %v", err)
	}
	if lowercaseKey != uppercaseKey {
		t.Fatalf("Windows-equivalent ownership paths differ: %q != %q", lowercaseKey, uppercaseKey)
	}
}
