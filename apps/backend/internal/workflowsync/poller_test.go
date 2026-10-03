package workflowsync

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
)

type shutdownBlockingGitHubClients struct {
	started chan struct{}
	release chan struct{}
}

type contextAwareGitHubClients struct {
	started  chan struct{}
	canceled chan struct{}
}

func (f *contextAwareGitHubClients) ListRepoDirectoryForWorkspace(ctx context.Context, _ string, _ string, _ string, _ string, _ string) ([]github.RepoContentEntry, error) {
	close(f.started)
	<-ctx.Done()
	close(f.canceled)
	return nil, ctx.Err()
}

func (f *contextAwareGitHubClients) GetRepoFileContentForWorkspace(context.Context, string, string, string, string, string) ([]byte, error) {
	return nil, nil
}

func (f *shutdownBlockingGitHubClients) ListRepoDirectoryForWorkspace(
	context.Context, string, string, string, string, string,
) ([]github.RepoContentEntry, error) {
	close(f.started)
	<-f.release
	return nil, nil
}

func (f *shutdownBlockingGitHubClients) GetRepoFileContentForWorkspace(
	context.Context, string, string, string, string, string,
) ([]byte, error) {
	return nil, nil
}

func TestPoller_StartStopIdempotent(t *testing.T) {
	svc, _ := setupTestService(t, seededMockClient())
	p := NewPoller(svc, svc.logger)

	p.Start(context.Background())
	t.Cleanup(p.Stop)
	p.Start(context.Background()) // second start is a no-op
	p.Stop()
	p.Stop() // second stop is a no-op
}

func TestPoller_SyncsDueConfigsOnTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, applier := setupTestService(t, seededMockClient())
		configureWorkspace(t, svc, "ws-1")

		p := NewPoller(svc, svc.logger)
		p.Start(context.Background())
		t.Cleanup(p.Stop)

		// No sync before the first tick: the loop waits a full interval so
		// boot doesn't hammer the GitHub API.
		synctest.Wait()
		assert.Zero(t, applier.callCount())

		time.Sleep(PollInterval + time.Second)
		synctest.Wait()
		p.Stop()

		require.Equal(t, 1, applier.callCount())
		cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
		require.NoError(t, err)
		assert.True(t, cfg.LastOk)
		assert.NotNil(t, cfg.LastSyncedAt)
	})
}

func TestPoller_StopWaitsForAutomaticSyncs(t *testing.T) {
	clients := &shutdownBlockingGitHubClients{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	store := setupTestStore(t)
	applier := &fakeApplier{}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	svc := NewService(store, clients, nil, applier, log)
	configureWorkspace(t, svc, "ws-1")

	p := NewPoller(svc, log)
	p.interval = time.Millisecond
	p.Start(context.Background())

	select {
	case <-clients.started:
	case <-time.After(time.Second):
		t.Fatal("automatic sync did not start")
	}

	stopped := make(chan struct{})
	go func() {
		p.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("poller stopped while an automatic sync was still running")
	case <-time.After(25 * time.Millisecond):
	}

	close(clients.release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("poller did not stop after the automatic sync completed")
	}
}

func TestPoller_ParentCancellationStopsAutomaticSyncs(t *testing.T) {
	clients := &contextAwareGitHubClients{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
	}
	store := setupTestStore(t)
	applier := &fakeApplier{}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	svc := NewService(store, clients, nil, applier, log)
	configureWorkspace(t, svc, "ws-1")

	ctx, cancel := context.WithCancel(context.Background())
	p := NewPoller(svc, log)
	p.interval = time.Millisecond
	p.Start(ctx)

	select {
	case <-clients.started:
	case <-time.After(time.Second):
		t.Fatal("automatic sync did not start")
	}
	cancel()

	select {
	case <-clients.canceled:
	case <-time.After(time.Second):
		t.Fatal("parent cancellation did not stop the automatic sync")
	}
}
