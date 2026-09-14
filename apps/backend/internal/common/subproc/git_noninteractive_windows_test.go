//go:build windows

package subproc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsManagedGitAskpassIsExecutedAndDenied(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ambient-askpass.cmd")
	if err := os.WriteFile(marker, []byte("@echo off\r\necho marker>\""+filepath.Join(t.TempDir(), "invoked")+"\"\r\nexit /b 0\r\n"), 0o700); err != nil {
		t.Fatalf("write ambient askpass: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := NewGitCommand(ctx, "credential", "fill")
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=1",
		"GIT_ASKPASS=" + marker,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=!false",
	}
	cmd.Stdin = strings.NewReader("protocol=https\nhost=example.invalid\n\n")

	output, err := RunGitCombinedOutputClass(ctx, GitInteractive, cmd)
	if err == nil {
		t.Fatalf("credential fill unexpectedly succeeded: %s", output)
	}
	if !strings.Contains(strings.ToLower(string(output)), "exit 1") {
		t.Fatalf("Git did not execute the managed deny-only askpass command: %s", output)
	}
}
