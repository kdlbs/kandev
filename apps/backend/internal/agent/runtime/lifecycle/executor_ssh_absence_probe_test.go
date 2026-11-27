package lifecycle

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.17
//
// TestRemoteProcessCommandLineCommandConfirmsAbsenceOnMacOSShapedHost runs
// the actual generated script (not a canned SSH fixture) through a real
// shell, with a fake `ps` on PATH shaped like macOS: `-p` is supported (the
// self-check succeeds) but the target pid is absent, and there is no /proc
// fallback to fall into. Before the fix, the script could not tell "this
// pid is gone" apart from "this ps doesn't understand -p", read the absent
// pid as an unsupported probe, and exited 2 with an error message — which
// verifyRemoteAgentctlIdentity then reported as an unproven identity rather
// than a confirmed-gone process, so a persisted stop of an already-exited
// remote agentctl retried forever on a host without /proc (macOS).
func TestRemoteProcessCommandLineCommandConfirmsAbsenceOnMacOSShapedHost(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	dir := t.TempDir()
	fakePS := filepath.Join(dir, "ps")
	// $2 is the pid argument, $4 is the -o value. The self-check
	// (`ps -p $$ -o pid=`) always succeeds; a lookup for any other pid (the
	// target, already exited) fails exactly like real `ps -p <dead-pid>`:
	// nonzero exit, no stdout, no stderr.
	script := "#!/bin/sh\ncase \"$4\" in\n  pid=) echo \"$2\"; exit 0 ;;\n  *) exit 1 ;;\nesac\n"
	if err := os.WriteFile(fakePS, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ps: %v", err)
	}

	cmd := exec.Command("sh", "-c", remoteProcessCommandLineCommand(99999))
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("script err = %v, want an *exec.ExitError", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("exit code = %d, want 1 (confirmed absence), stdout=%q stderr=%q",
			exitErr.ExitCode(), stdout.String(), stderr.String())
	}
	if stdout.String() != "" || stderr.String() != "" {
		t.Fatalf("stdout=%q stderr=%q, want both empty so remotePsProbeConfirmsAbsence reads this as absence",
			stdout.String(), stderr.String())
	}
}

// TestRemoteProcessCommandLineCommandReportsLiveProcessOnMacOSShapedHost
// covers the companion happy path with the same fake `-p`-supporting,
// /proc-less host: a pid the fake `ps` recognizes as the target still
// reports its command line rather than being swallowed by the new
// self-check branch.
func TestRemoteProcessCommandLineCommandReportsLiveProcessOnMacOSShapedHost(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	dir := t.TempDir()
	fakePS := filepath.Join(dir, "ps")
	// $2 is the pid argument, $4 the -o value. Both the target pid lookup
	// and the self-check succeed here — mirroring a live process.
	script := "#!/bin/sh\ncase \"$4\" in\n" +
		"  pid=) echo \"$2\"; exit 0 ;;\n" +
		"  command=) echo '/opt/kandev/bin/agentctl --workdir /remote/task'; exit 0 ;;\n" +
		"esac\n"
	if err := os.WriteFile(fakePS, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ps: %v", err)
	}

	cmd := exec.Command("sh", "-c", remoteProcessCommandLineCommand(4242))
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("script err = %v, stderr=%q", err, stderr.String())
	}
	if got := stdout.String(); got != "/opt/kandev/bin/agentctl --workdir /remote/task\n" {
		t.Fatalf("stdout = %q, want the agentctl command line", got)
	}
}
