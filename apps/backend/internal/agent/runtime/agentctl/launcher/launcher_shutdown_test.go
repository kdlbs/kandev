//go:build !windows

package launcher

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const plannedStopHelperEnv = "KANDEV_TEST_PLANNED_STOP_HELPER"
const plannedStopMarkerEnv = "KANDEV_TEST_PLANNED_STOP_MARKER"

// TestLauncherPlannedStopHelperProcess is the child side of
// TestLauncherPlannedStopKeepsLivenessUntilChildExit. It records whether the
// liveness pipe closed before the planned signal arrived.
func TestLauncherPlannedStopHelperProcess(t *testing.T) {
	if os.Getenv(plannedStopHelperEnv) != "1" {
		return
	}
	marker := os.Getenv(plannedStopMarkerEnv)
	pipe := os.NewFile(3, "parent-liveness-pipe")
	if pipe == nil {
		_ = os.WriteFile(marker, []byte("missing-pipe"), 0600)
		os.Exit(1)
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	_ = os.WriteFile(marker+".ready", []byte("ready"), 0600)
	eof := make(chan string, 1)
	go func() {
		buf := make([]byte, 1)
		if _, err := pipe.Read(buf); err != nil {
			eof <- "eof"
			return
		}
		eof <- "data"
	}()
	select {
	case <-sigCh:
		_ = os.WriteFile(marker, []byte("signal"), 0600)
		os.Exit(0)
	case result := <-eof:
		_ = os.WriteFile(marker, []byte(result), 0600)
		os.Exit(1)
	case <-time.After(10 * time.Second):
		_ = os.WriteFile(marker, []byte("timeout"), 0600)
		os.Exit(1)
	}
}

// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-004.1
// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-004.4
func TestLauncherPlannedStopKeepsLivenessUntilChildExit(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")
	readEnd, writeEnd, err := os.Pipe()
	require.NoError(t, err)

	cmd := exec.Command(os.Args[0], "-test.run=^TestLauncherPlannedStopHelperProcess$")
	cmd.Env = append(os.Environ(),
		plannedStopHelperEnv+"=1",
		plannedStopMarkerEnv+"="+marker,
	)
	cmd.ExtraFiles = []*os.File{readEnd}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	require.NoError(t, readEnd.Close())

	launcher := &Launcher{
		cmd:        cmd,
		exited:     make(chan struct{}),
		logger:     newLauncherTestLogger(t),
		parentPipe: writeEnd,
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		_ = writeEnd.Close()
	})
	go func() {
		_ = cmd.Wait()
		close(launcher.exited)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker + ".ready"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper process did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, launcher.Stop(ctx))

	content, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "signal", string(content), "the child must observe the planned signal before liveness pipe closure")
}
