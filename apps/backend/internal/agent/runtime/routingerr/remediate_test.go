package routingerr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestRemediateNpxCache_RemovesHashDirAndPreservesParent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cacheRoot := filepath.Join(home, ".npm", "_npx")
	hashDir := filepath.Join(cacheRoot, "d820eb7d96bc2600")
	if err := os.MkdirAll(filepath.Join(hashDir, "node_modules", "@anthropic-ai", "claude-agent-sdk-darwin-arm64"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := RemediateNpxCache(hashDir, zap.NewNop()); err != nil {
		t.Fatalf("RemediateNpxCache: %v", err)
	}

	if _, err := os.Stat(hashDir); !os.IsNotExist(err) {
		t.Errorf("hashDir still exists, err = %v", err)
	}
	// _npx parent should survive (we only wipe a specific hash dir).
	if _, err := os.Stat(cacheRoot); err != nil {
		t.Errorf("_npx parent should still exist, err = %v", err)
	}
}

func TestRemediateNpxCache_IdempotentWhenAbsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create the _npx root but not the hash dir.
	if err := os.MkdirAll(filepath.Join(home, ".npm", "_npx"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	absent := filepath.Join(home, ".npm", "_npx", "deadbeefcafef00d")
	if err := RemediateNpxCache(absent, zap.NewNop()); err != nil {
		t.Errorf("RemediateNpxCache on absent path = %v, want nil", err)
	}
}

func TestRemediateNpxCache_RejectsPrefixOnlySibling(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// _npxFOO is NOT a child of _npx/ — the trailing-separator guard must reject it.
	bad := filepath.Join(home, ".npm", "_npxFOO", "anything")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := RemediateNpxCache(bad, zap.NewNop())
	if err == nil {
		t.Fatal("expected guard error, got nil")
	}
	if !strings.Contains(err.Error(), "outside") {
		t.Errorf("error = %v, want one mentioning 'outside'", err)
	}
	if _, statErr := os.Stat(bad); statErr != nil {
		t.Errorf("guard should not delete the path, stat err = %v", statErr)
	}
}

func TestRemediateNpxCache_RejectsPathOutsideNpxCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	bad := filepath.Join(home, "etc")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := RemediateNpxCache(bad, zap.NewNop())
	if err == nil {
		t.Fatal("expected guard error, got nil")
	}
	if !strings.Contains(err.Error(), "outside") {
		t.Errorf("error = %v, want one mentioning 'outside'", err)
	}
	if _, statErr := os.Stat(bad); statErr != nil {
		t.Errorf("guard should not delete the path, stat err = %v", statErr)
	}
}

func TestRemediateNpxCache_RejectsEmptyPath(t *testing.T) {
	if err := RemediateNpxCache("", zap.NewNop()); err == nil {
		t.Fatal("expected error for empty path")
	}
}
