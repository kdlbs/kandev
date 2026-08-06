//go:build !windows

package lsp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestManagerStopReapsLanguageServerProcessGroup(t *testing.T) {
	workDir := t.TempDir()
	pidFile := filepath.Join(workDir, "lsp-pids")
	t.Setenv("KANDEV_TEST_LSP_PID_FILE", pidFile)
	serverPath := filepath.Join(workDir, "kotlin-lsp")
	script := "#!/bin/sh\n" +
		"sleep 300 &\n" +
		"child=$!\n" +
		"printf '%s %s' \"$$\" \"$child\" > \"$KANDEV_TEST_LSP_PID_FILE\"\n" +
		"wait\n"
	if err := os.WriteFile(serverPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake language server: %v", err)
	}
	log := testLogger()
	processManager := process.NewManager(&config.InstanceConfig{WorkDir: workDir}, log)
	manager := NewManager(Config{
		WorkDir: workDir, WorkspaceURI: "file://" + workDir, OwnerID: "task-process-tree",
	}, processManager, &fakeInstaller{binary: serverPath}, log)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = manager.Close(ctx)
		_ = processManager.StopForTeardown(ctx)
	})

	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatalf("start language server: %v", err)
	}
	parentPID, childPID := waitForProcessTreePIDs(t, pidFile)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := manager.Stop(ctx, StopRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatalf("stop language server: %v", err)
	}
	waitForProcessExit(t, parentPID)
	waitForProcessExit(t, childPID)
}

func waitForProcessTreePIDs(t *testing.T, pidFile string) (int, int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(pidFile)
		if err == nil {
			parts := strings.Fields(string(contents))
			if len(parts) == 2 {
				parent, parentErr := strconv.Atoi(parts[0])
				child, childErr := strconv.Atoi(parts[1])
				if parentErr == nil && childErr == nil {
					return parent, child
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("language-server process tree did not report PIDs in %s", pidFile)
	return 0, 0
}

func waitForProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("process %d remained after language-server stop: %s", pid, fmt.Sprint(syscall.Kill(pid, 0)))
}
