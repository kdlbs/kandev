package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestListUnresolvedChildStallTurnsPagesPastUnresolvedRows proves a page cursor
// reaches a newer candidate behind more than one page of older candidates that
// stay unresolved, and that resolved and non-child turns are never listed.
func TestListUnresolvedChildStallTurnsPagesPastUnresolvedRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedSessionForTurns(t, repo, "task-child-stall-scan", "session-child-stall-scan")
	base := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	start := models.ChildStallStart{ParentTaskID: "parent", WorkspaceID: "ws", TransitionID: 1}.ToMap()
	create := func(id string, at time.Time, metadata map[string]interface{}) {
		t.Helper()
		completed := at
		turn := &models.Turn{
			ID: id, TaskSessionID: "session-child-stall-scan", TaskID: "task-child-stall-scan",
			StartedAt: at, CompletedAt: &completed, Metadata: metadata,
		}
		if err := repo.CreateTurn(ctx, turn); err != nil {
			t.Fatalf("CreateTurn(%s): %v", id, err)
		}
	}
	const limit = 3
	for i := 0; i < limit+1; i++ {
		create(fmt.Sprintf("old-%d", i), base.Add(time.Duration(i)*time.Minute), map[string]interface{}{
			models.TurnMetaKeyChildStallStart: start,
			models.TurnMetaKeyChildStall:      models.ChildStallState{State: "waiting_parent"}.ToMap(),
		})
	}
	create("resolved", base.Add(10*time.Minute), map[string]interface{}{
		models.TurnMetaKeyChildStallStart:    start,
		models.TurnMetaKeyChildStallResolved: true,
	})
	create("not-a-child", base.Add(11*time.Minute), nil)
	create("newer", base.Add(12*time.Minute), map[string]interface{}{models.TurnMetaKeyChildStallStart: start})

	var seen []string
	var cursor *models.ChildStallScanCursor
	for page := 0; page < 10; page++ {
		turns, err := repo.ListUnresolvedChildStallTurns(ctx, base.Add(-time.Hour), cursor, limit)
		if err != nil {
			t.Fatalf("ListUnresolvedChildStallTurns: %v", err)
		}
		for _, turn := range turns {
			seen = append(seen, turn.ID)
		}
		if len(turns) < limit {
			break
		}
		last := turns[len(turns)-1]
		cursor = &models.ChildStallScanCursor{CompletedAt: *last.CompletedAt, TurnID: last.ID}
	}
	want := []string{"old-0", "old-1", "old-2", "old-3", "newer"}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Fatalf("paged turns = %v, want %v", seen, want)
	}
}
