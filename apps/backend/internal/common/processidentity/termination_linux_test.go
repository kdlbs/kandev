//go:build linux

package processidentity

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func TestOwnedSessionTerminationRequiresNoLiveDescendants(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	identity, err := Capture(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if terminated, err := OwnedSessionTerminated(identity); err != nil || terminated {
		t.Fatalf("live owner terminated: %v, %v", terminated, err)
	}
	if state, err := Inspect(identity); err != nil || state != StateAlive {
		t.Fatalf("live owner: %s, %v", state, err)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if terminated, err := OwnedSessionTerminated(identity); err != nil || !terminated {
		t.Fatalf("terminated owner: %v, %v", terminated, err)
	}
}

func TestSurvivingChildPreventsOwnedSessionTerminationProof(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = read.Close() }()
	cmd := exec.Command("/bin/sh", "-c", "sleep 60 & echo $! >&3; wait")
	cmd.ExtraFiles = []*os.File{write}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = write.Close()
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); _ = cmd.Wait() })
	var child int
	if _, err := fmt.Fscanln(read, &child); err != nil {
		t.Fatal(err)
	}
	identity, err := Capture(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(child, 0); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	terminated, err := OwnedSessionTerminated(identity)
	if err != nil || terminated {
		t.Fatalf("surviving child allowed continuation: %v, %v", terminated, err)
	}
}
