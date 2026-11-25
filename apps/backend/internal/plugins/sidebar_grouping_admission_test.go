package plugins

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/stretchr/testify/require"
)

type statusSavedBarrier struct {
	store.Store
	id       string
	saved    chan struct{}
	inactive chan struct{}
}

func (s *statusSavedBarrier) Save(record *store.Record) error {
	if err := s.Store.Save(record); err != nil {
		return err
	}
	if record.ID == s.id && record.Status == StatusActive {
		close(s.saved)
	}
	if record.ID == s.id && record.Status == StatusDisabled {
		close(s.inactive)
	}
	return nil
}

func TestSidebarStatusAdmissionDoesNotBlockOtherPluginMetadata(t *testing.T) {
	svc, _, fsStore, _ := newTestServiceWithDir(t)
	first := installTestPlugin(t, svc, "kandev-plugin-first")
	second := installTestPlugin(t, svc, "kandev-plugin-second")
	first.Status = StatusDisabled
	require.NoError(t, fsStore.Save(first))
	svc.registry.Add(first)
	barrier := &statusSavedBarrier{Store: fsStore, id: first.ID, saved: make(chan struct{}), inactive: make(chan struct{})}
	svc.store = barrier
	svc.executorProviderOpMu.Lock()
	activated := make(chan error, 1)
	go func() { activated <- svc.setStatus(first.ID, StatusActive) }()
	<-barrier.saved
	// Taking the service mutex after Save puts activation beyond its commit boundary.
	svc.mu.Lock()
	released := svc.recordPersistMu.TryLock()
	if released {
		svc.recordPersistMu.Unlock()
	}
	svc.mu.Unlock()
	if !released {
		svc.executorProviderOpMu.Unlock()
		<-activated
		t.Fatal("activation retained persistence while waiting for executor admission")
	}
	metadata := make(chan struct{})
	go func() { svc.recordRestartCount(second.ID); close(metadata) }()
	select {
	case <-metadata:
	case <-time.After(5 * time.Second):
		t.Error("other plugin metadata blocked behind executor admission")
	}
	disabled := make(chan error, 1)
	go func() { disabled <- svc.setStatus(first.ID, StatusDisabled) }()
	committedEarly := false
	select {
	case <-barrier.inactive:
		committedEarly = true
	case <-time.After(100 * time.Millisecond):
		// Observation window: the same plugin cannot overtake pending admission.
	}
	svc.executorProviderOpMu.Unlock()
	require.NoError(t, <-activated)
	require.NoError(t, <-disabled)
	require.False(t, committedEarly, "same-plugin status overtook pending activation admission")
	<-metadata
	require.Len(t, svc.List(), 2)
}
