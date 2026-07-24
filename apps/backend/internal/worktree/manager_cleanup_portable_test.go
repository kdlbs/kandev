package worktree

import (
	"context"
	"errors"
	"testing"
)

func TestRemoveDirWithRetries_ReturnsPortableFilesystemError(t *testing.T) {
	mgr := &Manager{logger: newTestLogger()}
	wantErr := errors.New("filesystem removal failed")
	removeCalls := 0

	err := mgr.removeDirWithRetries(context.Background(), "worktree", 3, 0, func(string) error {
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

func TestRemoveDirWithRetriesAndUnixFallback_RetriesBeforeFallback(t *testing.T) {
	mgr := &Manager{logger: newTestLogger()}
	wantErr := errors.New("filesystem removal failed")
	removeCalls := 0
	fallbackCalls := 0

	err := mgr.removeDirWithRetriesAndFallback(context.Background(), "worktree", 3, 0,
		func(string) error {
			removeCalls++
			return wantErr
		},
		func(context.Context, string) error {
			fallbackCalls++
			return nil
		},
		true,
	)
	if err != nil {
		t.Fatalf("removeDirWithRetriesAndFallback() error = %v, want nil", err)
	}
	if removeCalls != 3 {
		t.Fatalf("remove function calls = %d, want 3", removeCalls)
	}
	if fallbackCalls != 1 {
		t.Fatalf("Unix fallback calls = %d, want 1", fallbackCalls)
	}
}

func TestRemoveDirWithRetriesAndUnixFallback_SkipsFallbackOnWindows(t *testing.T) {
	mgr := &Manager{logger: newTestLogger()}
	wantErr := errors.New("filesystem removal failed")
	fallbackCalls := 0

	err := mgr.removeDirWithRetriesAndFallback(context.Background(), "worktree", 1, 0,
		func(string) error { return wantErr },
		func(context.Context, string) error {
			fallbackCalls++
			return nil
		},
		false,
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("removeDirWithRetriesAndFallback() error = %v, want wrapped filesystem error", err)
	}
	if fallbackCalls != 0 {
		t.Fatalf("Unix fallback calls = %d, want 0", fallbackCalls)
	}
}

func TestRemoveDirWithRetriesAndUnixFallback_PassesContextToFallback(t *testing.T) {
	mgr := &Manager{logger: newTestLogger()}
	ctx := context.Background()
	fallbackCalled := false

	err := mgr.removeDirWithRetriesAndFallback(ctx, "worktree", 1, 0,
		func(string) error { return errors.New("filesystem removal failed") },
		func(got context.Context, _ string) error {
			if got != ctx {
				t.Fatal("fallback received a different context")
			}
			fallbackCalled = true
			return nil
		},
		true,
	)
	if err != nil {
		t.Fatalf("removeDirWithRetriesAndFallback() error = %v, want nil", err)
	}
	if !fallbackCalled {
		t.Fatal("fallback was not called")
	}
}

func TestRemoveDirWithRetriesAndUnixFallback_CanceledContextSkipsRemoval(t *testing.T) {
	mgr := &Manager{logger: newTestLogger()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	removeCalls := 0
	fallbackCalls := 0

	err := mgr.removeDirWithRetriesAndFallback(ctx, "worktree", 1, 0,
		func(string) error {
			removeCalls++
			return errors.New("filesystem removal failed")
		},
		func(context.Context, string) error {
			fallbackCalls++
			return nil
		},
		true,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("removeDirWithRetriesAndFallback() error = %v, want context cancellation", err)
	}
	if removeCalls != 0 {
		t.Fatalf("remove function calls = %d, want 0", removeCalls)
	}
	if fallbackCalls != 0 {
		t.Fatalf("Unix fallback calls = %d, want 0", fallbackCalls)
	}
}
