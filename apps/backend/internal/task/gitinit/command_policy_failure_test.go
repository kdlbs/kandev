//go:build linux || darwin

package gitinit

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCommitCommandPolicyFailureExitsNonZeroWithCause(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "filter.evil.clean", "cat"},
		{"config", "filter.evil.required", "true"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	directory, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = directory.Close() })

	cmd, err := CommitCommandContext(context.Background(), dir, directory, "add", "-A")
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected a policy failure, got success: %s", out)
	}
	if !strings.Contains(string(out), "required but not permitted") || !strings.Contains(string(out), "evil") {
		t.Fatalf("output does not name the cause: %s", out)
	}
}
