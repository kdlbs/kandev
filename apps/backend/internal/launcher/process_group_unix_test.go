//go:build linux || darwin

package launcher

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const launcherSignalHelperEnv = "KANDEV_LAUNCHER_SIGNAL_HELPER"
const launcherProcessTreeHelperEnv = "KANDEV_LAUNCHER_PROCESS_TREE_HELPER"
const launcherProcessTreeDescendantEnv = "KANDEV_LAUNCHER_PROCESS_TREE_DESCENDANT"

func TestConfigureManagedProcessCreatesProcessGroup(t *testing.T) {
	cmd := exec.Command("kandev")
	configureManagedProcess(cmd)

	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatalf("configureManagedProcess should set Setpgid")
	}
}

func TestManagedProcessKillSendsGracefulSignalBeforeForceKill(t *testing.T) {
	tempDir := t.TempDir()
	readyFile := filepath.Join(tempDir, "ready")
	termFile := filepath.Join(tempDir, "term")
	cmd := exec.Command(os.Args[0], "-test.run=TestLauncherSignalHelper")
	cmd.Env = append(os.Environ(),
		launcherSignalHelperEnv+"=1",
		"KANDEV_LAUNCHER_SIGNAL_HELPER_READY_FILE="+readyFile,
		"KANDEV_LAUNCHER_SIGNAL_HELPER_FILE="+termFile,
	)
	configureManagedProcess(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fixture: %v", err)
	}

	proc := &managedProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		code := 0
		if err != nil {
			code = 1
			if exitErr, ok := err.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			}
		}
		proc.mu.Lock()
		proc.exitCode = code
		proc.exited = true
		proc.mu.Unlock()
		close(proc.done)
	}()
	t.Cleanup(func() {
		if exited, _ := proc.Exited(); !exited {
			_ = killManagedProcessGroup(cmd.Process.Pid)
			<-proc.done
		}
	})

	waitForFile(t, readyFile)
	proc.kill()

	raw, err := os.ReadFile(termFile)
	if err != nil {
		t.Fatalf("expected fixture to handle SIGTERM before force kill: %v", err)
	}
	if string(raw) != "term" {
		t.Fatalf("SIGTERM marker = %q, want term", string(raw))
	}
}

func TestManagedProcessKillSignalsOnlyRootBeforeForceKill(t *testing.T) {
	tempDir := t.TempDir()
	rootReadyFile := filepath.Join(tempDir, "root-ready")
	rootTermFile := filepath.Join(tempDir, "root-term")
	descendantReadyFile := filepath.Join(tempDir, "descendant-ready")
	descendantTermFile := filepath.Join(tempDir, "descendant-term")
	releaseFile := filepath.Join(tempDir, "release")
	cmd := exec.Command(os.Args[0], "-test.run=TestLauncherProcessTreeHelper")
	cmd.Env = append(os.Environ(),
		launcherProcessTreeHelperEnv+"=1",
		"KANDEV_LAUNCHER_PROCESS_TREE_ROOT_READY_FILE="+rootReadyFile,
		"KANDEV_LAUNCHER_PROCESS_TREE_ROOT_TERM_FILE="+rootTermFile,
		"KANDEV_LAUNCHER_PROCESS_TREE_DESCENDANT_READY_FILE="+descendantReadyFile,
		"KANDEV_LAUNCHER_PROCESS_TREE_DESCENDANT_TERM_FILE="+descendantTermFile,
		"KANDEV_LAUNCHER_PROCESS_TREE_RELEASE_FILE="+releaseFile,
	)
	configureManagedProcess(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start process-tree fixture: %v", err)
	}
	proc := &managedProcess{label: "process-tree-fixture", cmd: cmd, done: make(chan struct{})}
	go waitForManagedProcess(t, proc)
	t.Cleanup(func() {
		if exited, _ := proc.Exited(); !exited {
			_ = killManagedProcessGroup(cmd.Process.Pid)
			waitForManagedProcessDone(t, proc, 5*time.Second)
		}
	})

	waitForFile(t, rootReadyFile)
	killDone := make(chan managedProcessShutdownResult, 1)
	go func() {
		killDone <- proc.kill()
	}()
	waitForFile(t, rootTermFile)
	if fileAppears(t, descendantTermFile, 500*time.Millisecond) {
		t.Fatalf("descendant received the launcher's initial graceful signal")
	}
	if err := cmd.Process.Signal(syscall.SIGUSR1); err != nil {
		t.Fatalf("release root fixture: %v", err)
	}
	result := <-killDone
	if !result.graceful || result.forceKilled {
		t.Fatalf("kill result graceful=%v forceKilled=%v err=%v, want true/false/nil",
			result.graceful, result.forceKilled, result.err)
	}
}

func TestAttachSignalsSingleSignalGracefulShutdown(t *testing.T) {
	tempDir := t.TempDir()
	termFile := filepath.Join(tempDir, "term")
	proc := startManagedSignalHelper(t, termFile, "")

	exitCh, output := captureLauncherExit(t)
	supervisor := newSupervisor()
	supervisor.add(proc)
	supervisor.attachSignals()

	sendLauncherTestSignal(t, os.Interrupt)

	waitForLauncherExitCode(t, exitCh, 0)
	waitForManagedProcessDone(t, proc, 5*time.Second)
	if raw, err := os.ReadFile(termFile); err != nil || string(raw) != "term" {
		t.Fatalf("SIGTERM marker = %q, %v; want term, nil", string(raw), err)
	}
	if got := output.String(); !strings.Contains(got, "graceful shutdown complete") {
		t.Fatalf("shutdown output missing graceful completion: %q", got)
	}
}

func TestAttachSignalsSecondSignalForceKillsChildren(t *testing.T) {
	tempDir := t.TempDir()
	termFile := filepath.Join(tempDir, "term")
	proc := startManagedSignalHelper(t, termFile, "hold-after-term")

	exitCh, output := captureLauncherExit(t)
	supervisor := newSupervisor()
	supervisor.add(proc)
	supervisor.attachSignals()

	sendLauncherTestSignal(t, os.Interrupt)
	waitForFile(t, termFile)
	sendLauncherTestSignal(t, os.Interrupt)

	waitForLauncherExitCode(t, exitCh, 1)
	waitForManagedProcessDone(t, proc, 5*time.Second)
	waitForOutputContains(t, output, "forced shutdown after second signal")
	waitForOutputContains(t, output, "forced shutdown complete")
	waitForOutputContains(t, output, "graceful shutdown complete")
}

func TestLauncherSignalHelper(t *testing.T) {
	if os.Getenv(launcherSignalHelperEnv) != "1" {
		return
	}
	termFile := os.Getenv("KANDEV_LAUNCHER_SIGNAL_HELPER_FILE")
	if termFile == "" {
		os.Exit(2)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	readyFile := os.Getenv("KANDEV_LAUNCHER_SIGNAL_HELPER_READY_FILE")
	if readyFile == "" {
		os.Exit(4)
	}
	if err := os.WriteFile(readyFile, []byte("ready"), 0o600); err != nil {
		os.Exit(5)
	}
	<-signals
	if err := os.WriteFile(termFile, []byte("term"), 0o600); err != nil {
		os.Exit(3)
	}
	if os.Getenv("KANDEV_LAUNCHER_SIGNAL_HELPER_MODE") == "hold-after-term" {
		for {
			time.Sleep(time.Hour)
		}
	}
	os.Exit(0)
}

func TestLauncherProcessTreeHelper(t *testing.T) {
	if os.Getenv(launcherProcessTreeHelperEnv) != "1" {
		return
	}
	descendantReadyFile := os.Getenv("KANDEV_LAUNCHER_PROCESS_TREE_DESCENDANT_READY_FILE")
	descendantTermFile := os.Getenv("KANDEV_LAUNCHER_PROCESS_TREE_DESCENDANT_TERM_FILE")
	rootReadyFile := os.Getenv("KANDEV_LAUNCHER_PROCESS_TREE_ROOT_READY_FILE")
	rootTermFile := os.Getenv("KANDEV_LAUNCHER_PROCESS_TREE_ROOT_TERM_FILE")
	releaseFile := os.Getenv("KANDEV_LAUNCHER_PROCESS_TREE_RELEASE_FILE")
	descendant := exec.Command(os.Args[0], "-test.run=TestLauncherProcessTreeDescendantHelper")
	descendant.Env = append(os.Environ(),
		launcherProcessTreeDescendantEnv+"=1",
		"KANDEV_LAUNCHER_PROCESS_TREE_DESCENDANT_READY_FILE="+descendantReadyFile,
		"KANDEV_LAUNCHER_PROCESS_TREE_DESCENDANT_TERM_FILE="+descendantTermFile,
	)
	if err := descendant.Start(); err != nil {
		t.Fatalf("start descendant fixture: %v", err)
	}
	waitForFile(t, descendantReadyFile)
	if err := os.WriteFile(rootReadyFile, []byte("ready"), 0o600); err != nil {
		t.Fatalf("write root ready file: %v", err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGUSR1)
	defer signal.Stop(signals)
	if got := <-signals; got != syscall.SIGTERM {
		t.Fatalf("root received signal %v, want SIGTERM", got)
	}
	if err := os.WriteFile(rootTermFile, []byte("term"), 0o600); err != nil {
		t.Fatalf("write root term file: %v", err)
	}
	for {
		if _, err := os.Stat(releaseFile); err == nil {
			break
		}
		if got := <-signals; got == syscall.SIGUSR1 {
			if err := os.WriteFile(releaseFile, []byte("release"), 0o600); err != nil {
				t.Fatalf("write release file: %v", err)
			}
		}
	}
	_ = descendant.Process.Kill()
	_ = descendant.Wait()
}

func TestLauncherProcessTreeDescendantHelper(t *testing.T) {
	if os.Getenv(launcherProcessTreeDescendantEnv) != "1" {
		return
	}
	readyFile := os.Getenv("KANDEV_LAUNCHER_PROCESS_TREE_DESCENDANT_READY_FILE")
	termFile := os.Getenv("KANDEV_LAUNCHER_PROCESS_TREE_DESCENDANT_TERM_FILE")
	if err := os.WriteFile(readyFile, []byte("ready"), 0o600); err != nil {
		t.Fatalf("write descendant ready file: %v", err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	defer signal.Stop(signals)
	for got := range signals {
		if got == syscall.SIGTERM {
			if err := os.WriteFile(termFile, []byte("term"), 0o600); err != nil {
				t.Fatalf("write descendant term file: %v", err)
			}
		}
	}
}

func startManagedSignalHelper(t *testing.T, termFile string, mode string) *managedProcess {
	t.Helper()
	readyFile := filepath.Join(t.TempDir(), "ready")
	cmd := exec.Command(os.Args[0], "-test.run=TestLauncherSignalHelper")
	cmd.Env = append(os.Environ(),
		launcherSignalHelperEnv+"=1",
		"KANDEV_LAUNCHER_SIGNAL_HELPER_READY_FILE="+readyFile,
		"KANDEV_LAUNCHER_SIGNAL_HELPER_FILE="+termFile,
		"KANDEV_LAUNCHER_SIGNAL_HELPER_MODE="+mode,
	)
	configureManagedProcess(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fixture: %v", err)
	}
	proc := &managedProcess{label: "fixture", cmd: cmd, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		code := 0
		if err != nil {
			code = 1
			if exitErr, ok := err.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			}
		}
		proc.mu.Lock()
		proc.exitCode = code
		proc.exited = true
		proc.mu.Unlock()
		close(proc.done)
	}()
	t.Cleanup(func() {
		if exited, _ := proc.Exited(); !exited {
			_ = killManagedProcessGroup(cmd.Process.Pid)
			waitForManagedProcessDone(t, proc, 5*time.Second)
		}
	})
	waitForFile(t, readyFile)
	return proc
}

func captureLauncherExit(t *testing.T) (chan int, *safeOutput) {
	t.Helper()
	exitCh := make(chan int, 1)
	output := &safeOutput{}
	oldExit := launcherExit
	oldStatusOutput := launcherStatusOutput
	launcherExit = func(code int) {
		exitCh <- code
	}
	launcherStatusOutput = output
	t.Cleanup(func() {
		launcherExit = oldExit
		launcherStatusOutput = oldStatusOutput
	})
	return exitCh, output
}

func sendLauncherTestSignal(t *testing.T, sig os.Signal) {
	t.Helper()
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find current process: %v", err)
	}
	if err := proc.Signal(sig); err != nil {
		t.Fatalf("send signal %v: %v", sig, err)
	}
}

func waitForLauncherExitCode(t *testing.T, exitCh <-chan int, want int) {
	t.Helper()
	select {
	case got := <-exitCh:
		if got != want {
			t.Fatalf("launcher exit code = %d, want %d", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for launcher exit code %d", want)
	}
}

func waitForManagedProcessDone(t *testing.T, proc *managedProcess, timeout time.Duration) {
	t.Helper()
	select {
	case <-proc.done:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for managed process pid=%d", proc.cmd.Process.Pid)
	}
}

func waitForOutputContains(t *testing.T, output *safeOutput, want string) {
	t.Helper()
	for range 500 {
		if strings.Contains(output.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("shutdown output missing %q: %q", want, output.String())
}

type safeOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *safeOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}

func (o *safeOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	for range 100 {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func fileAppears(t *testing.T, path string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func waitForManagedProcess(t *testing.T, proc *managedProcess) {
	t.Helper()
	err := proc.cmd.Wait()
	code := 0
	if err != nil {
		code = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		}
	}
	proc.mu.Lock()
	proc.exitCode = code
	proc.exited = true
	proc.mu.Unlock()
	close(proc.done)
}
