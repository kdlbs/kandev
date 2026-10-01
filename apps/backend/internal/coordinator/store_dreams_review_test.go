package coordinator

import (
	"context"
	"testing"
	"time"
)

func TestPruneDreams_KeepsARowWhoseEpisodeIsNotArchived(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	old := dreamT0.Add(-500 * 24 * time.Hour)
	for _, id := range []string{"keep", "drop"} {
		d := newDream(id, "c1", old)
		d.WindowStart = old.Add(-time.Hour)
		if ok, err := s.InsertRunningDream(ctx, d); err != nil || !ok {
			t.Fatal(ok, err)
		}
		if id == "keep" {
			_, _ = s.SetDreamEpisodeTask(ctx, id, "task-keep")
		}
		_, _ = s.FailDream(ctx, id, "paused", old)
	}
	if n, err := s.PruneDreams(ctx, dreamT0, 100); err != nil || n != 1 {
		t.Fatalf("prune = %d %v, want 1", n, err)
	}
	if _, err := s.GetDream(ctx, "c1", "keep"); err != nil {
		t.Fatalf("a row with an unarchived episode was pruned: %v", err)
	}
	if err := s.MarkDreamEpisodeArchived(ctx, "keep", dreamT0); err != nil {
		t.Fatal(err)
	}
	if n, err := s.PruneDreams(ctx, dreamT0, 100); err != nil || n != 1 {
		t.Fatalf("prune after archive = %d %v, want 1", n, err)
	}
}

func TestDeleteCoordinator_RemovesDreamRows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	c := newPausableCoordinator(t, s)
	d := newDream("d1", c.ID, dreamT0)
	if ok, err := s.InsertRunningDream(ctx, d); err != nil || !ok {
		t.Fatal(ok, err)
	}
	d.Status = DreamClean
	items := []DreamItem{{ID: "i1", Position: 0, Kind: "note_add", Gate: "pass", CitedTurnIDs: []string{"a", "b"}}}
	if ok, err := s.FinishDream(ctx, d, items, dreamT0.Add(time.Minute)); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := s.RateDreamItem(ctx, c.ID, "d1", "i1", "u1", "useful"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCoordinator(ctx, "ws-1", c.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"coordinator_dreams", "coordinator_dream_items", "coordinator_dream_ratings"} {
		if n := countRows(t, s, table); n != 0 {
			t.Fatalf("%s kept %d rows after the coordinator was deleted", table, n)
		}
	}
}
