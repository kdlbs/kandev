package sandboxruntime

import (
	"context"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
)

func TestAuditShutdownReportsUnpersistedWorkerResults(t *testing.T) {
	manager := NewManager(&policy.Service{}, nil, t.TempDir())
	manager.markAuditPersistenceFailed()
	require.ErrorIs(t, manager.Close(), policy.ErrConflict)
}

func TestAuditCleanupWaitsForSelectedWorkerPersistence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager := NewManager(&policy.Service{}, nil, t.TempDir())
		cancelled, release := make(chan struct{}), make(chan struct{})
		ctx, cancel := context.WithCancel(manager.ctx)
		job := &auditJob{orgID: "org", attemptID: "attempt", cancel: cancel, done: make(chan struct{})}
		manager.audit.jobs["job"] = job
		released := false
		manager.audit.wait.Add(1)
		defer func() {
			if !released {
				close(release)
			}
			require.NoError(t, manager.Close())
		}()
		go func() {
			defer manager.audit.wait.Done()
			defer close(job.done)
			<-ctx.Done()
			close(cancelled)
			<-release
		}()
		cleaned := make(chan error, 1)
		go func() {
			cleaned <- manager.cancelAuditJobs(t.Context(), map[auditAttemptKey]bool{{org: "org", attempt: "attempt"}: true})
		}()
		<-cancelled
		synctest.Wait()
		select {
		case <-cleaned:
			t.Fatal("cleanup returned before the worker persisted its final state")
		default:
		}
		close(release)
		released = true
		require.NoError(t, <-cleaned)
	})
}

func TestAuditShutdownCancelsAndJoinsOwnedWorkers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager := NewManager(&policy.Service{}, nil, t.TempDir())
		cancelled, release := make(chan struct{}), make(chan struct{})
		released := false
		manager.audit.wait.Add(1)
		defer func() {
			if !released {
				close(release)
			}
			manager.audit.wait.Wait()
		}()
		go func() { defer manager.audit.wait.Done(); <-manager.ctx.Done(); close(cancelled); <-release }()
		closed := make(chan error, 1)
		go func() { closed <- manager.Close() }()
		<-cancelled
		synctest.Wait()
		select {
		case <-closed:
			t.Fatal("shutdown returned before its cancelled worker exited")
		default:
		}
		close(release)
		released = true
		require.NoError(t, <-closed)
	})
}
