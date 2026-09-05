package lifecycle

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sshStdinEnvReadyMarker is printed by the child as the statement before the
// prelude reads stdin, so the test can order its write against the import
// rather than against a guessed startup duration.
const sshStdinEnvReadyMarker = "kandev-stdin-env-ready"

// sshStdinEnvReadyTimeout bounds the wait for that marker. It only has to
// cover shell startup on a loaded machine; the test fails fast rather than
// hanging when the shell never reaches the import.
const sshStdinEnvReadyTimeout = 30 * time.Second

// TestSSHStdinEnvImportReadsStdinToEOF feeds the environment only after the
// shell has reached the stdin import, which is the ordering sshd produces on a
// real target. bash 3.2 (the /bin/bash on macOS) sources /dev/stdin by reading
// just the bytes already buffered when it opens the FIFO, so the previous
// `. /dev/stdin` prelude imported nothing there.
//
// The readiness marker is what makes this a regression test rather than a race:
// a fixed sleep after Start proves only that time passed. If the write won that
// race the environment sat in the pipe buffer, where even the old prelude found
// it, and the test would have passed against the bug it exists to catch.
func TestSSHStdinEnvImportReadsStdinToEOF(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "command-substitution-ran")
	wantValue := "bar baz $(touch " + marker + ") `printf backtick` it's\nsecond"
	envScript, err := buildSSHEnvInitScript(map[string]string{"KANDEV_TEST_STDIN_ENV": wantValue})
	if err != nil {
		t.Fatalf("buildSSHEnvInitScript() error = %v", err)
	}
	for _, shell := range []string{"sh", "bash", "zsh"} {
		path, err := exec.LookPath(shell)
		if err != nil {
			continue
		}
		t.Run(shell, func(t *testing.T) {
			script := `printf '%s\n' ` + sshStdinEnvReadyMarker + " >&2\n" +
				sshScriptWithEnvironment(`printf '%s' "$KANDEV_TEST_STDIN_ENV"`)
			cmd := exec.Command(path, "-c", script)
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatalf("stdin pipe: %v", err)
			}
			stderrPipe, err := cmd.StderrPipe()
			if err != nil {
				t.Fatalf("stderr pipe: %v", err)
			}
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			if err := cmd.Start(); err != nil {
				t.Fatalf("start %s: %v", shell, err)
			}

			// Reads stderr to EOF so the child never blocks on it, and reports
			// the marker exactly once. Every read of stderrTail below happens
			// after drained closes.
			ready := make(chan error, 1)
			drained := make(chan struct{})
			var stderrTail strings.Builder
			go func() {
				defer close(drained)
				reader := bufio.NewReader(stderrPipe)
				signalled := false
				for {
					line, readErr := reader.ReadString('\n')
					if !signalled && strings.Contains(line, sshStdinEnvReadyMarker) {
						signalled = true
						ready <- nil
					} else {
						stderrTail.WriteString(line)
					}
					if readErr != nil {
						if !signalled {
							ready <- fmt.Errorf("%s exited before the readiness marker: %w", shell, readErr)
						}
						return
					}
				}
			}()

			abort := func(format string, args ...interface{}) {
				t.Helper()
				_ = cmd.Process.Kill()
				<-drained
				_ = cmd.Wait()
				t.Fatalf(format+"\nstderr:\n%s", append(args, stderrTail.String())...)
			}
			select {
			case err := <-ready:
				if err != nil {
					abort("%v", err)
				}
			case <-time.After(sshStdinEnvReadyTimeout):
				abort("%s did not reach the stdin import within %s", shell, sshStdinEnvReadyTimeout)
			}

			// A prelude that does not read to EOF finishes and exits while the
			// write is still in flight, so the pipe breaks. That is the defect
			// under test, not a harness failure: record it and let the value
			// assertion below report what the shell actually imported.
			_, writeErr := io.WriteString(stdin, envScript)
			closeErr := stdin.Close()
			<-drained
			waitErr := cmd.Wait()
			if got := stdout.String(); got != wantValue {
				t.Fatalf("%s imported %q, want %q\nwrite: %v\nclose: %v\nexit: %v\nstderr:\n%s",
					shell, got, wantValue, writeErr, closeErr, waitErr, stderrTail.String())
			}
			if waitErr != nil {
				t.Fatalf("%s exited with %v:\nstdout:\n%s\nstderr:\n%s", shell, waitErr, stdout.String(), stderrTail.String())
			}
			if _, statErr := os.Stat(marker); statErr == nil {
				t.Fatalf("%s evaluated command substitution in the environment value", shell)
			} else if !os.IsNotExist(statErr) {
				t.Fatalf("stat command-substitution marker: %v", statErr)
			}
		})
	}
}
