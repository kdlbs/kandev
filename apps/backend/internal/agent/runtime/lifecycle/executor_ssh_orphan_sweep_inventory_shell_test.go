//go:build !windows

package lifecycle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSHOrphanInventoryUsesPOSIXShellForEmptyGlobs(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}

	for _, tc := range []struct {
		name        string
		makeTaskDir bool
	}{
		{name: "empty root"},
		{name: "task without session pidfiles", makeTaskDir: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.makeTaskDir {
				taskStateDir := filepath.Join(root, "tasks", "task-1.kandev", "sessions")
				if err := os.MkdirAll(taskStateDir, 0o755); err != nil {
					t.Fatalf("create task state directory: %v", err)
				}
			}

			command := sshOrphanInventoryCommand(root)
			if !strings.HasPrefix(command, "sh -c ") {
				t.Fatalf("inventory command = %q, want an explicit POSIX shell boundary", command)
			}
			cmd := exec.Command(sh, "-c", command)
			cmd.Env = os.Environ()
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("inventory failed on unmatched globs: %v\n%s", err, output)
			} else if len(output) != 0 {
				t.Fatalf("inventory output = %q, want empty output", output)
			}
		})
	}
}
