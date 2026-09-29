//go:build unix

package mcpconfig

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExecNativeMCPCommandRunnerBoundsInheritedPipeAfterCancellation(t *testing.T) {
	root := t.TempDir()
	started := filepath.Join(root, "started")
	pidPath := filepath.Join(root, "child.pid")
	command := filepath.Join(root, "native-fixture")
	script := "#!/bin/sh\n(sleep 30) &\nprintf '%s\\n' \"$!\" > \"" + pidPath + "\"\nprintf started > \"" + started + "\"\nwait\n"
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		value NativeMCPCommandResult
		err   error
	}
	finished := make(chan result, 1)
	go func() {
		value, err := (ExecNativeMCPCommandRunner{}).Run(ctx, command, []string{"mcp"}, root, map[string]string{"PATH": "/usr/bin:/bin"})
		finished <- result{value: value, err: err}
	}()
	waitForNativeMCPFile(t, started)
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("read child PID: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		t.Fatalf("parse child PID: %v", err)
	}
	cancel()
	select {
	case got := <-finished:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("runner error = %v, want context cancellation", got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runner remained blocked by descendant inheriting stdout")
	}
	waitForNativeMCPProcessGone(t, pid)
}

func isNativeMCPProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	if runtime.GOOS == "linux" {
		if raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
			if end := bytes.LastIndexByte(raw, ')'); end >= 0 && end+2 < len(raw) {
				return raw[end+2] != 'Z'
			}
		}
	}
	return true
}

func waitForNativeMCPProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for isNativeMCPProcessAlive(pid) {
		select {
		case <-deadline.C:
			t.Fatalf("descendant process %d survived command cancellation", pid)
		case <-ticker.C:
		}
	}
}

func waitForNativeMCPFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q", path)
}
