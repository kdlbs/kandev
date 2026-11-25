//go:build linux || darwin

package launcher

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"
)

// @covers AC-CLI-VERSION-JSON-001.5
func TestVersionJSONBrokenPipe(t *testing.T) {
	const helperEnv = "KANDEV_VERSION_SIGPIPE_TEST_MODE"
	if mode := os.Getenv(helperEnv); mode != "" {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGPIPE)
		signal.Reset(syscall.SIGPIPE)
		if mode == "control" {
			_, _ = os.Stdout.Write([]byte("control"))
			os.Exit(70)
		}
		os.Exit(Run([]string{"--version", "--json"}, BuildInfo{}))
	}
	for _, mode := range []string{"control", "version"} {
		t.Run(mode, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = writer.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestVersionJSONBrokenPipe$")
			cmd.Env = append(os.Environ(), helperEnv+"="+mode)
			cmd.Stdout = writer
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			err = cmd.Run()
			if ctx.Err() != nil {
				t.Fatalf("child timeout: %v", ctx.Err())
			}
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Fatalf("child error = %v, stderr = %q", err, stderr.String())
			}
			if mode == "control" {
				status, ok := exit.Sys().(syscall.WaitStatus)
				if !ok || !status.Signaled() || status.Signal() != syscall.SIGPIPE {
					t.Fatalf("control status = %v", exit)
				}
			} else if exit.ExitCode() != 1 || !strings.Contains(stderr.String(), "failed to write version output") || !strings.Contains(stderr.String(), "broken pipe") {
				t.Fatalf("version failure = %v, stderr = %q", exit, stderr.String())
			}
		})
	}
}
