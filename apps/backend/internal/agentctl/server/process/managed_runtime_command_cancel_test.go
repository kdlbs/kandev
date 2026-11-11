//go:build !windows

package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	tools "github.com/kandev/kandev/internal/tools/installer"
	"github.com/stretchr/testify/require"
)

func TestManagedRuntimePreparationCommandCancelsWithInstance(t *testing.T) {
	manager := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = manager.StopForTeardown(stopCtx)
	})
	marker := filepath.Join(t.TempDir(), "started")
	done := make(chan error, 1)
	go func() {
		_, err := manager.runManagedRuntimeCommand(ctx, tools.CommandSpec{
			Path: "/bin/sh", Args: []string{"-c", `printf ready > "$READY_FILE"; exec sleep 30`},
			Env: map[string]string{"READY_FILE": marker},
		})
		done <- err
	}()
	require.Eventually(t, func() bool { _, err := os.Stat(marker); return err == nil }, 5*time.Second, 10*time.Millisecond)
	manager.CloseAdmission()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("preparation command after teardown = %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("instance teardown did not cancel its preparation command")
	}
}
