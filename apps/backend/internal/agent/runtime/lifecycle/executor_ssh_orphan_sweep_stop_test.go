package lifecycle

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// waitForPIDFile polls path until it contains a parseable positive pid,
// or fails the test after a bounded wait.
func waitForPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(data))); convErr == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for pid file %s", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.16
//
// This test runs sshOrphanStopCommand's actual shell script (not a canned SSH
// fixture) against two real local processes: a "target" that ignores SIGTERM
// (forcing the ladder to escalate to SIGKILL) and spawns a "child" in its own
// process group via bash job control (`set -m`), mirroring the real agentctl
// child process-group layout (see procattr_unix.go's Setpgid). Killing only
// the target pid with SIGKILL would strand the child exactly as it did
// before this fix; the direct child's process group must also be signalled.
func TestSSHOrphanStopCommandKillsProcessAndDirectChildProcessGroup(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	dir := t.TempDir()
	childPIDFile := filepath.Join(dir, "child.pid")
	taskDirPath := filepath.Join(dir, "tasks", "task-1")

	targetScript := fmt.Sprintf(`set -m
trap '' TERM
sleep 100 &
echo $! > %s
wait
`, shellQuote(childPIDFile))

	// The real target is a plain bash process, not agentctl, so its own
	// command line is given a trailing "agentctl --workdir <taskDirPath>"
	// argv tail purely so the stop script's identity recheck (added for
	// R1-F2) sees a match — exercising the exact same script this test
	// already runs, rather than a shortcut around it.
	target := exec.Command("bash", "-c", targetScript, "agentctl", "--workdir", taskDirPath)
	if err := target.Start(); err != nil {
		t.Fatalf("start target process: %v", err)
	}
	targetPID := target.Process.Pid
	targetReaped := false
	t.Cleanup(func() {
		_ = target.Process.Kill()
		if !targetReaped {
			_ = target.Wait()
		}
	})

	childPID := waitForPIDFile(t, childPIDFile)
	t.Cleanup(func() {
		_ = syscall.Kill(childPID, syscall.SIGKILL)
	})

	if !processAlive(targetPID) {
		t.Fatalf("target pid %d is not alive before the stop", targetPID)
	}
	if !processAlive(childPID) {
		t.Fatalf("child pid %d is not alive before the stop", childPID)
	}

	script := sshOrphanStopCommand(targetPID, taskDirPath, "")
	output, err := exec.Command("sh", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("stop command failed: %v\n%s", err, output)
	}

	// The stop command's own signals are delivered asynchronously from this
	// process's perspective: this test is the target's real parent, so until
	// it reaps the exited child with Wait, the kernel keeps it as a zombie —
	// which kill(pid, 0) still reports as "alive". Reap synchronously here
	// (blocking until the kill above actually lands) before asserting liveness.
	_ = target.Wait()
	targetReaped = true

	if processAlive(targetPID) {
		t.Fatalf("target pid %d is still alive after the stop command", targetPID)
	}
	if processAlive(childPID) {
		t.Fatalf("direct child pid %d is still alive after SIGKILL escalation — its process group was not signalled", childPID)
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.16
//
// A process that already exited between inventory and stop must not cause
// the stop command to fail, and must not touch an unrelated live process
// that happens to share the dead pid's old ppid lineage.
func TestSSHOrphanStopCommandOnAlreadyExitedProcessSucceeds(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Run(); err != nil {
		t.Fatalf("spawn short-lived process: %v", err)
	}
	deadPID := cmd.Process.Pid

	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "session")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}

	script := sshOrphanStopCommand(deadPID, filepath.Join(dir, "task-1"), sessionDir)
	output, err := exec.Command("sh", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("stop command on an already-exited pid failed: %v\n%s", err, output)
	}
	if _, statErr := os.Stat(sessionDir); !os.IsNotExist(statErr) {
		t.Fatalf("session dir %s was not removed for a pidfile-attributed stop", sessionDir)
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.16
//
// Review Round 2 (R2-F2 part 2): between the inventory snapshot and this
// stop command, a fresh agentctl launch can claim the same session
// directory and rewrite its agentctl.pid to a different pid — the pidfile
// equivalent of the pid-reuse race R1-F2 already guards against for the
// kill itself. Removing the directory in that case would delete state out
// from under the new, live process. This proves the directory survives
// when its pidfile no longer names the pid this stop command was given.
func TestSSHOrphanStopCommandSessionDirSurvivesWhenPidfileNamesAnotherPID(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Run(); err != nil {
		t.Fatalf("spawn short-lived process: %v", err)
	}
	deadPID := cmd.Process.Pid

	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "session")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	otherPID := deadPID + 1
	pidFile := filepath.Join(sessionDir, "agentctl.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(otherPID)), 0o644); err != nil {
		t.Fatalf("write pidfile: %v", err)
	}

	script := sshOrphanStopCommand(deadPID, filepath.Join(dir, "task-1"), sessionDir)
	output, err := exec.Command("sh", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("stop command on an already-exited pid failed: %v\n%s", err, output)
	}
	if _, statErr := os.Stat(sessionDir); statErr != nil {
		t.Fatalf("session dir %s was removed even though its pidfile now names a different pid (%d): %v", sessionDir, otherPID, statErr)
	}
	if got, readErr := os.ReadFile(pidFile); readErr != nil || strings.TrimSpace(string(got)) != strconv.Itoa(otherPID) {
		t.Fatalf("pidfile content changed, got %q err %v, want %d untouched", got, readErr, otherPID)
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.16
//
// Review Round 1 (R1-F2): the stop script must re-check the target pid's own
// command line before signalling it, because the pid can have exited and
// been reused by an unrelated process in the window between the inventory
// snapshot and this stop call. This proves a real, live process whose
// command line does not name agentctl with the expected --workdir is left
// completely alone: not signalled, and the script still exits 0 (a no-op is
// success, not a failure to report).
func TestSSHOrphanStopCommandMismatchedIdentityLeavesProcessAlive(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	target := exec.Command("sleep", "100")
	if err := target.Start(); err != nil {
		t.Fatalf("start target process: %v", err)
	}
	targetPID := target.Process.Pid
	targetReaped := false
	t.Cleanup(func() {
		_ = target.Process.Kill()
		if !targetReaped {
			_ = target.Wait()
		}
	})

	if !processAlive(targetPID) {
		t.Fatalf("target pid %d is not alive before the stop", targetPID)
	}

	// The target's real command line is a plain "sleep 100": it never names
	// agentctl or this taskDirPath, simulating the pid having been reused by
	// an unrelated process since the inventory snapshot was taken.
	script := sshOrphanStopCommand(targetPID, "/remote/tasks/task-1", "")
	output, err := exec.Command("sh", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("stop command on a mismatched pid should succeed as a no-op: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "no longer matches") {
		t.Fatalf("stop command output = %q, want a message noting the identity mismatch", output)
	}
	if !processAlive(targetPID) {
		_ = target.Process.Kill()
		targetReaped = true
		_ = target.Wait()
		t.Fatalf("target pid %d was signalled despite a command-line identity mismatch", targetPID)
	}
}
