package plugins

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	"github.com/kandev/kandev/internal/plugins/store"
)

type catalogReadBarrierStore struct {
	store.Store
	blockRead   atomic.Bool
	readStarted chan struct{}
	releaseRead chan struct{}
	committed   chan struct{}
}

func (s *catalogReadBarrierStore) GetConfig(id string) (map[string]any, error) {
	config, err := s.Store.GetConfig(id)
	if s.blockRead.Swap(false) {
		close(s.readStarted)
		<-s.releaseRead
	}
	return config, err
}

func (s *catalogReadBarrierStore) SetConfig(id string, config map[string]any) error {
	if err := s.Store.SetConfig(id, config); err != nil {
		return err
	}
	close(s.committed)
	return nil
}

type catalogSnapshotResult struct {
	snapshot plugintools.Snapshot
	err      error
}

type catalogSnapshotListener struct {
	service *Service
	result  chan catalogSnapshotResult
}

func (l *catalogSnapshotListener) NotifyAgentToolCatalogChanged() {
	snapshot, err := l.service.AgentToolCatalog()
	l.result <- catalogSnapshotResult{snapshot: snapshot, err: err}
}

func TestAgentToolCatalogOrdersReadinessAcrossConfigCommit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before string
		after  string
		tools  int
	}{
		{name: "configure", before: " ", after: "ready", tools: 1},
		{name: "clear", before: "ready", after: " ", tools: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				svc, fsStore, _ := newTestService(t)
				const id = "catalog-order"
				addAgentToolRecord(t, svc, fsStore, requiredConfigToolRecord(id, map[string]any{
					"type": "object", "required": []any{"token"},
					"properties": map[string]any{"token": map[string]any{"type": "string"}},
				}))
				if err := fsStore.SetConfig(id, map[string]any{"token": tc.before}); err != nil {
					t.Fatal(err)
				}
				if _, err := svc.AgentToolCatalog(); err != nil {
					t.Fatal(err)
				}
				barrier := &catalogReadBarrierStore{Store: fsStore, readStarted: make(chan struct{}),
					releaseRead: make(chan struct{}), committed: make(chan struct{})}
				barrier.blockRead.Store(true)
				svc.store = barrier
				listener := &catalogSnapshotListener{service: svc, result: make(chan catalogSnapshotResult, 1)}
				svc.SetAgentToolCatalogListener(listener)
				oldResult := make(chan catalogSnapshotResult, 1)
				go func() {
					snapshot, err := svc.AgentToolCatalog()
					oldResult <- catalogSnapshotResult{snapshot: snapshot, err: err}
				}()
				<-barrier.readStarted
				serialized := !svc.mu.TryLock()
				if !serialized {
					svc.mu.Unlock()
				}
				updated := make(chan error, 1)
				go func() { updated <- svc.UpdateConfig(context.Background(), id, map[string]any{"token": tc.after}) }()
				<-barrier.committed
				// Mutex contention is not a durable synctest wait. Only drain the
				// post-commit refresh when the old calculation left the mutex free.
				if !serialized {
					synctest.Wait()
				}
				close(barrier.releaseRead)
				old := <-oldResult
				updateErr := <-updated
				fresh := <-listener.result
				if old.err != nil || updateErr != nil || fresh.err != nil {
					t.Fatalf("catalog/update errors: %v, %v, %v", old.err, updateErr, fresh.err)
				}
				if old.snapshot.Generation != fresh.snapshot.Generation || old.snapshot.Revision >= fresh.snapshot.Revision {
					t.Fatalf("pre-commit snapshot revision %d must precede post-commit revision %d", old.snapshot.Revision, fresh.snapshot.Revision)
				}
				if len(fresh.snapshot.Tools) != tc.tools {
					t.Fatalf("post-commit tools = %d, want %d", len(fresh.snapshot.Tools), tc.tools)
				}
			})
		})
	}
}
