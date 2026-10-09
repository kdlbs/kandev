package service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFolderOpeningAvailable(t *testing.T) {
	for _, tc := range []struct{ platform, command string }{
		{"darwin", "open"}, {"linux", "xdg-open"}, {"windows", "explorer"}, {"other", ""},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			if got := folderOpenCommand(tc.platform); got != tc.command {
				t.Fatalf("command = %q, want %q", got, tc.command)
			}
		})
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if FolderOpeningAvailable() {
		t.Fatal("missing opener reported available")
	}
	svc := &Service{}
	if err := svc.OpenFolder(context.Background(), "session", ""); err != ErrFolderUnavailable {
		t.Fatalf("missing opener error = %v", err)
	}
	command := folderOpenCommand(runtime.GOOS)
	if command == "" {
		return
	}
	if runtime.GOOS == "windows" {
		command += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, command), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if !FolderOpeningAvailable() {
		t.Fatal("installed opener reported unavailable")
	}
}
