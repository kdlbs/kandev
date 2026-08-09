package ownershiplock

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOwnershipLockRejectsSecondProcessAndReleasesAfterNormalExit(t *testing.T) {
	home := t.TempDir()
	targets, err := Targets(home, "sqlite", "")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	owner, err := Acquire(targets)
	if err != nil {
		t.Fatalf("Acquire primary owner: %v", err)
	}
	t.Cleanup(func() { _ = owner.Close() })

	conflict := startLockHelper(t, home, "conflict")
	if line := readHelperLine(t, conflict); line != "conflict" {
		t.Fatalf("helper result = %q, want conflict", line)
	}
	if err := conflict.cmd.Wait(); err != nil {
		t.Fatalf("conflict helper: %v; stderr=%s", err, conflict.stderr.String())
	}

	_ = owner.Close()
	released := startLockHelper(t, home, "normal")
	if line := readHelperLine(t, released); line != "ready" {
		t.Fatalf("normal helper result = %q, want ready", line)
	}
	if err := released.cmd.Wait(); err != nil {
		t.Fatalf("normal helper: %v; stderr=%s", err, released.stderr.String())
	}

	second, err := Acquire(targets)
	if err != nil {
		t.Fatalf("Acquire after normal exit: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("release after normal exit: %v", err)
	}
}

func TestOwnershipLockReleasesAfterForcedProcessExit(t *testing.T) {
	home := t.TempDir()
	helper := startLockHelper(t, home, "hold")
	if line := readHelperLine(t, helper); line != "ready" {
		t.Fatalf("holding helper result = %q, want ready", line)
	}

	if err := helper.cmd.Process.Kill(); err != nil {
		t.Fatalf("kill holding helper: %v", err)
	}
	if err := helper.cmd.Wait(); err == nil {
		t.Fatal("holding helper exited successfully after forced termination")
	}

	targets, err := Targets(home, "sqlite", "")
	if err != nil {
		t.Fatalf("Targets after forced exit: %v", err)
	}
	owner, err := Acquire(targets)
	if err != nil {
		t.Fatalf("Acquire after forced exit: %v", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("release after forced exit: %v", err)
	}
}

func TestOwnershipLockAllowsIndependentHomes(t *testing.T) {
	firstHome := t.TempDir()
	firstTargets, err := Targets(firstHome, "sqlite", "")
	if err != nil {
		t.Fatalf("first Targets: %v", err)
	}
	first, err := Acquire(firstTargets)
	if err != nil {
		t.Fatalf("Acquire first home: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })

	secondHome := t.TempDir()
	second := startLockHelper(t, secondHome, "normal")
	if line := readHelperLine(t, second); line != "ready" {
		t.Fatalf("independent helper result = %q, want ready", line)
	}
	if err := second.cmd.Wait(); err != nil {
		t.Fatalf("independent helper: %v; stderr=%s", err, second.stderr.String())
	}
}

func TestOwnershipLockHelper(t *testing.T) {
	if os.Getenv("KANDEV_OWNERSHIP_LOCK_HELPER") != "1" {
		return
	}
	home := os.Getenv("KANDEV_OWNERSHIP_HOME")
	targets, err := Targets(home, "sqlite", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Targets: %v\n", err)
		os.Exit(2)
	}
	owner, err := Acquire(targets)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			_, _ = fmt.Fprintln(os.Stdout, "conflict")
			return
		}
		fmt.Fprintf(os.Stderr, "Acquire: %v\n", err)
		os.Exit(2)
	}
	defer func() { _ = owner.Close() }()
	_, _ = fmt.Fprintln(os.Stdout, "ready")
	if os.Getenv("KANDEV_OWNERSHIP_MODE") == "hold" {
		select {}
	}
}

type lockHelper struct {
	cmd    *exec.Cmd
	stdout *os.File
	stderr *strings.Builder
}

func startLockHelper(t *testing.T, home, mode string) *lockHelper {
	t.Helper()
	stdout, stdin, err := os.Pipe()
	if err != nil {
		t.Fatalf("create helper stdout pipe: %v", err)
	}
	cmd := exec.Command(os.Args[0], "-test.run", "^TestOwnershipLockHelper$")
	cmd.Env = append(os.Environ(),
		"KANDEV_OWNERSHIP_LOCK_HELPER=1",
		"KANDEV_OWNERSHIP_HOME="+home,
		"KANDEV_OWNERSHIP_MODE="+mode,
	)
	cmd.Stdout = stdin
	stderr := new(strings.Builder)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stdin.Close()
		t.Fatalf("start lock helper: %v", err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		_ = stdout.Close()
		_ = stdin.Close()
	})
	return &lockHelper{cmd: cmd, stdout: stdout, stderr: stderr}
}

func readHelperLine(t *testing.T, helper *lockHelper) string {
	t.Helper()
	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(helper.stdout)
		if scanner.Scan() {
			lines <- scanner.Text()
			return
		}
		lines <- ""
	}()
	select {
	case line := <-lines:
		return line
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for lock helper")
		return ""
	}
}

func TestOwnershipLockConflictErrorIncludesTarget(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	targets, err := Targets(home, "sqlite", "")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	owner, err := Acquire(targets)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	t.Cleanup(func() { _ = owner.Close() })

	_, err = Acquire(targets)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("second Acquire error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), home) {
		t.Fatalf("conflict error = %q, want home path", err)
	}
}
