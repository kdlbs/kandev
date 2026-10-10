//go:build linux

package utility

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRuntimeObservationCommandTimeoutCleansDescendants(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "slow codex")
	pidFile := filepath.Join(dir, "child.pid")
	writeExecutable(t, binary, "#!/bin/sh\nsleep 30 &\necho $! > \""+pidFile+"\"\nwait\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := runRuntimeObservationCommand(ctx, binary, []string{"--version"}, os.Environ(), dir, zap.NewNop())
		result <- err
	}()
	pid := readPID(t, pidFile)
	deadline := time.AfterFunc(75*time.Millisecond, cancel)
	defer deadline.Stop()
	started := time.Now()
	var err error
	select {
	case err = <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out command did not finish after cancellation")
	}
	if err == nil {
		t.Fatal("cancelled command returned success")
	}
	if time.Since(started) > 3*time.Second {
		t.Fatalf("command cleanup exceeded the bound: %v", time.Since(started))
	}
	waitUntil(t, time.Second, func() bool { return !processRunning(pid) }, "observation child %d survived timeout", pid)
}
