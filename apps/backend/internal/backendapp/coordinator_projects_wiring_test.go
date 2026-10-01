package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestRegisterCoordinatorSubscribers_PhaseTwoTidiesDeletedProjects(t *testing.T) {
	for _, phase2 := range []bool{false, true} {
		pool := newCoordinatorTestPool(t)
		log := newTestLogger()
		svc, err := initCoordinatorWiring(context.Background(), pool, newCoordinatorTestTracker(t), nil, nil, nil, true, phase2, false, false, log)
		if err != nil {
			t.Fatalf("initCoordinatorWiring: %v", err)
		}
		store, err := coordinator.NewStore(pool.Writer(), pool.Reader())
		if err != nil {
			t.Fatalf("NewStore: %v", err)
		}
		c := &coordinator.Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
		if err := store.CreateCoordinator(context.Background(), c); err != nil {
			t.Fatalf("CreateCoordinator: %v", err)
		}
		if _, err := pool.Writer().Exec(
			`INSERT INTO coordinator_watch_projects (coordinator_id, entry_kind, entry_id, workspace_id, created_at) VALUES (?, 'repository', 'repo-1', 'ws-1', CURRENT_TIMESTAMP)`, c.ID); err != nil {
			t.Fatalf("seed entry: %v", err)
		}

		memBus := bus.NewMemoryEventBus(log)
		registerCoordinatorSubscribers(nil, memBus, svc, log)
		evt := bus.NewEvent(events.RepositoryDeleted, "task-service", map[string]interface{}{"id": "repo-1"})
		if err := memBus.Publish(context.Background(), events.RepositoryDeleted, evt); err != nil {
			t.Fatalf("Publish: %v", err)
		}

		var n int
		if err := pool.Reader().Get(&n, `SELECT COUNT(*) FROM coordinator_watch_projects WHERE coordinator_id = ?`, c.ID); err != nil {
			t.Fatalf("count: %v", err)
		}
		want := 1
		if phase2 {
			want = 0
		}
		if n != want {
			t.Fatalf("phase2=%v: %d entries after repository.deleted, want %d", phase2, n, want)
		}
		memBus.Close()
	}
}

func TestPhase31Effective_FollowsPhase3AndItsOwnFlag(t *testing.T) {
	for _, on := range []bool{false, true} {
		for _, p2 := range []bool{false, true} {
			for _, p3 := range []bool{false, true} {
				for _, p31 := range []bool{false, true} {
					want := on && p2 && p3 && p31
					if got := phase31Effective(on, p2, p3, p31); got != want {
						t.Fatalf("phase31Effective(%v,%v,%v,%v) = %v, want %v", on, p2, p3, p31, got, want)
					}
				}
			}
		}
	}
}
