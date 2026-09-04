package lifecycle

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestSSHStdinEnvImportReadsStdinToEOF feeds the environment only after the
// shell has started, which is the ordering sshd produces on a real target.
// bash 3.2 (the /bin/bash on macOS) sources /dev/stdin by reading just the
// bytes already buffered when it opens the FIFO, so the previous
// `. /dev/stdin` prelude imported nothing there.
func TestSSHStdinEnvImportReadsStdinToEOF(t *testing.T) {
	envScript, err := buildSSHEnvInitScript(map[string]string{"KANDEV_TEST_STDIN_ENV": "bar baz"})
	if err != nil {
		t.Fatalf("buildSSHEnvInitScript() error = %v", err)
	}
	for _, shell := range []string{"sh", "bash", "zsh"} {
		path, err := exec.LookPath(shell)
		if err != nil {
			continue
		}
		t.Run(shell, func(t *testing.T) {
			cmd := exec.Command(path, "-c", sshScriptWithEnvironment(`printf '%s\n' "value=${KANDEV_TEST_STDIN_ENV:-unset}"`))
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatalf("stdin pipe: %v", err)
			}
			var output bytes.Buffer
			cmd.Stdout = &output
			cmd.Stderr = &output
			if err := cmd.Start(); err != nil {
				t.Fatalf("start %s: %v", shell, err)
			}
			time.Sleep(200 * time.Millisecond)
			if _, err := io.WriteString(stdin, envScript); err != nil {
				t.Fatalf("write env: %v", err)
			}
			if err := stdin.Close(); err != nil {
				t.Fatalf("close stdin: %v", err)
			}
			if err := cmd.Wait(); err != nil {
				t.Fatalf("%s exited with %v:\n%s", shell, err, output.String())
			}
			if got := strings.TrimSpace(output.String()); got != "value=bar baz" {
				t.Fatalf("%s imported %q, want %q", shell, got, "value=bar baz")
			}
		})
	}
}
