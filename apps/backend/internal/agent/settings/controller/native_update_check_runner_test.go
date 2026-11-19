package controller

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
)

const (
	nativeCheckHelperEnv      = "KANDEV_NATIVE_CHECK_TEST_HELPER"
	nativeCheckHeartbeatEnv   = "KANDEV_NATIVE_CHECK_TEST_HEARTBEAT"
	nativeCheckHelperExitCode = 3
)

// TestMain doubles as the native check helper process. The runner starts the
// test binary directly, so the helper sees exactly the runner's argv and env.
func TestMain(m *testing.M) {
	mode := os.Getenv(nativeCheckHelperEnv)
	if mode == "" {
		os.Exit(m.Run())
	}
	os.Exit(runNativeCheckHelper(mode))
}

func runNativeCheckHelper(mode string) int {
	switch mode {
	case "report":
		prefix, set := os.LookupEnv("npm_config_prefix")
		fmt.Printf("args=%s\n", strings.Join(os.Args[1:], " "))
		fmt.Printf("npm_config_prefix=%v:%s\n", set, prefix)
		fmt.Fprintln(os.Stderr, "stderr line")
		return nativeCheckHelperExitCode
	case "sleep":
		time.Sleep(time.Minute)
	case "flood":
		_, _ = os.Stdout.Write([]byte(strings.Repeat("x", nativeUpdateCheckOutputLimit/2)))
		_, _ = os.Stderr.Write([]byte(strings.Repeat("y", nativeUpdateCheckOutputLimit/2+1)))
		time.Sleep(time.Minute)
	case "spawn":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), nativeCheckHelperEnv+"=heartbeat")
		if err := child.Start(); err != nil {
			return 5
		}
		time.Sleep(time.Minute)
	case "orphan-pipe":
		// Reports success, then exits while a descendant still holds the
		// output pipes, so the runner can only finish through WaitDelay.
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), nativeCheckHelperEnv+"=sleep")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			return 5
		}
		fmt.Println("Current version: 18.13.3\n✔ Already up to date")
	case "heartbeat":
		path := os.Getenv(nativeCheckHeartbeatEnv)
		// Bounded so a regression cannot leave the helper running forever.
		for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); {
			if file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				_, _ = file.WriteString(".")
				_ = file.Close()
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	return 0
}

func nativeCheckHelperCommand(args ...string) agents.Command {
	return agents.NewCommand(append([]string{os.Args[0]}, args...)...)
}

func TestNativeCheckRunnerUsesUpdaterEnvironmentWithoutAddedArgs(t *testing.T) {
	t.Setenv(nativeCheckHelperEnv, "report")
	t.Setenv("npm_config_prefix", filepath.Join(t.TempDir(), "workspace-prefix"))
	run := execNativeUpdateCheckRunner{}.Run(context.Background(), nativeCheckHelperCommand("update", "--check"))
	if run.Reason != "" {
		t.Fatalf("reason = %q, want a completed run", run.Reason)
	}
	if run.Output.ExitCode != nativeCheckHelperExitCode {
		t.Errorf("exit code = %d, want %d", run.Output.ExitCode, nativeCheckHelperExitCode)
	}
	if !strings.Contains(run.Output.Stdout, "args=update --check\n") {
		t.Errorf("stdout = %q, want exactly the declared arguments", run.Output.Stdout)
	}
	if !strings.Contains(run.Output.Stdout, "npm_config_prefix=false:") {
		t.Errorf("stdout = %q, want npm_config_prefix filtered like the updater", run.Output.Stdout)
	}
	if run.Output.Stderr != "stderr line\n" && run.Output.Stderr != "stderr line\r\n" {
		t.Errorf("stderr = %q", run.Output.Stderr)
	}
	want, err := exec.LookPath(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if run.Executable != want {
		t.Errorf("executable = %q, want %q", run.Executable, want)
	}
}

func TestNativeCheckRunnerMissingExecutable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "omp-missing")
	run := execNativeUpdateCheckRunner{}.Run(context.Background(), agents.NewCommand(missing, "update", "--check"))
	if run.Reason != agents.NativeUpdateCheckReasonNotInstalled || run.Executable != "" {
		t.Errorf("run = %+v, want not_installed without a path", run)
	}
	if _, err := (execNativeUpdateCheckRunner{}).Resolve(agents.NewCommand(missing)); err == nil {
		t.Error("Resolve succeeded for a missing executable")
	}
}

// Launch resolution reads PATH directories and symlinks that can block on
// unreachable storage before a process exists. Cancellation must return at
// once without starting the process, even after the resolution completes.
func TestNativeCheckRunnerCancelDuringBlockedLaunchResolution(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "started")
	t.Setenv(nativeCheckHelperEnv, "heartbeat")
	t.Setenv(nativeCheckHeartbeatEnv, marker)
	entered, gate, resolved := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate) }) }
	t.Cleanup(release)
	runner := execNativeUpdateCheckRunner{resolveLaunch: func(command agents.Command) (string, string, error) {
		defer close(resolved)
		close(entered)
		<-gate
		return execNativeUpdateCheckRunner{}.resolveLaunchPaths(command)
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan nativeUpdateCheckRun, 1)
	go func() { done <- runner.Run(ctx, nativeCheckHelperCommand()) }()
	awaitNativeSignal(t, "blocked launch resolution", entered)
	cancel()
	select {
	case run := <-done:
		if run.Reason != agents.NativeUpdateCheckReasonCancelled {
			t.Errorf("reason = %q, want cancelled", run.Reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation waited for blocked launch resolution")
	}
	// Resolution recovers after cancellation; nothing may start from it. A
	// started helper writes the marker within milliseconds.
	release()
	awaitNativeSignal(t, "launch resolution after release", resolved)
	time.Sleep(500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the check started after its resolution recovered")
	}
}

func TestNativeCheckRunnerTimeoutCoversBlockedLaunchResolution(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "started")
	t.Setenv(nativeCheckHelperEnv, "heartbeat")
	t.Setenv(nativeCheckHeartbeatEnv, marker)
	entered, gate, resolved := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate) }) }
	t.Cleanup(release)
	runner := execNativeUpdateCheckRunner{
		timeout: 100 * time.Millisecond,
		resolveLaunch: func(command agents.Command) (string, string, error) {
			defer close(resolved)
			close(entered)
			<-gate
			return execNativeUpdateCheckRunner{}.resolveLaunchPaths(command)
		},
	}
	done := make(chan nativeUpdateCheckRun, 1)
	go func() { done <- runner.Run(context.Background(), nativeCheckHelperCommand()) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("launch resolution did not start")
	}
	select {
	case run := <-done:
		if run.Reason != agents.NativeUpdateCheckReasonTimeout {
			t.Fatalf("reason = %q, want timeout", run.Reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout did not include blocked launch resolution")
	}
	release()
	select {
	case <-resolved:
	case <-time.After(5 * time.Second):
		t.Fatal("blocked launch resolution did not return")
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the check started after launch resolution returned past its deadline")
	}
}

func TestNativeCheckRunnerTimeout(t *testing.T) {
	t.Setenv(nativeCheckHelperEnv, "sleep")
	started := time.Now()
	run := execNativeUpdateCheckRunner{timeout: 300 * time.Millisecond}.Run(context.Background(), nativeCheckHelperCommand())
	if run.Reason != agents.NativeUpdateCheckReasonTimeout {
		t.Fatalf("reason = %q, want timeout", run.Reason)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("timeout took %s", elapsed)
	}
	if run.Output != (agents.NativeUpdateCheckOutput{}) {
		t.Errorf("output = %+v, want none for a stopped run", run.Output)
	}
}

func TestNativeCheckRunnerOutputLimitStopsEarly(t *testing.T) {
	t.Setenv(nativeCheckHelperEnv, "flood")
	started := time.Now()
	run := execNativeUpdateCheckRunner{timeout: 30 * time.Second}.Run(context.Background(), nativeCheckHelperCommand())
	if run.Reason != agents.NativeUpdateCheckReasonOutputLimit {
		t.Fatalf("reason = %q, want output_limit", run.Reason)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("overflow stop took %s; the run waited for its timeout", elapsed)
	}
	if run.Output != (agents.NativeUpdateCheckOutput{}) {
		t.Error("partial output was returned after overflow")
	}
}

func TestNativeCheckRunnerRejectsOutputCutByWaitDelay(t *testing.T) {
	t.Setenv(nativeCheckHelperEnv, "orphan-pipe")
	run := execNativeUpdateCheckRunner{timeout: 30 * time.Second}.Run(context.Background(), nativeCheckHelperCommand())
	if run.Reason != agents.NativeUpdateCheckReasonCheckFailed {
		t.Fatalf("reason = %q, output = %+v; want check_failed for output closed by WaitDelay", run.Reason, run.Output)
	}
	if run.Output != (agents.NativeUpdateCheckOutput{}) {
		t.Errorf("partial output = %+v, want none", run.Output)
	}
}

func TestNativeCheckRunnerKillsDescendants(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason agents.NativeUpdateCheckReason
		run    func(command agents.Command, heartbeat string) nativeUpdateCheckRun
	}{
		{
			// Generous enough for the descendant to start under -race load.
			name:   "timeout",
			reason: agents.NativeUpdateCheckReasonTimeout,
			run: func(command agents.Command, _ string) nativeUpdateCheckRun {
				return execNativeUpdateCheckRunner{timeout: 5 * time.Second}.Run(context.Background(), command)
			},
		},
		{
			// Cancel only once the descendant is observably running.
			name:   "cancellation",
			reason: agents.NativeUpdateCheckReasonCancelled,
			run: func(command agents.Command, heartbeat string) nativeUpdateCheckRun {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				go func() {
					defer cancel()
					for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
						if _, err := os.Stat(heartbeat); err == nil {
							return
						}
					}
				}()
				return execNativeUpdateCheckRunner{}.Run(ctx, command)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			heartbeat := filepath.Join(t.TempDir(), "heartbeat")
			t.Setenv(nativeCheckHelperEnv, "spawn")
			t.Setenv(nativeCheckHeartbeatEnv, heartbeat)
			run := tc.run(nativeCheckHelperCommand(), heartbeat)
			if run.Reason != tc.reason {
				t.Fatalf("reason = %q, want %q", run.Reason, tc.reason)
			}
			requireHeartbeatStopped(t, heartbeat)
		})
	}
}

// requireHeartbeatStopped proves the descendant is gone: it appends to the
// heartbeat file every 20ms while alive.
func requireHeartbeatStopped(t *testing.T, path string) {
	t.Helper()
	time.Sleep(100 * time.Millisecond)
	first, err := os.Stat(path)
	if err != nil {
		t.Fatalf("descendant never started: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	second, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if second.Size() != first.Size() {
		t.Fatalf("descendant still alive: heartbeat grew from %d to %d bytes", first.Size(), second.Size())
	}
}
