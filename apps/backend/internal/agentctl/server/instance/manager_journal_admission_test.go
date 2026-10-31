package instance

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
func TestCreateInstanceRejectsUnavailableJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	owner, err := journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	defer func() { _ = owner.Close() }()

	mgr := newConcurrencyTestManager(t, 2)
	_, err = mgr.CreateInstance(context.Background(), &CreateRequest{
		ID:                 "blocked",
		SessionID:          "session-blocked",
		WorkspacePath:      t.TempDir(),
		DurableJournalPath: path,
	})
	require.Error(t, err)
	var openErr *journal.OpenError
	require.ErrorAs(t, err, &openErr)
	require.Equal(t, journal.OpenReasonLockTimeout, openErr.Reason())
	_, published := mgr.GetInstance("blocked")
	require.False(t, published, "a rejected journal must never publish an instance")

	healthy, err := mgr.CreateInstance(context.Background(), &CreateRequest{
		ID:            "healthy",
		WorkspacePath: t.TempDir(),
	})
	require.NoError(t, err, "another instance must remain usable after journal admission fails")
	require.NoError(t, mgr.StopInstance(context.Background(), healthy.ID))

	require.NoError(t, owner.Close())
	require.Eventually(t, func() bool {
		mgr.mu.RLock()
		defer mgr.mu.RUnlock()
		return len(mgr.provisional) == 0
	}, 5*time.Second, 10*time.Millisecond, "the rejected instance must release its provisional ownership")

	recovered, err := mgr.CreateInstance(context.Background(), &CreateRequest{
		ID:                 "blocked",
		SessionID:          "session-blocked",
		WorkspacePath:      t.TempDir(),
		DurableJournalPath: path,
	})
	require.NoError(t, err, "the exact rejected identity must be reusable after cleanup")
	require.NoError(t, mgr.StopInstance(context.Background(), recovered.ID))
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.3
func TestCreateInstanceTeardownReleasesJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	mgr := newConcurrencyTestManager(t, 2)

	first, err := mgr.CreateInstance(context.Background(), &CreateRequest{
		ID:                 "first",
		SessionID:          "session-shared",
		WorkspacePath:      t.TempDir(),
		DurableJournalPath: path,
	})
	require.NoError(t, err)
	require.NoError(t, mgr.StopInstance(context.Background(), first.ID))

	second, err := mgr.CreateInstance(context.Background(), &CreateRequest{
		ID:                 "second",
		SessionID:          "session-shared",
		WorkspacePath:      t.TempDir(),
		DurableJournalPath: path,
	})
	require.NoError(t, err, "exact instance teardown must close the journal before a replacement opens it")
	require.NoError(t, mgr.StopInstance(context.Background(), second.ID))
}
