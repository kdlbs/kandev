//go:build !windows

package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const backupSignalReportEnv = "KANDEV_BACKUP_SIGNAL_TEST_REPORT"
const backupSignalModeEnv = "KANDEV_BACKUP_SIGNAL_TEST_MODE"

type backupSignalResult struct {
	Code       int
	Supervisor bool
	Backend    bool
}

func init() {
	backupSignalHelper = runBackupSignalHelper
}

// @covers AC-PLATFORM-GO-DEV-LAUNCHER-001.16
func TestRunDevSnapshotInterruptCleanup(t *testing.T) {
	for _, mode := range []string{"interrupt_copy", "notification_released"} {
		t.Run(mode, func(t *testing.T) {
			clearLauncherConfigurationEnvironment(t)
			repo := makeRepoTree(t)
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("KANDEV_TASK_ID", "")
			source := filepath.Join(t.TempDir(), "production.db")
			createBackupFixture(t, source, false)
			before := readBackupFixtureFile(t, source)
			t.Setenv("KANDEV_DATABASE_PATH", source)
			previous := seedBackupHistory(t, home)
			report := filepath.Join(t.TempDir(), "outcome.json")
			waitErr := interruptBackupHelper(t, repo, mode, report)
			if mode == "notification_released" {
				assertSignalTerminatedHelper(t, waitErr)
				assertSuccessfulBackupHandoff(t, home)
			} else {
				assertInterruptedBackupResult(t, report, waitErr)
				assertBackupHistoryUnchanged(t, home, previous)
			}
			assertBackupSourceUnchanged(t, source, before, nil)
			assertBackupRecords(t, source)
		})
	}
}

func interruptBackupHelper(t *testing.T, repo, mode, report string) error {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready, notify, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ready.Close() })
	t.Cleanup(func() { _ = notify.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, executable, backupSignalHelperArg)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), backupSignalHelperEnv+"=1", backupSignalModeEnv+"="+mode, backupSignalReportEnv+"="+report)
	cmd.ExtraFiles = []*os.File{notify}
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	joined := false
	t.Cleanup(func() {
		if !joined {
			cancel()
			_ = cmd.Wait()
		}
	})
	_ = notify.Close()
	if err := ready.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var barrier [1]byte
	if _, err := io.ReadFull(ready, barrier[:]); err != nil {
		t.Fatalf("helper readiness: %v", err)
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	joined = true
	if ctx.Err() != nil {
		t.Fatalf("helper did not join within bound: %v\n%s", ctx.Err(), output.String())
	}
	if mode == "interrupt_copy" && strings.Contains(output.String(), "backed up production db") {
		t.Fatalf("interrupted snapshot announced success: %s", output.String())
	}
	t.Logf("helper mode=%s pid=%d actual wait=%v output=%s", mode, cmd.Process.Pid, err, output.String())
	return err
}

func assertInterruptedBackupResult(t *testing.T, report string, waitErr error) {
	t.Helper()
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("snapshot SIGINT bypassed cooperative cleanup: helper exit=%v, want reported dev exit 1", waitErr)
	}
	content, err := os.ReadFile(report)
	if err != nil {
		t.Fatalf("snapshot helper exited before cleanup report: %v", err)
	}
	var result backupSignalResult
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != 1 || result.Supervisor || result.Backend {
		t.Fatalf("interrupted admission launched child or supervisor: %+v", result)
	}
}

func assertSignalTerminatedHelper(t *testing.T, waitErr error) {
	t.Helper()
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		t.Fatalf("handoff helper exit=%v, want SIGINT", waitErr)
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGINT {
		t.Fatalf("snapshot signal owner remained registered at supervisor boundary: %v", waitErr)
	}
}

func assertSuccessfulBackupHandoff(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".kandev", "data", "backups")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != maxBackups {
		t.Fatalf("successful handoff left staging or invalid history: %v", entries)
	}
	newest := ""
	var latest time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("successful handoff left staging: %s", entry.Name())
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
			newest = filepath.Join(dir, entry.Name())
		}
	}
	assertBackupRecords(t, newest)
}

func runBackupSignalHelper() int {
	ready := os.NewFile(3, "snapshot-ready")
	if ready == nil {
		return 2
	}
	defer func() { _ = ready.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	result := backupSignalResult{}
	newSupervisorFn = func() *processSupervisor {
		result.Supervisor = true
		return newSupervisor()
	}
	launchBackendFn = func(_ backendLaunchConfig) (*restartableBackend, func(), error) {
		result.Backend = true
		return nil, nil, errors.New("test-only backend boundary")
	}
	if os.Getenv(backupSignalModeEnv) == "interrupt_copy" {
		copyProductionSnapshotFn = func(copyCtx context.Context, source, staged string) error {
			if err := copyProductionSnapshot(copyCtx, source, staged); err != nil {
				return err
			}
			if _, err := ready.Write([]byte{1}); err != nil {
				return err
			}
			select {
			case <-copyCtx.Done():
				return copyCtx.Err()
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		attachSignalsFn = func(_ *processSupervisor) {}
	} else {
		attachSignalsFn = func(_ *processSupervisor) {
			_, _ = ready.Write([]byte{1})
			<-ctx.Done()
		}
	}
	result.Code = Run([]string{"dev", "--headless"}, BuildInfo{})
	content, err := json.Marshal(result)
	if err == nil {
		err = os.WriteFile(os.Getenv(backupSignalReportEnv), content, 0o600)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return result.Code
}
