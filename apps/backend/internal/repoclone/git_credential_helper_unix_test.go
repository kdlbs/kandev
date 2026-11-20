//go:build !windows

package repoclone

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitCredentialHelperSupportsRepeatedRequests(t *testing.T) {
	tmpDir := filepath.Join(t.TempDir(), "tmp ' credential helpers")
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		t.Fatalf("create temporary directory: %v", err)
	}
	t.Setenv("TMPDIR", tmpDir)

	cmd := exec.Command("sh", "-c", `set -e
for _ in 1 2; do
  printf 'protocol=https\nhost=github.com\npath=acme/private.git\n\n' | git credential fill
done`)
	cleanup, err := configureGitCommand(cmd, &cloneAuth{
		origin: "https://github.com", username: "x-token-auth", password: "transient-token",
	})
	if err != nil {
		t.Fatalf("configure Git credential helper: %v", err)
	}
	t.Cleanup(cleanup)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run Git credential helper twice: %v: %s", err, output)
	}
	if got := strings.Count(string(output), "username=x-token-auth\n"); got != 2 {
		t.Fatalf("credential helper returned username %d times, want 2: %q", got, output)
	}
	if got := strings.Count(string(output), "password=transient-token\n"); got != 2 {
		t.Fatalf("credential helper returned password %d times, want 2: %q", got, output)
	}
}
