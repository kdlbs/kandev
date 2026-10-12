//go:build unix

package launcher

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
)

func TestLauncherExitContainsOwnedDescendants(t *testing.T) {
	unrelated := exec.Command("sleep", "90")
	if err := unrelated.Start(); err != nil {
		t.Fatalf("start unrelated process: %v", err)
	}
	t.Cleanup(func() {
		_ = unrelated.Process.Kill()
		_ = unrelated.Wait()
	})

	cmd := exec.Command("sh", "-c", "sleep 90 & echo $!; exit 0")
	cmd.SysProcAttr = buildSysProcAttr(false)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start owned runtime: %v", err)
	}
	identity, err := processidentity.Capture(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("capture runtime identity: %v", err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read descendant pid: %v", err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || childPID <= 0 {
		t.Fatalf("descendant pid = %q, err = %v", line, err)
	}
	childIdentity, err := processidentity.Capture(childPID)
	if err != nil {
		t.Fatalf("capture descendant identity: %v", err)
	}
	_ = stdout.Close()

	exitReports := make(chan ExitReport, 1)
	launcher := &Launcher{
		cmd:             cmd,
		exited:          make(chan struct{}),
		logger:          newUnexpectedExitTestLogger(t),
		processIdentity: identity,
		onRuntimeExit:   func(report ExitReport) { exitReports <- report },
	}
	go launcher.monitorExit()
	select {
	case <-launcher.exited:
	case <-time.After(5 * time.Second):
		t.Fatal("launcher monitor did not finish")
	}

	select {
	case report := <-exitReports:
		if !report.Contained {
			t.Fatalf("unexpected exit was not contained: %v", report.ContainErr)
		}
		if report.Identity != identity {
			t.Fatalf("reported identity = %#v, want %#v", report.Identity, identity)
		}
	case <-time.After(time.Second):
		t.Fatal("unexpected exit report was not delivered")
	}

	state, err := processidentity.Inspect(childIdentity)
	if err != nil {
		t.Fatalf("inspect descendant after containment: %v", err)
	}
	if state != processidentity.StateExited {
		t.Fatalf("descendant state = %q, want %q", state, processidentity.StateExited)
	}
	if err := syscall.Kill(unrelated.Process.Pid, 0); err != nil {
		t.Fatalf("unrelated sentinel process was affected: %v", err)
	}
}
