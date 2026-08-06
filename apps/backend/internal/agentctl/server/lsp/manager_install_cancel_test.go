package lsp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tools "github.com/kandev/kandev/internal/tools/installer"
)

type blockingInstallRegistry struct {
	strategy *blockingInstallStrategy
}

type blockingInstallStrategy struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingInstallRegistry) BinaryPath(string) (string, error) {
	return "", errors.New("language server is not installed")
}

func (r *blockingInstallRegistry) StrategyFor(string) (tools.Strategy, error) {
	return r.strategy, nil
}

func (s *blockingInstallStrategy) Install(ctx context.Context) (*tools.InstallResult, error) {
	s.once.Do(func() { close(s.started) })
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.release:
		return &tools.InstallResult{BinaryPath: "/trusted/kotlin-lsp"}, nil
	}
}

func (s *blockingInstallStrategy) Name() string { return "blocking-test" }

func TestManagerStopCancelsPendingInstallBeforeSlotLock(t *testing.T) {
	strategy := &blockingInstallStrategy{started: make(chan struct{}), release: make(chan struct{})}
	processes := newFakeProcessManager(func(int) *fakeLSPServer { return newFakeLSPServer() })
	manager := NewManager(
		Config{WorkDir: "/workspace", WorkspaceURI: "file:///workspace", OwnerID: "task-1"},
		processes,
		&blockingInstallRegistry{strategy: strategy},
		testLogger(),
	)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := manager.Close(ctx); err != nil {
			t.Errorf("close manager: %v", err)
		}
	})

	startDone := make(chan error, 1)
	go func() {
		_, err := manager.Start(context.Background(), StartRequest{
			Language: "go", Generation: 1, AutoInstall: true,
		})
		startDone <- err
	}()
	select {
	case <-strategy.started:
	case <-time.After(time.Second):
		t.Fatal("installer did not start")
	}

	stopDone := make(chan error, 1)
	go func() {
		_, err := manager.Stop(context.Background(), StopRequest{Language: "go", Generation: 1})
		stopDone <- err
	}()
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("stop pending install: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		close(strategy.release)
		<-startDone
		<-stopDone
		t.Fatal("Stop waited for the pending installer instead of canceling it")
	}
	if err := <-startDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("start error = %v, want canceled install", err)
	}
	if started, _, _ := processes.counts(); started != 0 {
		t.Fatalf("language-server processes started = %d, want 0", started)
	}
	if got := manager.Snapshot("go").Phase; got != "off" {
		t.Fatalf("phase = %q, want off", got)
	}
}
