//go:build linux

package processidentity

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCaptureRejectsReusedProcessIdentity(t *testing.T) {
	identity, err := Capture(syscall.Getpid())
	if err != nil {
		t.Fatalf("Capture(current pid): %v", err)
	}
	separator := strings.LastIndexByte(identity.BirthToken, ':')
	identity.BirthToken = identity.BirthToken[:separator+1] + "1"

	state, err := Inspect(identity)
	if err != nil {
		t.Fatalf("Inspect(mismatched identity): %v", err)
	}
	if state != StateReused {
		t.Fatalf("Inspect() = %q, want %q", state, StateReused)
	}
}

func TestRuntimeCleanupRejectsReusedPID(t *testing.T) {
	identity, err := Capture(syscall.Getpid())
	if err != nil {
		t.Fatalf("Capture(current pid): %v", err)
	}
	identity.GroupID = identity.PID
	identity.SessionID = identity.PID

	err = TerminateOwnedSession(context.Background(), identity, 0)
	if !errors.Is(err, ErrUnverifiableIdentity) {
		t.Fatalf("TerminateOwnedSession(reused pid) error = %v, want unverifiable identity", err)
	}
	if state, inspectErr := Inspect(identity); inspectErr != nil || state != StateReused {
		t.Fatalf("Inspect(reused pid) = %q, %v, want reused", state, inspectErr)
	}
}

func TestTerminateOwnedSessionKillsDescendantsOnly(t *testing.T) {
	unrelated := exec.Command("sleep", "90")
	if err := unrelated.Start(); err != nil {
		t.Fatalf("start unrelated process: %v", err)
	}
	t.Cleanup(func() {
		_ = unrelated.Process.Kill()
		_ = unrelated.Wait()
	})

	cmd := exec.Command("sh", "-c", "sleep 90 & echo $!; exit 0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start owned process: %v", err)
	}
	identity, err := Capture(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("Capture(owned process): %v", err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read descendant pid: %v", err)
	}
	descendantPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || descendantPID <= 0 {
		t.Fatalf("descendant pid = %q, err = %v", line, err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait for owned process: %v", err)
	}
	_ = stdout.Close()

	if err := TerminateOwnedSession(context.Background(), identity, 100*time.Millisecond); err != nil {
		t.Fatalf("TerminateOwnedSession: %v", err)
	}
	stat, err := readProcStat(descendantPID)
	if err == nil && stat.state != 'Z' && stat.state != 'X' {
		t.Fatalf("owned descendant %d is still running (state %c)", descendantPID, stat.state)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect owned descendant %d: %v", descendantPID, err)
	}
	if err := syscall.Kill(unrelated.Process.Pid, 0); err != nil {
		t.Fatalf("unrelated process %d was affected: %v", unrelated.Process.Pid, err)
	}
}

func TestTerminateOwnedSessionRejectsMissingIdentity(t *testing.T) {
	if err := TerminateOwnedSession(context.Background(), Identity{PID: syscall.Getpid()}, time.Millisecond); err == nil {
		t.Fatal("TerminateOwnedSession() error = nil, want missing-identity error")
	}
}
