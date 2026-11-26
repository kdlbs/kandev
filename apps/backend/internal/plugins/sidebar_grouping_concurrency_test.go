package plugins

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/stretchr/testify/require"
)

type statusCommitBus struct {
	bus.EventBus
	inspect func()
}

func (b *statusCommitBus) Publish(_ context.Context, _ string, _ *bus.Event) error {
	b.inspect()
	return nil
}

func TestSidebarStatusCommitReleasesPersistenceBeforePublication(t *testing.T) {
	svc, _, fsStore, _ := newTestServiceWithDir(t)
	record := installTestPlugin(t, svc, "kandev-plugin-first")
	for _, status := range []Status{StatusDisabled, StatusActive} {
		notified := false
		svc.eventBus = &statusCommitBus{inspect: func() {
			notified = true
			unlocked := svc.recordPersistMu.TryLock()
			if unlocked {
				svc.recordPersistMu.Unlock()
			}
			require.True(t, unlocked, "subscriber must be able to acquire status persistence after taking service mutex")
			persisted, err := fsStore.Get(record.ID)
			require.NoError(t, err)
			require.Equal(t, status, persisted.Status)
		}}
		require.NoError(t, svc.setStatus(record.ID, status))
		require.True(t, notified)
	}
}
