package sqlite

// Behavioral coverage for ListTaskUsageEvents (docs/specs/task-cost-ledger/spec.md
// AC-16): (occurred_at, id) ascending total order, limit semantics, and the
// empty-non-nil-slice contract for an unknown task or a task with no rows.

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestListTaskUsageEvents_OrdersByOccurredAtThenID(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-order")

	base := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	// Two rows share the same occurred_at, so only insertion-order id can
	// break the tie; the third row is deliberately inserted with an earlier
	// occurred_at than the first two, to prove ordering is by occurred_at
	// first, not by insertion/id order.
	first := newTestUsageEvent("evt-order-a", "task-order", "")
	first.OccurredAt = base
	second := newTestUsageEvent("evt-order-b", "task-order", "")
	second.OccurredAt = base
	third := newTestUsageEvent("evt-order-c", "task-order", "")
	third.OccurredAt = base.Add(-time.Hour)

	for _, event := range []*models.TaskUsageEvent{first, second, third} {
		if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
			t.Fatalf("CreateTaskUsageEvent(%s): %v", event.UsageEventID, err)
		}
	}

	events, err := repo.ListTaskUsageEvents(context.Background(), "task-order", 0)
	if err != nil {
		t.Fatalf("ListTaskUsageEvents: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("len(events) = %d, want 3", len(events))
	}
	wantOrder := []string{"evt-order-c", "evt-order-a", "evt-order-b"}
	for i, want := range wantOrder {
		if events[i].UsageEventID != want {
			t.Errorf("events[%d].UsageEventID = %q, want %q", i, events[i].UsageEventID, want)
		}
	}
}

func TestListTaskUsageEvents_LimitNonPositiveMeansNoLimit(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-nolimit")
	for _, id := range []string{"evt-nl-1", "evt-nl-2", "evt-nl-3"} {
		if err := repo.CreateTaskUsageEvent(context.Background(), newTestUsageEvent(id, "task-nolimit", "")); err != nil {
			t.Fatalf("CreateTaskUsageEvent(%s): %v", id, err)
		}
	}

	for _, limit := range []int{0, -1} {
		events, err := repo.ListTaskUsageEvents(context.Background(), "task-nolimit", limit)
		if err != nil {
			t.Fatalf("ListTaskUsageEvents(limit=%d): %v", limit, err)
		}
		if len(events) != 3 {
			t.Errorf("ListTaskUsageEvents(limit=%d) len = %d, want 3", limit, len(events))
		}
	}
}

func TestListTaskUsageEvents_LimitAppliedInAscendingOrder(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-limit")

	base := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	for i, id := range []string{"evt-lim-1", "evt-lim-2", "evt-lim-3"} {
		event := newTestUsageEvent(id, "task-limit", "")
		event.OccurredAt = base.Add(time.Duration(i) * time.Minute)
		if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
			t.Fatalf("CreateTaskUsageEvent(%s): %v", id, err)
		}
	}

	events, err := repo.ListTaskUsageEvents(context.Background(), "task-limit", 2)
	if err != nil {
		t.Fatalf("ListTaskUsageEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].UsageEventID != "evt-lim-1" || events[1].UsageEventID != "evt-lim-2" {
		t.Errorf("events = [%s, %s], want [evt-lim-1, evt-lim-2]", events[0].UsageEventID, events[1].UsageEventID)
	}
}

func TestListTaskUsageEvents_UnknownTask_ReturnsEmptyNonNilSlice(t *testing.T) {
	repo := newUsageEventsTestRepo(t)

	events, err := repo.ListTaskUsageEvents(context.Background(), "task-does-not-exist", 0)
	if err != nil {
		t.Fatalf("ListTaskUsageEvents: %v", err)
	}
	if events == nil {
		t.Fatal("events = nil, want a non-nil empty slice")
	}
	if len(events) != 0 {
		t.Errorf("len(events) = %d, want 0", len(events))
	}
}

func TestListTaskUsageEvents_TaskWithNoRows_ReturnsEmptyNonNilSlice(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-empty")

	events, err := repo.ListTaskUsageEvents(context.Background(), "task-empty", 0)
	if err != nil {
		t.Fatalf("ListTaskUsageEvents: %v", err)
	}
	if events == nil {
		t.Fatal("events = nil, want a non-nil empty slice")
	}
	if len(events) != 0 {
		t.Errorf("len(events) = %d, want 0", len(events))
	}
}

func TestListSessionUsageTurnsAndEventsByTurn(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-turn-list")
	createUsageEventsTestSession(t, repo, "session-turn-list", "task-turn-list")

	cacheWrite := int64(5)
	for _, event := range []*models.TaskUsageEvent{
		{UsageEventID: "evt-turn-direct", TaskID: "task-turn-list", SessionID: "session-turn-list", TurnID: "turn-1", NativeScope: "direct", MeasurementSource: "response", UsageCompleteness: "exact", UsageSchemaVersion: 1, ProviderThreadID: "thread-root", ProviderTurnID: "native-1", ProviderResponseID: "response-1", ReportedCacheWriteTokens: &cacheWrite, CostSource: "unpriced", ContractVersion: 1, OccurredAt: time.Now().UTC(), CreatedAt: time.Now().UTC()},
		{UsageEventID: "evt-turn-child", TaskID: "task-turn-list", SessionID: "session-turn-list", TurnID: "turn-1", NativeScope: "child", MeasurementSource: "response", UsageCompleteness: "exact", UsageSchemaVersion: 1, ProviderThreadID: "thread-child", ProviderTurnID: "native-child-1", ProviderResponseID: "response-child-1", CostSource: "models_dev_list", ContractVersion: 1, OccurredAt: time.Now().UTC(), CreatedAt: time.Now().UTC()},
		{UsageEventID: "evt-turn-next", TaskID: "task-turn-list", SessionID: "session-turn-list", TurnID: "turn-2", ContractVersion: 1, CostSource: "unpriced", OccurredAt: time.Now().UTC(), CreatedAt: time.Now().UTC()},
	} {
		if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
			t.Fatalf("CreateTaskUsageEvent(%s): %v", event.UsageEventID, err)
		}
	}

	turns, err := repo.ListSessionUsageTurnCursors(context.Background(), "session-turn-list", 0, 1)
	if err != nil {
		t.Fatalf("ListSessionUsageTurnCursors first page: %v", err)
	}
	if len(turns) != 1 || turns[0].TurnID != "turn-2" || turns[0].Cursor <= 0 {
		t.Fatalf("first page = %#v, want newest turn-2 with a positive cursor", turns)
	}
	next, err := repo.ListSessionUsageTurnCursors(context.Background(), "session-turn-list", turns[0].Cursor, 1)
	if err != nil {
		t.Fatalf("ListSessionUsageTurnCursors next page: %v", err)
	}
	if len(next) != 1 || next[0].TurnID != "turn-1" {
		t.Fatalf("next page = %#v, want older turn-1", next)
	}

	rows, err := repo.ListSessionUsageEventsByTurn(context.Background(), "session-turn-list", "turn-1")
	if err != nil {
		t.Fatalf("ListSessionUsageEventsByTurn: %v", err)
	}
	if len(rows) != 2 || rows[0].ProviderResponseID != "response-1" || rows[0].ReportedCacheWriteTokens == nil || *rows[0].ReportedCacheWriteTokens != 5 || rows[1].NativeScope != "child" {
		t.Fatalf("turn rows = %#v, want direct and child metadata round-tripped", rows)
	}
}
