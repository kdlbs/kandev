//go:build linux

package sshreconcile

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/common/sshlaunch"
)

type ownedController struct {
	command  *exec.Cmd
	pid      int
	childPID int
}

type controllerChildren int

const (
	controllerNoChildren controllerChildren = iota
	controllerSurvivingChild
	controllerDrainedChild
)

func startOwnedController(t *testing.T, generation, taskDir string) *ownedController {
	t.Helper()
	return startOwnedControllerWithChildren(t, generation, taskDir, controllerNoChildren)
}

// startOwnedControllerWithChildren starts a real owned process in its own
// session, mirroring the dedicated login session an SSH controller receives.
// The process reports this task's workdir in argv so the helper's exact
// command-line check passes.
func startOwnedControllerWithChildren(
	t *testing.T,
	generation, taskDir string,
	children controllerChildren,
) *ownedController {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is unavailable")
	}
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	readyFile := filepath.Join(t.TempDir(), "ready")
	code := ownedControllerCode(children, pidFile, readyFile, python)
	command := exec.Command(python, "-c", code, "--workdir", taskDir)
	// The helper looks for an agentctl executable in argv. Python is the real
	// executable; argv carries the controller identity instead.
	command.Args = []string{"agentctl", "-c", code, "--workdir", taskDir}
	command.Env = append(os.Environ(), sshlaunch.GenerationEnv+"="+generation)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		t.Fatalf("start owned controller: %v", err)
	}
	controller := &ownedController{command: command, pid: command.Process.Pid}
	t.Cleanup(func() {
		if controller.childPID > 0 {
			_ = syscall.Kill(controller.childPID, syscall.SIGKILL)
		}
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
	})
	waitControllerReady(t, controller.pid, taskDir)
	waitControllerSignalReady(t, readyFile)
	if children != controllerNoChildren {
		controller.childPID = readControllerChildPID(t, pidFile)
	}
	return controller
}

func ownedControllerCode(children controllerChildren, pidFile, readyFile, python string) string {
	const childTerminates = `import signal,sys,time
signal.signal(signal.SIGTERM, lambda *a: sys.exit(0))
time.sleep(30)`
	const childSurvives = `import signal,time
signal.signal(signal.SIGTERM, signal.SIG_IGN)
time.sleep(30)`
	switch children {
	case controllerSurvivingChild:
		return fmt.Sprintf(`import signal,subprocess,sys,time
child = subprocess.Popen([%q, "-c", %q])
open(%q, "w").write(str(child.pid))
signal.signal(signal.SIGTERM, lambda *a: sys.exit(0))
open(%q, "w").close()
time.sleep(30)`, python, childSurvives, pidFile, readyFile)
	case controllerDrainedChild:
		return fmt.Sprintf(`import signal,subprocess,sys,time
child = subprocess.Popen([%q, "-c", %q])
open(%q, "w").write(str(child.pid))
def term(*_):
    child.terminate()
    child.wait()
    sys.exit(0)
signal.signal(signal.SIGTERM, term)
open(%q, "w").close()
time.sleep(30)`, python, childTerminates, pidFile, readyFile)
	default:
		return fmt.Sprintf(`import signal,sys,time
signal.signal(signal.SIGTERM, lambda *a: sys.exit(0))
open(%q, "w").close()
time.sleep(30)`, readyFile)
	}
}

func waitControllerSignalReady(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("owned controller did not install its TERM handler")
}

func readControllerChildPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if convErr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("owned controller child pid file %s was not written", path)
	return 0
}

// waitControllerReady blocks until the script interpreter has replaced the
// forked child image and the process reports the expected argv.
func waitControllerReady(t *testing.T, pid int, taskDir string) {
	t.Helper()
	cmdlinePath := filepath.Join("/proc", strconv.Itoa(pid), "cmdline")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(cmdlinePath)
		if err == nil && strings.Contains(string(raw), taskDir) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("owned controller %d did not become ready", pid)
}

func (c *ownedController) alive() bool {
	return c.command.Process.Signal(syscall.Signal(0)) == nil
}

func ownedReceipt(t *testing.T, controller *ownedController, generation, taskDir, sessionDir string) sshlaunch.Receipt {
	t.Helper()
	identity, err := processidentity.Capture(controller.pid)
	if err != nil {
		t.Fatalf("capture owned controller identity: %v", err)
	}
	return sshlaunch.Receipt{
		Version:    sshlaunch.Version,
		Generation: generation,
		Owner: sshlaunch.Owner{
			TaskID:               "task-1",
			SessionID:            "session-1",
			ControllerInstanceID: "instance-1",
			TaskDir:              taskDir,
			SessionDir:           sessionDir,
			Target: sshlaunch.TargetBinding{
				Host:            "runner.example.test",
				Port:            "22",
				User:            "kandev",
				HostFingerprint: "SHA256:test",
			},
		},
		UID:     os.Geteuid(),
		Process: identity,
	}
}

func newGeneration(t *testing.T) string {
	t.Helper()
	generation, err := sshlaunch.NewGeneration()
	if err != nil {
		t.Fatalf("NewGeneration: %v", err)
	}
	return generation
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.19
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.22
func TestReconcileStopsTheRecordedProcess(t *testing.T) {
	taskDir := t.TempDir()
	sessionDir := filepath.Join(taskDir, "sessions", "session-1")
	generation := newGeneration(t)
	controller := startOwnedController(t, generation, taskDir)
	receipt := ownedReceipt(t, controller, generation, taskDir, sessionDir)

	result := Reconcile(context.Background(), sshlaunch.ReconcileRequest{Version: sshlaunch.Version, Receipt: receipt})
	if result.Outcome != sshlaunch.OutcomeStopped {
		t.Fatalf("result = %#v, want stopped", result)
	}
	exited := make(chan struct{})
	go func() {
		_, _ = controller.command.Process.Wait()
		close(exited)
	}()
	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("controller process did not exit after reconciliation")
	}

	// A repeated reconciliation observes absence and sends nothing.
	again := Reconcile(context.Background(), sshlaunch.ReconcileRequest{Version: sshlaunch.Version, Receipt: receipt})
	if again.Outcome != sshlaunch.OutcomeAbsent {
		t.Fatalf("second result = %#v, want absent", again)
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.20
func TestReconcileRejectsIdentityChanges(t *testing.T) {
	taskDir := t.TempDir()
	generation := newGeneration(t)
	controller := startOwnedController(t, generation, taskDir)
	sessionDir := filepath.Join(taskDir, "sessions", "session-1")
	valid := ownedReceipt(t, controller, generation, taskDir, sessionDir)

	cases := map[string]func(*sshlaunch.Receipt){
		"birth token": func(receipt *sshlaunch.Receipt) {
			receipt.Process.BirthToken = bumpBirthToken(t, receipt.Process.BirthToken)
		},
		"generation": func(receipt *sshlaunch.Receipt) {
			receipt.Generation = newGeneration(t)
		},
		"workdir": func(receipt *sshlaunch.Receipt) {
			receipt.Owner.TaskDir = filepath.Join(taskDir, "other")
			receipt.Owner.SessionDir = filepath.Join(receipt.Owner.TaskDir, "sessions", "session-1")
		},
		"uid": func(receipt *sshlaunch.Receipt) {
			receipt.UID = os.Geteuid() + 1
		},
		"session directory present": func(receipt *sshlaunch.Receipt) {
			if err := os.MkdirAll(receipt.Owner.SessionDir, 0o755); err != nil {
				t.Fatalf("create replacement session directory: %v", err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(receipt.Owner.SessionDir) })
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			receipt := valid
			mutate(&receipt)
			result := Reconcile(context.Background(), sshlaunch.ReconcileRequest{Version: sshlaunch.Version, Receipt: receipt})
			if result.Outcome != sshlaunch.OutcomeUnresolved {
				t.Fatalf("result = %#v, want unresolved", result)
			}
			if result.Reason != sshlaunch.ReasonIdentityConflict {
				t.Fatalf("reason = %q, want identity_conflict", result.Reason)
			}
			if !controller.alive() {
				t.Fatal("identity conflict must not signal the recorded process")
			}
			if _, err := os.Stat(taskDir); err != nil {
				t.Fatalf("reconciliation removed the task directory: %v", err)
			}
			if name == "session directory present" {
				if _, err := os.Stat(sessionDir); err != nil {
					t.Fatalf("reconciliation removed the replacement session directory: %v", err)
				}
			}
		})
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.20
func TestReconcileRejectsReusedPID(t *testing.T) {
	taskDir := t.TempDir()
	generation := newGeneration(t)
	controller := startOwnedController(t, generation, taskDir)
	receipt := ownedReceipt(t, controller, generation, taskDir, filepath.Join(taskDir, "sessions", "session-1"))
	receipt.Process.BirthToken = bumpBirthToken(t, receipt.Process.BirthToken)

	result := Reconcile(context.Background(), sshlaunch.ReconcileRequest{Version: sshlaunch.Version, Receipt: receipt})
	if result.Outcome != sshlaunch.OutcomeUnresolved || result.Reason != sshlaunch.ReasonIdentityConflict {
		t.Fatalf("result = %#v, want unresolved identity_conflict", result)
	}
	if !controller.alive() {
		t.Fatal("a reused-PID verdict must not signal the current holder")
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.21
func TestReconcileRejectsIncompleteReceipt(t *testing.T) {
	result := Reconcile(context.Background(), sshlaunch.ReconcileRequest{Version: sshlaunch.Version})
	if result.Outcome != sshlaunch.OutcomeUnresolved || result.Reason != sshlaunch.ReasonProbeUnknown {
		t.Fatalf("result = %#v, want unresolved probe_unknown", result)
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.22
func TestReconcileAbsentProcessWithoutDirectoryCompletes(t *testing.T) {
	taskDir := t.TempDir()
	generation := newGeneration(t)
	controller := startOwnedController(t, generation, taskDir)
	receipt := ownedReceipt(t, controller, generation, taskDir, filepath.Join(taskDir, "sessions", "session-1"))
	_ = controller.command.Process.Kill()
	_, _ = controller.command.Process.Wait()

	result := Reconcile(context.Background(), sshlaunch.ReconcileRequest{Version: sshlaunch.Version, Receipt: receipt})
	if result.Outcome != sshlaunch.OutcomeAbsent {
		t.Fatalf("result = %#v, want absent", result)
	}
}

func TestReconcileCommandLineAndMarkerParsers(t *testing.T) {
	taskDir := "/home/kandev/.kandev/tasks/task-1"
	cmdline := []byte("agentctl\x00--workdir\x00" + taskDir + "\x00")
	if !commandLineMatches(cmdline, taskDir) {
		t.Fatal("exact agentctl workdir was rejected")
	}
	if commandLineMatches(cmdline, taskDir+"-sibling") {
		t.Fatal("a sibling workdir prefix was accepted")
	}
	if commandLineMatches([]byte("python\x00--workdir\x00"+taskDir+"\x00"), taskDir) {
		t.Fatal("a non-agentctl command line was accepted")
	}
	if !commandLineMatches([]byte("agentctl\x00--workdir="+taskDir+"\x00"), taskDir) {
		t.Fatal("the --workdir=value form was rejected")
	}
	generation := newGeneration(t)
	env := []byte("PATH=/usr/bin\x00" + sshlaunch.GenerationEnv + "=" + generation + "\x00")
	if !launchMarkerMatches(env, generation) {
		t.Fatal("the exact launch marker was rejected")
	}
	if launchMarkerMatches(env, newGeneration(t)) {
		t.Fatal("a different generation marker was accepted")
	}
}

func TestDirectoryAbsenceFailure(t *testing.T) {
	root := t.TempDir()
	taskDir := filepath.Join(root, "task")
	missing := filepath.Join(taskDir, "sessions", "session-1")
	if _, ok := directoryAbsenceFailure(missing, taskDir); !ok {
		t.Fatal("a positively absent session path was refused")
	}
	present := filepath.Join(taskDir, "present")
	if err := os.MkdirAll(present, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if failure, ok := directoryAbsenceFailure(present, taskDir); ok {
		t.Fatal("a present session path was accepted")
	} else if failure.Reason != sshlaunch.ReasonIdentityConflict {
		t.Fatalf("reason = %q, want identity_conflict", failure.Reason)
	}
	file := filepath.Join(taskDir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if failure, ok := directoryAbsenceFailure(file, taskDir); ok {
		t.Fatal("a present file was accepted as an absent directory")
	} else if failure.Reason != sshlaunch.ReasonIdentityConflict {
		t.Fatalf("reason = %q, want identity_conflict", failure.Reason)
	}
}

// bumpBirthToken advances the Linux start-tick field so the token cannot match
// the live process, without relying on PID reuse timing.
func bumpBirthToken(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ":")
	if len(parts) != 3 {
		t.Fatalf("unexpected birth token %q", token)
	}
	start, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		t.Fatalf("parse birth token start ticks: %v", err)
	}
	return parts[0] + ":" + parts[1] + ":" + strconv.FormatUint(start+1, 10)
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.20
func TestReconcileReplacementBeforeSignalPreservesTargets(t *testing.T) {
	taskDir := t.TempDir()
	generation := newGeneration(t)
	controller := startOwnedController(t, generation, taskDir)
	sessionDir := filepath.Join(taskDir, "sessions", "session-1")
	receipt := ownedReceipt(t, controller, generation, taskDir, sessionDir)

	original := beforeSignalRecheck
	beforeSignalRecheck = func() {
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Errorf("create replacement directory: %v", err)
		}
	}
	t.Cleanup(func() { beforeSignalRecheck = original })

	result := Reconcile(context.Background(), sshlaunch.ReconcileRequest{Version: sshlaunch.Version, Receipt: receipt})
	if result.Outcome != sshlaunch.OutcomeUnresolved || result.Reason != sshlaunch.ReasonIdentityConflict {
		t.Fatalf("result = %#v, want unresolved identity_conflict", result)
	}
	if !controller.alive() {
		t.Fatal("a directory that appeared before the signal must protect the process")
	}
	if _, err := os.Stat(sessionDir); err != nil {
		t.Fatalf("reconciliation removed the replacement directory: %v", err)
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.19
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.22
func TestReconcileRetainsSurvivingChild(t *testing.T) {
	taskDir := t.TempDir()
	generation := newGeneration(t)
	controller := startOwnedControllerWithChildren(t, generation, taskDir, controllerSurvivingChild)
	if controller.childPID <= 0 {
		t.Fatal("surviving-child controller has no child pid")
	}
	receipt := ownedReceipt(t, controller, generation, taskDir, filepath.Join(taskDir, "sessions", "session-1"))

	result := Reconcile(context.Background(), sshlaunch.ReconcileRequest{
		Version: sshlaunch.Version, Mode: sshlaunch.ReconcileModeTerminate, Receipt: receipt,
	})
	if result.Outcome != sshlaunch.OutcomeUnresolved || result.Reason != sshlaunch.ReasonTerminationUnconfirmed {
		t.Fatalf("result = %#v, want unresolved termination_unconfirmed", result)
	}
	if !controller.alive() {
		t.Fatal("the helper must not force-kill a controller that did not exit")
	}
	if !processAlive(controller.childPID) {
		t.Fatal("the test child exited before the assertion")
	}

	// A retry after the leader exits must not discard the surviving child.
	_ = syscall.Kill(controller.pid, syscall.SIGKILL)
	_, _ = controller.command.Process.Wait()
	waitProcessGone(t, controller.pid)
	again := Reconcile(context.Background(), sshlaunch.ReconcileRequest{
		Version: sshlaunch.Version, Mode: sshlaunch.ReconcileModeTerminate, Receipt: receipt,
	})
	if again.Outcome != sshlaunch.OutcomeUnresolved || again.Reason != sshlaunch.ReasonTerminationUnconfirmed {
		t.Fatalf("retry result = %#v, want unresolved while a child survives", again)
	}
	if !processAlive(controller.childPID) {
		t.Fatal("retry released a surviving child")
	}

	_ = syscall.Kill(controller.childPID, syscall.SIGKILL)
	waitProcessGone(t, controller.childPID)
	settled := Reconcile(context.Background(), sshlaunch.ReconcileRequest{
		Version: sshlaunch.Version, Mode: sshlaunch.ReconcileModeTerminate, Receipt: receipt,
	})
	if settled.Outcome != sshlaunch.OutcomeAbsent {
		t.Fatalf("settled result = %#v, want absent after child drainage", settled)
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.19
func TestReconcileStopsAfterChildDrainage(t *testing.T) {
	taskDir := t.TempDir()
	generation := newGeneration(t)
	controller := startOwnedControllerWithChildren(t, generation, taskDir, controllerDrainedChild)
	if controller.childPID <= 0 {
		t.Fatal("draining controller has no child pid")
	}
	receipt := ownedReceipt(t, controller, generation, taskDir, filepath.Join(taskDir, "sessions", "session-1"))

	result := Reconcile(context.Background(), sshlaunch.ReconcileRequest{
		Version: sshlaunch.Version, Mode: sshlaunch.ReconcileModeTerminate, Receipt: receipt,
	})
	if result.Outcome != sshlaunch.OutcomeStopped {
		t.Fatalf("result = %#v, want stopped after the child drained", result)
	}
	if processAlive(controller.childPID) {
		t.Fatal("a child drained by the controller must not survive the stop")
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.20
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestReconcileVerifyModeChecksIdentityWithoutSignalling(t *testing.T) {
	taskDir := t.TempDir()
	sessionDir := filepath.Join(taskDir, "sessions", "session-1")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("create session directory: %v", err)
	}
	generation := newGeneration(t)
	controller := startOwnedController(t, generation, taskDir)
	receipt := ownedReceipt(t, controller, generation, taskDir, sessionDir)

	verified := Reconcile(context.Background(), sshlaunch.ReconcileRequest{
		Version: sshlaunch.Version, Mode: sshlaunch.ReconcileModeVerify, Receipt: receipt,
	})
	if verified.Outcome != sshlaunch.OutcomeVerified {
		t.Fatalf("result = %#v, want verified", verified)
	}
	if !controller.alive() {
		t.Fatal("verify mode signalled the recorded process")
	}

	foreign := receipt
	foreign.Process.BirthToken = bumpBirthToken(t, foreign.Process.BirthToken)
	conflict := Reconcile(context.Background(), sshlaunch.ReconcileRequest{
		Version: sshlaunch.Version, Mode: sshlaunch.ReconcileModeVerify, Receipt: foreign,
	})
	if conflict.Outcome != sshlaunch.OutcomeUnresolved || conflict.Reason != sshlaunch.ReasonIdentityConflict {
		t.Fatalf("result = %#v, want unresolved identity_conflict", conflict)
	}
	if !controller.alive() {
		t.Fatal("a failed verify must not signal the recorded process")
	}
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func waitProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("process %d did not exit", pid)
}

func TestParseEffectiveUID(t *testing.T) {
	status := []byte("Name:\tpython3\nUid:\t1000\t1001\t1000\t1000\nGid:\t1000\t1000\t1000\t1000\n")
	uid, err := parseEffectiveUID(status)
	if err != nil || uid != 1001 {
		t.Fatalf("parseEffectiveUID = (%d, %v), want (1001, nil)", uid, err)
	}
	for _, invalid := range [][]byte{
		[]byte("Name:\tpython3\n"),
		[]byte("Uid:\t1000\n"),
		[]byte("Uid:\t1000\tnot-a-uid\t1000\t1000\n"),
	} {
		if _, err := parseEffectiveUID(invalid); err == nil {
			t.Fatalf("parseEffectiveUID(%q) accepted an invalid status", invalid)
		}
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.20
func TestReconcileRejectsSymlinkedSessionAncestors(t *testing.T) {
	realTask := t.TempDir()
	linkTask := filepath.Join(t.TempDir(), "task-link")
	if err := os.Symlink(realTask, linkTask); err != nil {
		t.Fatalf("symlink task dir: %v", err)
	}
	sessionDir := filepath.Join(linkTask, "sessions", "session-1")
	receipt := sshlaunch.Receipt{
		Version:    sshlaunch.Version,
		Generation: newGeneration(t),
		Owner: sshlaunch.Owner{
			TaskID: "task-1", SessionID: "session-1", ControllerInstanceID: "instance-1",
			TaskDir: linkTask, SessionDir: sessionDir,
			Target: sshlaunch.TargetBinding{
				Host: "runner.example.test", Port: "22", User: "kandev", HostFingerprint: "SHA256:test",
			},
		},
		UID: os.Geteuid(),
		Process: processidentity.Identity{
			PID: 4242, GroupID: 4242, SessionID: 4242, BirthToken: "linux:test-boot:1",
		},
	}
	result := Reconcile(context.Background(), sshlaunch.ReconcileRequest{
		Version: sshlaunch.Version, Mode: sshlaunch.ReconcileModeTerminate, Receipt: receipt,
	})
	if result.Outcome != sshlaunch.OutcomeUnresolved || result.Reason != sshlaunch.ReasonIdentityConflict {
		t.Fatalf("symlinked task dir result = %#v, want unresolved identity_conflict", result)
	}

	realSessions := filepath.Join(realTask, "sessions")
	if err := os.MkdirAll(realSessions, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	linkedSessions := filepath.Join(realTask, "linked-sessions")
	if err := os.Symlink(realSessions, linkedSessions); err != nil {
		t.Fatalf("symlink sessions: %v", err)
	}
	receipt.Owner.TaskDir = realTask
	receipt.Owner.SessionDir = filepath.Join(linkedSessions, "session-1")
	result = Reconcile(context.Background(), sshlaunch.ReconcileRequest{
		Version: sshlaunch.Version, Mode: sshlaunch.ReconcileModeTerminate, Receipt: receipt,
	})
	if result.Outcome != sshlaunch.OutcomeUnresolved || result.Reason != sshlaunch.ReasonIdentityConflict {
		t.Fatalf("symlinked sessions dir result = %#v, want unresolved identity_conflict", result)
	}
}
