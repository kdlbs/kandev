//go:build windows

package service

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishLocalRepositoryDoesNotReplaceExistingWindowsTarget(t *testing.T) {
	parent := t.TempDir()
	stagingPath := filepath.Join(parent, "staging")
	targetPath := filepath.Join(parent, "target")
	if err := os.Mkdir(stagingPath, 0o700); err != nil {
		t.Fatalf("Mkdir staging: %v", err)
	}
	if err := os.Mkdir(targetPath, 0o755); err != nil {
		t.Fatalf("Mkdir target: %v", err)
	}
	markerPath := filepath.Join(targetPath, "keep.txt")
	if err := os.WriteFile(markerPath, []byte("keep"), 0o644); err != nil {
		t.Fatalf("WriteFile marker: %v", err)
	}

	err := publishLocalRepository(stagingPath, targetPath)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("publishLocalRepository error = %v, want fs.ErrExist", err)
	}
	content, readErr := os.ReadFile(markerPath)
	if readErr != nil || string(content) != "keep" {
		t.Fatalf("target marker = %q, error %v; existing target was modified", content, readErr)
	}
}
