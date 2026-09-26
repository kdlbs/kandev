//go:build !windows

package proclive

import (
	"os"
	"os/exec"
	"testing"
)

func TestAliveReportsCurrentProcess(t *testing.T) {
	alive, known := Alive(int64(os.Getpid()))
	if !alive || !known {
		t.Fatalf("Alive(self) = (%t, %t), want (true, true)", alive, known)
	}
}

func TestAliveReportsReapedProcessAsDead(t *testing.T) {
	child := exec.Command("sh", "-c", "exit 0")
	if err := child.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	t.Cleanup(func() {
		if child.ProcessState == nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	})
	if err := child.Wait(); err != nil {
		t.Fatalf("wait child: %v", err)
	}

	alive, known := Alive(int64(child.Process.Pid))
	if alive || !known {
		t.Fatalf("Alive(reaped child) = (%t, %t), want (false, true)", alive, known)
	}
}

func TestAliveReportsNonPositivePIDAsDeadAndKnown(t *testing.T) {
	for _, pid := range []int64{0, -1} {
		alive, known := Alive(pid)
		if alive || !known {
			t.Errorf("Alive(%d) = (%t, %t), want (false, true)", pid, alive, known)
		}
	}
}
