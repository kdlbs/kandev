package nomad

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPoolAllocationDirRemoveDeletesOnlyThisJobDirectory(t *testing.T) {
	root := t.TempDir()
	s := poolFenceState(t, root, "env-a")
	own := filepath.Join(s.AllocationRoot, s.JobID)
	other := filepath.Join(s.AllocationRoot, "kandev-other")
	for _, d := range []string{own, other} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "bootstrap.sh.env"), []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	command, err := AllocationDirRemoveCommand(s)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if out, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
			t.Fatalf("remove %d: %v %s", i, err, out)
		}
	}
	if _, err := os.Stat(own); !os.IsNotExist(err) {
		t.Fatalf("allocation dir remains: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("sibling allocation dir removed: %v", err)
	}
}

func TestPoolAllocationDirRemoveRefusesSymlinkAndLegacy(t *testing.T) {
	root := t.TempDir()
	s := poolFenceState(t, root, "env-a")
	target := filepath.Join(root, "keep")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.AllocationRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(s.AllocationRoot, s.JobID)); err != nil {
		t.Fatal(err)
	}
	command, err := AllocationDirRemoveCommand(s)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Command("sh", "-c", command).Run() == nil {
		t.Fatal("removal followed a symlinked allocation dir")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("symlink target removed: %v", err)
	}
	if c, err := AllocationDirRemoveCommand(destroyState()); err != nil || c != "" {
		t.Fatalf("legacy pinned worker must keep its allocation dir: %q %v", c, err)
	}
}

func TestPoolAllocationDirRemoveRejectsMismatchedJobID(t *testing.T) {
	s := poolFenceState(t, t.TempDir(), "env-a")
	s.JobID = "../escape"
	if _, err := AllocationDirRemoveCommand(s); err == nil {
		t.Fatal("expected refusal for an unsafe job id")
	}
}
