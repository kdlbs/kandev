package improvekandev

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/stretchr/testify/require"
)

func TestBootstrapJobManagerScopesOwnerAndExpiresResults(t *testing.T) {
	manager := newBootstrapJobManager()
	t.Cleanup(manager.stop)
	identity := authn.Identity{UserID: "user-1", OrgID: "org-1"}
	finished := make(chan struct{})
	jobID, err := manager.start(identity, func(context.Context) (*BootstrapResponse, string) {
		close(finished)
		return &BootstrapResponse{WorkspaceID: "workspace-1"}, ""
	})
	require.NoError(t, err)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("bootstrap worker did not finish")
	}
	require.Eventually(t, func() bool {
		snapshot, ok := manager.get(jobID, ownerForIdentity(identity))
		return ok && !snapshot.pending && snapshot.response != nil
	}, 5*time.Second, time.Millisecond, "bootstrap result was not stored")
	_, ok := manager.get(jobID, ownerForIdentity(authn.Identity{UserID: "user-2", OrgID: "org-1"}))
	require.False(t, ok, "another user in the same organization must not read the result")
	_, ok = manager.get(jobID, ownerForIdentity(authn.Identity{UserID: "user-1", OrgID: "org-2"}))
	require.False(t, ok, "the result must remain bound to its initiating organization")

	manager.mu.Lock()
	manager.jobs[jobID].finishedAt = time.Now().Add(-bootstrapJobRetention)
	manager.mu.Unlock()
	_, ok = manager.get(jobID, ownerForIdentity(identity))
	require.False(t, ok, "expired results must be removed")
}

func TestBootstrapJobManagerRetainsSuccessfulResultAfterCancellation(t *testing.T) {
	manager := newBootstrapJobManager()
	t.Cleanup(manager.stop)
	identity := authn.Identity{UserID: "user-1", OrgID: "org-1"}
	started := make(chan struct{})
	jobID, err := manager.start(identity, func(ctx context.Context) (*BootstrapResponse, string) {
		close(started)
		<-ctx.Done()
		return &BootstrapResponse{WorkspaceID: "workspace-1"}, ""
	})
	require.NoError(t, err)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("bootstrap worker did not start")
	}

	manager.stop()

	snapshot, ok := manager.get(jobID, ownerForIdentity(identity))
	require.True(t, ok)
	require.False(t, snapshot.pending)
	require.Empty(t, snapshot.publicError)
	require.NotNil(t, snapshot.response)
	require.Equal(t, "workspace-1", snapshot.response.WorkspaceID)
}

func TestBootstrapJobManagerBoundsActiveJobsAndEvictsFinishedJobs(t *testing.T) {
	manager := newBootstrapJobManager()
	t.Cleanup(manager.stop)
	finished := make(chan struct{})
	finishedID, err := manager.start(authn.Identity{UserID: "user-1"}, func(context.Context) (*BootstrapResponse, string) {
		close(finished)
		return &BootstrapResponse{WorkspaceID: "workspace-1"}, ""
	})
	require.NoError(t, err)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("finished job did not return")
	}
	require.Eventually(t, func() bool {
		snapshot, ok := manager.get(finishedID, ownerForIdentity(authn.Identity{UserID: "user-1"}))
		return ok && !snapshot.pending
	}, 5*time.Second, time.Millisecond, "finished job was not recorded")

	startBlocked := func() string {
		t.Helper()
		id, startErr := manager.start(authn.Identity{UserID: "user-1"}, func(ctx context.Context) (*BootstrapResponse, string) {
			<-ctx.Done()
			return nil, "canceled"
		})
		require.NoError(t, startErr)
		return id
	}
	for i := 0; i < bootstrapJobCapacity-1; i++ {
		startBlocked()
	}
	startBlocked() // Evicts the completed result to keep the map bounded.
	manager.mu.Lock()
	jobCount := len(manager.jobs)
	manager.mu.Unlock()
	require.Equal(t, bootstrapJobCapacity, jobCount)
	_, err = manager.start(authn.Identity{UserID: "user-1"}, func(context.Context) (*BootstrapResponse, string) {
		return nil, ""
	})
	require.ErrorIs(t, err, errBootstrapJobsFull)
	manager.stop()
}
