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

func TestRemoveDirWithRetriesAndUnixFallback_RetriesBeforeFallback(t *testing.T) {
	mgr := &Manager{logger: newTestLogger()}
	wantErr := errors.New("filesystem removal failed")
	removeCalls := 0
	fallbackCalls := 0

	err := mgr.removeDirWithRetriesAndFallback("worktree", 3, 0,
		func(string) error {
			removeCalls++
			return wantErr
		},
		func(string) error {
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

	err := mgr.removeDirWithRetriesAndFallback("worktree", 1, 0,
		func(string) error { return wantErr },
		func(string) error {
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
