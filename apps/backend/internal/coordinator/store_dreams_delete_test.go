package coordinator

import (
	"context"
	"testing"
	"time"
)

func TestDeleteCoordinator_ArchivesItsOpenDreamEpisodeTasks(t *testing.T) {
	svc, store := newTestServiceForSubscribers(t)
	ctx := context.Background()
	c := &Coordinator{WorkspaceID: "ws-1", Name: "C", AgentProfileID: "a", ExecutorProfileID: "e",
		TaskAgentProfileID: "ta", TaskExecutorProfileID: "te"}
	if err := store.CreateCoordinator(ctx, c); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	open := Dream{ID: NewDreamID(), CoordinatorID: c.ID, WindowStart: now.Add(-2 * time.Hour), WindowEnd: now.Add(-time.Hour), StartedAt: now.Add(-time.Hour), Model: "m"}
	if held, err := store.InsertRunningDream(ctx, open); err != nil || !held {
		t.Fatalf("insert: held=%v err=%v", held, err)
	}
	if ok, err := store.SetDreamEpisodeTask(ctx, open.ID, "episode-open"); err != nil || !ok {
		t.Fatalf("bind: ok=%v err=%v", ok, err)
	}

	var archived []string
	svc.SetDreamEpisodeArchiver(func(_ context.Context, id string) error {
		archived = append(archived, id)
		return nil
	})
	if err := svc.DeleteCoordinator(ctx, c.WorkspaceID, c.ID); err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0] != "episode-open" {
		t.Fatalf("archived = %v, want the open episode task", archived)
	}
	if ids, err := store.OpenDreamEpisodeTasks(ctx, c.ID); err != nil || len(ids) != 0 {
		t.Fatalf("rows left behind: %v %v", ids, err)
	}
}
