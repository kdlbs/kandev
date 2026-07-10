//go:build windows

package utility

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const acpCommandTestFixtureEnv = "KANDEV_ACP_COMMAND_TEST_FIXTURE"

func TestMain(m *testing.M) {
	if spec := os.Getenv(acpCommandTestFixtureEnv); spec != "" {
		runACPCommandTestFixture(spec)
		return
	}
	os.Exit(m.Run())
}

func TestWindowsACPCommandLifecycleJobKillsDescendants(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), acpCommandTestFixtureEnv+"="+fmt.Sprintf("delay-then-child %s 200 30", pidFile))
	setACPCommandProcAttr(cmd)
	require.NoError(t, cmd.Start())
	parentPID := cmd.Process.Pid
	waited := false
	t.Cleanup(func() {
		if waited {
			return
		}
		_ = killACPProcessGroup(parentPID)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	lifecycle, err := installACPCommandLifecycle(cmd)
	require.NoError(t, err)
	childPID := waitForACPCommandChildPID(t, pidFile, 5*time.Second)

	releaseACPCommandLifecycle(lifecycle)
	_ = cmd.Wait()
	waited = true

	require.Eventually(t, func() bool {
		return !acpCommandWindowsProcessAlive(childPID)
	}, 5*time.Second, 50*time.Millisecond,
		"child process %d should be killed when the job handle is released", childPID)
}

func TestWindowsACPCommandLifecycleInstallFailsForInvalidPID(t *testing.T) {
	_, err := installACPCommandLifecycle(&exec.Cmd{Process: &os.Process{Pid: -1}})
	require.Error(t, err)
}

func runACPCommandTestFixture(spec string) {
	parts := strings.Fields(spec)
	if len(parts) == 0 {
		fmt.Fprintln(os.Stderr, "fixture: empty spec")
		os.Exit(2)
	}
	switch parts[0] {
	case "sleep":
		if len(parts) != 2 {
			fmt.Fprintln(os.Stderr, "fixture: sleep takes 1 arg")
			os.Exit(2)
		}
		secs, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "fixture: sleep: bad seconds %q\n", parts[1])
			os.Exit(2)
		}
		time.Sleep(time.Duration(secs) * time.Second)
	case "cat":
		_, _ = io.Copy(os.Stdout, os.Stdin)
	case "delay-then-child":
		if len(parts) != 4 {
			fmt.Fprintln(os.Stderr, "fixture: delay-then-child takes 3 args")
			os.Exit(2)
		}
		pidFile := parts[1]
		delayMS, err := strconv.Atoi(parts[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "fixture: delay-then-child: bad delay %q\n", parts[2])
			os.Exit(2)
		}
		secs, err := strconv.Atoi(parts[3])
		if err != nil {
			fmt.Fprintf(os.Stderr, "fixture: delay-then-child: bad seconds %q\n", parts[3])
			os.Exit(2)
		}
		time.Sleep(time.Duration(delayMS) * time.Millisecond)
		childCmd := exec.Command(os.Args[0])
		childCmd.Env = append(os.Environ(), acpCommandTestFixtureEnv+"=sleep "+strconv.Itoa(secs))
		if err := childCmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "fixture: delay-then-child: spawn child: %v\n", err)
			os.Exit(2)
		}
		if err := os.WriteFile(pidFile, []byte(strconv.Itoa(childCmd.Process.Pid)), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "fixture: delay-then-child: write pidfile: %v\n", err)
			os.Exit(2)
		}
		time.Sleep(time.Duration(secs) * time.Second)
	default:
		fmt.Fprintf(os.Stderr, "fixture: unknown command %q\n", parts[0])
		os.Exit(2)
	}
	os.Exit(0)
}

func waitForACPCommandChildPID(t *testing.T, pidFile string, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		raw, err := os.ReadFile(pidFile)
		if err == nil {
			pid, perr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if perr == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for fixture to write child pid to %s", pidFile)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func acpCommandWindowsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	output, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH").Output()
	if err != nil {
		return false
	}
	text := strings.ToLower(string(output))
	if strings.Contains(text, "no tasks") {
		return false
	}
	return strings.Contains(text, strconv.Itoa(pid))
}
