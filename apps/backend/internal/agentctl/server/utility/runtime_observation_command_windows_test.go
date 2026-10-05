//go:build windows

package utility

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestRuntimeObservationRunsWindowsCommandShimWithSpaces(t *testing.T) {
	for _, extension := range []string{"cmd", "bat"} {
		t.Run(extension, func(t *testing.T) {
			dir := t.TempDir()
			shim := filepath.Join(dir, "Codex CLI", "codex."+extension)
			if err := os.MkdirAll(filepath.Dir(shim), 0o700); err != nil {
				t.Fatalf("create shim directory: %v", err)
			}
			if err := os.WriteFile(shim, []byte("@echo off\r\necho codex-cli 0.177.3\r\n"), 0o600); err != nil {
				t.Fatalf("write shim: %v", err)
			}
			output, err := runRuntimeObservationCommand(
				context.Background(), shim, []string{"--version"}, os.Environ(), dir, zap.NewNop(),
			)
			if err != nil {
				t.Fatalf("run command shim: %v", err)
			}
			if got := parseCodexVersion(output); got != "0.177.3" {
				t.Fatalf("parsed version = %q, want 0.177.3; output=%q", got, output)
			}
			if strings.Contains(string(output), filepath.Base(dir)) {
				t.Fatalf("command output unexpectedly included its path: %q", output)
			}
		})
	}
}
