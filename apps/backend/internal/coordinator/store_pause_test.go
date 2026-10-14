package coordinator

import (
	"context"
	"strings"
	"testing"
)

func newPausableCoordinator(t *testing.T, s *Store) *Coordinator {
	t.Helper()
	c := &Coordinator{WorkspaceID: "ws-1", Name: "c", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := s.CreateCoordinator(context.Background(), c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	return c
}

func TestPauseColumns_FreshAndReplayed(t *testing.T) {
	check := func(t *testing.T, s *Store) {
		t.Helper()
		for table, want := range map[string][]string{
			"coordinators":                 {"paused_at ", "paused_by "},
			"coordinator_unattended_turns": {"pause_requested_at ", "pause_cancel_at "},
		} {
			cols := strings.Join(tableColumns(t, s.db, table), "|")
			for _, w := range want {
				if !strings.Contains(cols, w) {
					t.Fatalf("%s is missing column %q in %s", table, w, cols)
				}
			}
		}
	}
	check(t, newTestStore(t))

	conn := openSQLitePool(t)
	upgradeFromPhase1(t, conn, false)
	upgraded, err := NewStore(conn, conn)
	if err != nil {
		t.Fatalf("NewStore upgrade: %v", err)
	}
	check(t, upgraded)
	replayed, err := NewStore(conn, conn)
	if err != nil {
		t.Fatalf("NewStore replay: %v", err)
	}
	check(t, replayed)
}

func TestSetPaused_PauseResumeIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	c := newPausableCoordinator(t, s)

	changed, err := s.SetPaused(ctx, c.ID, true, "user-1")
	if err != nil || !changed {
		t.Fatalf("first pause = (%v, %v), want (true, nil)", changed, err)
	}
	got, err := s.GetCoordinatorByID(ctx, c.ID)
	if err != nil || got.PausedAt == nil || got.PausedBy != "user-1" {
		t.Fatalf("after pause: %+v err=%v", got, err)
	}
	firstAt := *got.PausedAt

	changed, err = s.SetPaused(ctx, c.ID, true, "user-2")
	if err != nil || changed {
		t.Fatalf("second pause = (%v, %v), want (false, nil)", changed, err)
	}
	got, _ = s.GetCoordinatorByID(ctx, c.ID)
	if got.PausedBy != "user-1" || !got.PausedAt.Equal(firstAt) {
		t.Fatalf("second pause changed the state: %+v", got)
	}

	changed, err = s.SetPaused(ctx, c.ID, false, "user-2")
	if err != nil || !changed {
		t.Fatalf("resume = (%v, %v), want (true, nil)", changed, err)
	}
	changed, err = s.SetPaused(ctx, c.ID, false, "user-2")
	if err != nil || changed {
		t.Fatalf("second resume = (%v, %v), want (false, nil)", changed, err)
	}
	got, _ = s.GetCoordinatorByID(ctx, c.ID)
	if got.PausedAt != nil || got.PausedBy != "" {
		t.Fatalf("after resume: %+v", got)
	}
}

func TestSetPaused_AllowedWithAutonomyOffAndLeavesSettings(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	c := newPausableCoordinator(t, s)
	before, _ := s.GetCoordinatorByID(ctx, c.ID)
	if _, err := s.SetPaused(ctx, c.ID, true, "u"); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetCoordinatorByID(ctx, c.ID)
	if after.AutonomyEnabled != before.AutonomyEnabled || after.PolicyRevision != before.PolicyRevision ||
		after.ConfigRevision != before.ConfigRevision || after.CostCeilingSubcents != nil {
		t.Fatalf("pause changed settings: before=%+v after=%+v", before, after)
	}
}

func TestSetPaused_UnknownCoordinator(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.SetPaused(context.Background(), "missing", true, "u"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestPatchCoordinator_DoesNotClobberPause(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	c := newPausableCoordinator(t, s)
	if _, err := s.SetPaused(ctx, c.ID, true, "u"); err != nil {
		t.Fatal(err)
	}
	name := "renamed"
	if _, _, err := s.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Name: &name}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetCoordinatorByID(ctx, c.ID)
	if got.PausedAt == nil || got.PausedBy != "u" {
		t.Fatalf("patch clobbered the pause: %+v", got)
	}
}

func TestIsPaused_AndPausedIDs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, b := newPausableCoordinator(t, s), newPausableCoordinator(t, s)
	if paused, err := s.IsPaused(ctx, a.ID); err != nil || paused {
		t.Fatalf("IsPaused = (%v, %v)", paused, err)
	}
	if paused, err := s.IsPaused(ctx, "no-row"); err != nil || paused {
		t.Fatalf("missing row IsPaused = (%v, %v), want (false, nil)", paused, err)
	}
	_, _ = s.SetPaused(ctx, b.ID, true, "u")
	_, _ = s.SetPaused(ctx, a.ID, true, "u")
	ids, err := s.PausedCoordinatorIDs(ctx)
	if err != nil || len(ids) != 2 || ids[0] > ids[1] {
		t.Fatalf("PausedCoordinatorIDs = %v, %v (want 2 ids ordered)", ids, err)
	}
}

func TestDeleteCoordinator_RemovesPauseState(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	c := newPausableCoordinator(t, s)
	_, _ = s.SetPaused(ctx, c.ID, true, "u")
	if err := s.DeleteCoordinator(ctx, "ws-1", c.ID); err != nil {
		t.Fatal(err)
	}
	ids, _ := s.PausedCoordinatorIDs(ctx)
	if len(ids) != 0 {
		t.Fatalf("paused ids after delete = %v", ids)
	}
}
