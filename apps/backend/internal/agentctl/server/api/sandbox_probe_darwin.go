//go:build darwin

package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// sandboxExecPath is absolute so the probe never resolves the binary through PATH.
const sandboxExecPath = "/usr/bin/sandbox-exec"

// seatbeltHostProbe proves that Seatbelt confinement works on this host.
func seatbeltHostProbe(ctx context.Context) error {
	return seatbeltProbe(ctx, sandboxExecPath, "")
}

// seatbeltProbe creates two files in a fresh directory, runs an unsandboxed
// control read of the second, then reads both under a deny-default profile that
// grants only the first. It passes only when the control read and the granted
// read succeed and the ungranted read fails with a permission error.
func seatbeltProbe(ctx context.Context, sandboxExec, base string) (err error) {
	dir, err := os.MkdirTemp(base, "kandev-seatbelt-probe-")
	if err != nil {
		return fmt.Errorf("seatbelt probe: create directory: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(dir); err == nil && removeErr != nil {
			err = fmt.Errorf("seatbelt probe: remove directory: %w", removeErr)
		}
	}()
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("seatbelt probe: resolve directory: %w", err)
	}
	granted, ungranted := filepath.Join(dir, "granted"), filepath.Join(dir, "ungranted")
	for _, name := range []string{granted, ungranted} {
		if err = os.WriteFile(name, []byte("probe"), 0600); err != nil {
			return fmt.Errorf("seatbelt probe: write fixture: %w", err)
		}
	}
	if _, err = runProbeCommand(ctx, "/bin/cat", ungranted); err != nil {
		return fmt.Errorf("seatbelt probe: control read: %w", err)
	}
	profile := seatbeltProbeProfile(granted)
	if stderr, runErr := runProbeCommand(ctx, sandboxExec, "-p", profile, "/bin/cat", granted); runErr != nil {
		return fmt.Errorf("seatbelt probe: granted read: %w: %s", runErr, strings.TrimSpace(stderr))
	}
	stderr, err := runProbeCommand(ctx, sandboxExec, "-p", profile, "/bin/cat", ungranted)
	if err == nil {
		return errors.New("seatbelt probe: ungranted read was not confined")
	}
	if !strings.Contains(stderr, "Operation not permitted") && !strings.Contains(stderr, "Permission denied") {
		return fmt.Errorf("seatbelt probe: ungranted read failed without a permission error: %w", err)
	}
	return nil
}

// seatbeltProbeProfile grants the reader only what it needs to start: the
// binary, the dyld shared cache and system libraries, the filesystem root
// entry, and the one granted file.
func seatbeltProbeProfile(granted string) string {
	return fmt.Sprintf(`(version 1)
(deny default)
(allow process-exec* (literal "/bin/cat"))
(allow file-read* (literal "/") (literal "/bin/cat") (subpath "/usr/lib") (subpath "/System") (subpath "/private/var/db/dyld") (literal %q))`, granted)
}

// runProbeCommand runs one bounded command and kills its process group on every
// outcome. Output is returned for permission-error classification only.
func runProbeCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"LC_ALL=C", "PATH=/usr/bin:/bin"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	// Inherited pipes need no filesystem grant, unlike the null device.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stderr.String(), err
}
