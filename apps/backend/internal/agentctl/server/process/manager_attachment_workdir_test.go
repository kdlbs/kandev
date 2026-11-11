package process

import (
	"context"
	"os/exec"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/stretchr/testify/require"
)

func TestAttachmentWorkDir_RebindSelectsReplacementAdapter(t *testing.T) {
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("cat is required for the idle ACP process fixture")
	}
	original, replacement := t.TempDir(), t.TempDir()
	manager := NewManager(&config.InstanceConfig{WorkDir: original, Protocol: "acp", AgentArgs: []string{cat}}, newTestLogger(t))
	t.Cleanup(func() { require.NoError(t, manager.Stop(context.Background())) })
	require.NoError(t, manager.Start(context.Background()))
	require.NoError(t, manager.WithAttachmentWorkDir(func(root string) error {
		require.Equal(t, original, root)
		return nil
	}))
	require.NoError(t, manager.Stop(context.Background()))
	require.Error(t, manager.WithAttachmentWorkDir(func(string) error { t.Fatal("stopped adapter selected"); return nil }))
	require.NoError(t, manager.RebindWorkspace(context.Background(), replacement))
	require.NoError(t, manager.Start(context.Background()))
	require.NoError(t, manager.WithAttachmentWorkDir(func(root string) error {
		require.Equal(t, replacement, root)
		return nil
	}))
}

func TestAttachmentWorkDir_InstallationDrainsBeforeTeardown(t *testing.T) {
	root := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		manager := NewManager(&config.InstanceConfig{WorkDir: root}, newTestLogger(t))
		t.Cleanup(func() { require.NoError(t, manager.Stop(context.Background())) })
		entered, release := make(chan struct{}), make(chan struct{})
		installed, drained := make(chan error, 1), make(chan error, 1)
		go func() {
			installed <- manager.WithAttachmentWorkDir(func(selected string) error {
				close(entered)
				<-release
				if selected != root {
					return context.Canceled
				}
				return nil
			})
		}()
		<-entered
		manager.CloseAdmission()
		go func() { drained <- manager.WaitForAdmission(context.Background()) }()
		synctest.Wait()
		select {
		case <-drained:
			t.Error("teardown did not wait for admitted installation")
		default:
		}
		close(release)
		require.NoError(t, <-installed)
		require.NoError(t, <-drained)
		require.ErrorIs(t, manager.WithAttachmentWorkDir(func(string) error {
			t.Fatal("teardown admitted a new installation")
			return nil
		}), ErrManagerStopping)
	})
}

func TestAttachmentWorkDir_ShellStartupUsesCompatibleLockOrder(t *testing.T) {
	manager := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
	manager.mu.Lock()
	done := make(chan error, 1)
	go func() { done <- manager.StartShell() }()
	defer func() {
		manager.mu.Unlock()
		require.NoError(t, <-done)
		require.NoError(t, manager.Stop(context.Background()))
	}()
	require.Eventually(t, func() bool {
		if manager.startMu.TryLock() {
			manager.startMu.Unlock()
			return false
		}
		return true
	}, time.Second, time.Millisecond, "shell startup must reserve startup before waiting for the process lock")
}
