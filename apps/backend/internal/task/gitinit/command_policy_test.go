//go:build linux || darwin

package gitinit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCommitCommandRunsNoRepositorySuppliedCode(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "marker")
	hooks := t.TempDir()
	hook := "#!/bin/sh\necho ran >> '" + marker + "'\n"
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	filter := filepath.Join(t.TempDir(), "filter.sh")
	if err := os.WriteFile(filter, []byte("#!/bin/sh\necho filter >> '"+marker+"'\ncat\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("init: %v %s", err, out)
	}
	for _, kv := range [][2]string{{"core.hooksPath", hooks}, {"filter.x.clean", filter}} {
		if out, err := exec.Command("git", "-C", dir, "config", kv[0], kv[1]).CombinedOutput(); err != nil {
			t.Fatalf("config: %v %s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("*.txt filter=x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = directory.Close() })

	addCmd, err := CommitCommandContext(context.Background(), dir, directory, "add", "-A")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("add: %v %s", err, out)
	}
	commitCmd, err := CommitCommandContext(context.Background(), dir, directory, "commit", "-m", "initial")
	if err != nil {
		t.Fatal(err)
	}
	commitCmd.Env = append(commitCmd.Env, "GIT_AUTHOR_NAME=n", "GIT_AUTHOR_EMAIL=n@example.com", "GIT_COMMITTER_NAME=n", "GIT_COMMITTER_EMAIL=n@example.com")
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v %s", err, out)
	}
	if _, err := os.Stat(marker); err == nil {
		data, _ := os.ReadFile(marker)
		t.Fatalf("repository-supplied code ran in the initial commit:\n%s", data)
	}
}
