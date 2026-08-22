package ownershiplock

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	if os.Getenv("KANDEV_OWNERSHIP_LOCK_HELPER") == "1" {
		os.Exit(m.Run())
	}
	goleak.VerifyTestMain(m)
}

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

func TestOwnershipLockRejectsSymlinkedLockPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "lock")
	want := []byte("must remain unchanged")
	if err := os.WriteFile(target, want, 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	file, err := openLockFile(link)
	if err == nil {
		_ = file.Close()
		t.Fatal("openLockFile succeeded for symlinked lock path")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("symlink target changed to %q, want %q", got, want)
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
		timer := time.NewTimer(5 * time.Minute)
		defer timer.Stop()
		<-timer.C
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
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(helper.stdout)
		if scanner.Scan() {
			lines <- scanner.Text()
			return
		}
		lines <- ""
	}()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case line := <-lines:
		<-done
		return line
	case <-timer.C:
		_ = helper.stdout.Close()
		<-done
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
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("second Acquire error = %T, want ConflictError", err)
	}
	if conflict.Target.ResourcePath != targets[0].ResourcePath {
		t.Fatalf("conflict target = %q, want canonical home path %q", conflict.Target.ResourcePath, targets[0].ResourcePath)
	}
	if conflict.Owner == nil {
		t.Fatal("conflict owner metadata is nil")
	}
	if conflict.Owner.PID != int64(os.Getpid()) {
		t.Fatalf("conflict owner PID = %d, want %d", conflict.Owner.PID, os.Getpid())
	}
	if !strings.Contains(conflict.Error(), fmt.Sprintf("pid %d", os.Getpid())) {
		t.Fatalf("conflict error = %q, want owner PID", conflict.Error())
	}
}

func TestOwnershipLockConflictFallsBackWithoutUsableOwner(t *testing.T) {
	oldLiveness := ownerLivenessFn
	ownerLivenessFn = func(int64) (bool, bool) { return false, true }
	t.Cleanup(func() { ownerLivenessFn = oldLiveness })

	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: ownerMetadataFixture("")},
		{name: "invalid", data: ownerMetadataFixture("not json")},
		{name: "dead", data: ownerMetadataBytes(mustOwnerJSON(t, OwnerRecord{PID: 987654321, Executable: "dead-kandev", StartedAt: "2026-08-19T09:14:35Z"}))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
			if err := writeOwnerMetadataFixture(owner.held[0].file, test.data); err != nil {
				t.Fatalf("write conflict metadata: %v", err)
			}

			_, err = Acquire(targets)
			var conflict *ConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("second Acquire error = %v, want ConflictError", err)
			}
			want := fmt.Sprintf("%s target %q is already owned by another backend", TargetHome, targets[0].ResourcePath)
			if conflict.Error() != want {
				t.Fatalf("conflict error = %q, want %q", conflict.Error(), want)
			}
			if conflict.Owner != nil {
				t.Fatalf("conflict owner = %+v, want nil", conflict.Owner)
			}
		})
	}
}

func writeOwnerMetadataFixture(file *os.File, data []byte) error {
	if err := file.Truncate(0); err != nil {
		return err
	}
	n, err := file.WriteAt(data, 0)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func TestOwnershipLockConflictKeepsUnknownOwner(t *testing.T) {
	oldLiveness := ownerLivenessFn
	ownerLivenessFn = func(int64) (bool, bool) { return false, false }
	t.Cleanup(func() { ownerLivenessFn = oldLiveness })

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

	_, err = Acquire(targets)
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("second Acquire error = %v, want ConflictError", err)
	}
	if conflict.Owner == nil {
		t.Fatal("unknown liveness dropped owner metadata")
	}
}

func TestOwnershipLockMetadataWriteFailureDoesNotBlockAcquire(t *testing.T) {
	oldWriteOwner := writeOwnerFn
	writeOwnerFn = func(*os.File) error { return errors.New("metadata unavailable") }
	t.Cleanup(func() { writeOwnerFn = oldWriteOwner })

	home := t.TempDir()
	targets, err := Targets(home, "sqlite", "")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	owner, err := Acquire(targets)
	if err != nil {
		t.Fatalf("Acquire with metadata failure: %v", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func mustOwnerJSON(t *testing.T, owner OwnerRecord) []byte {
	t.Helper()
	data, err := json.Marshal(owner)
	if err != nil {
		t.Fatalf("marshal owner: %v", err)
	}
	return data
}
