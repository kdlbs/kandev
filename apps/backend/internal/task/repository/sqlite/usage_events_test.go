package sqlite

// Behavioral coverage for CreateTaskUsageEvent (docs/specs/task-cost-ledger/spec.md
// AC-11, AC-12, AC-13, AC-14, AC-30, AC-32, AC-33): the atomic ledger-insert +
// rollup transaction and its insert-failure classification/reaction.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func newTestUsageEvent(usageEventID, taskID, sessionID string) *models.TaskUsageEvent {
	now := time.Now().UTC()
	tokensCachedRead := int64(20)
	tokensCachedWrite := int64(5)
	tokensOut := int64(30)
	return &models.TaskUsageEvent{
		UsageEventID:      usageEventID,
		TaskID:            taskID,
		SessionID:         sessionID,
		AgentType:         "claude",
		Model:             "claude-sonnet-5",
		Provider:          "anthropic",
		TokensIn:          100,
		TokensCachedRead:  &tokensCachedRead,
		TokensCachedWrite: &tokensCachedWrite,
		TokensOut:         &tokensOut,
		TokensTotal:       155,
		CostSubcents:      42,
		CostSource:        "actual",
		ContractVersion:   1,
		OccurredAt:        now,
		CreatedAt:         now,
	}
}

func readTaskSessionRollup(t *testing.T, repo *Repository, sessionID string) (tokensIn, tokensCachedIn, tokensOut, costSubcents int64) {
	t.Helper()
	if err := repo.db.QueryRowx(repo.db.Rebind(`
		SELECT COALESCE(tokens_in, 0), COALESCE(tokens_cached_in, 0), COALESCE(tokens_out, 0), COALESCE(cost_subcents, 0)
		  FROM task_sessions WHERE id = ?
	`), sessionID).Scan(&tokensIn, &tokensCachedIn, &tokensOut, &costSubcents); err != nil {
		t.Fatalf("read task_sessions rollup: %v", err)
	}
	return
}

func countTaskUsageEventRows(t *testing.T, repo *Repository) int {
	t.Helper()
	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM task_usage_events`); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

func TestCreateTaskUsageEvent_HappyPath_InsertsRowAndIncrementsRollup(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-happy")
	createUsageEventsTestSession(t, repo, "session-happy", "task-happy")

	event := newTestUsageEvent("evt-happy", "task-happy", "session-happy")
	if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateTaskUsageEvent: %v", err)
	}

	if got := countTaskUsageEventRows(t, repo); got != 1 {
		t.Fatalf("row count = %d, want 1", got)
	}
	tokensIn, tokensCachedIn, tokensOut, costSubcents := readTaskSessionRollup(t, repo, "session-happy")
	if tokensIn != 100 || tokensCachedIn != 25 || tokensOut != 30 || costSubcents != 42 {
		t.Errorf("rollup = (%d,%d,%d,%d), want (100,25,30,42)", tokensIn, tokensCachedIn, tokensOut, costSubcents)
	}
}

// TestCreateTaskUsageEvent_SameSessionAndTurn_BothRowsCommitAndRollupSums
// pins AC-15: (session_id, turn_id) is an aggregation key for reads, not a
// uniqueness constraint on writes - a single turn producing multiple ledger
// rows (e.g. more than one prompt-usage event in one turn) must not be
// rejected or collapsed.
func TestCreateTaskUsageEvent_SameSessionAndTurn_BothRowsCommitAndRollupSums(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-same-turn")
	createUsageEventsTestSession(t, repo, "session-same-turn", "task-same-turn")

	first := newTestUsageEvent("evt-same-turn-1", "task-same-turn", "session-same-turn")
	first.TurnID = "turn-shared"
	if err := repo.CreateTaskUsageEvent(context.Background(), first); err != nil {
		t.Fatalf("CreateTaskUsageEvent (first): %v", err)
	}

	second := newTestUsageEvent("evt-same-turn-2", "task-same-turn", "session-same-turn")
	second.TurnID = "turn-shared"
	if err := repo.CreateTaskUsageEvent(context.Background(), second); err != nil {
		t.Fatalf("CreateTaskUsageEvent (second, same session_id+turn_id): %v", err)
	}

	if got := countTaskUsageEventRows(t, repo); got != 2 {
		t.Fatalf("row count = %d, want 2 (both rows for the shared turn must commit)", got)
	}

	events, err := repo.ListTaskUsageEvents(context.Background(), "task-same-turn", 0)
	if err != nil {
		t.Fatalf("ListTaskUsageEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("ListTaskUsageEvents returned %d events, want 2", len(events))
	}
	if events[0].TurnID != "turn-shared" || events[1].TurnID != "turn-shared" {
		t.Errorf("turn IDs = [%q, %q], want both rows to carry turn_id=turn-shared", events[0].TurnID, events[1].TurnID)
	}
	if events[0].UsageEventID == events[1].UsageEventID {
		t.Errorf("both rows have usage_event_id %q, want two distinct rows", events[0].UsageEventID)
	}

	tokensIn, tokensCachedIn, tokensOut, costSubcents := readTaskSessionRollup(t, repo, "session-same-turn")
	if tokensIn != 200 || tokensCachedIn != 50 || tokensOut != 60 || costSubcents != 84 {
		t.Errorf("rollup = (%d,%d,%d,%d), want (200,50,60,84) (both rows must contribute)", tokensIn, tokensCachedIn, tokensOut, costSubcents)
	}
}

// TestCreateTaskUsageEvent_NilTokenPointersCoalesceToZeroInRollup pins AC-12:
// a not-recorded token count (nil pointer) contributes zero to the rollup
// rather than propagating SQL NULL through the sum.
func TestCreateTaskUsageEvent_NilTokenPointersCoalesceToZeroInRollup(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-nil-tokens")
	createUsageEventsTestSession(t, repo, "session-nil-tokens", "task-nil-tokens")

	event := newTestUsageEvent("evt-nil-tokens", "task-nil-tokens", "session-nil-tokens")
	event.TokensCachedRead = nil
	event.TokensCachedWrite = nil
	event.TokensOut = nil

	if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateTaskUsageEvent: %v", err)
	}

	tokensIn, tokensCachedIn, tokensOut, costSubcents := readTaskSessionRollup(t, repo, "session-nil-tokens")
	if tokensIn != 100 || tokensCachedIn != 0 || tokensOut != 0 || costSubcents != 42 {
		t.Errorf("rollup = (%d,%d,%d,%d), want (100,0,0,42)", tokensIn, tokensCachedIn, tokensOut, costSubcents)
	}
}

// TestCreateTaskUsageEvent_DuplicateUsageEventID_ReturnsErrDuplicateNoRollup
// pins AC-13/AC-14/AC-32: a redelivered usage_event_id records no new row
// and increments the rollup no further, reported as a duplicate rather than
// an error.
func TestCreateTaskUsageEvent_DuplicateUsageEventID_ReturnsErrDuplicateNoRollup(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-dup")
	createUsageEventsTestSession(t, repo, "session-dup", "task-dup")

	first := newTestUsageEvent("evt-dup", "task-dup", "session-dup")
	if err := repo.CreateTaskUsageEvent(context.Background(), first); err != nil {
		t.Fatalf("first CreateTaskUsageEvent: %v", err)
	}

	redelivered := newTestUsageEvent("evt-dup", "task-dup", "session-dup")
	err := repo.CreateTaskUsageEvent(context.Background(), redelivered)
	if err != ErrDuplicateUsageEvent {
		t.Fatalf("redelivered CreateTaskUsageEvent error = %v, want ErrDuplicateUsageEvent", err)
	}

	if got := countTaskUsageEventRows(t, repo); got != 1 {
		t.Fatalf("row count = %d, want 1 (redelivery must not insert a second row)", got)
	}
	tokensIn, _, _, _ := readTaskSessionRollup(t, repo, "session-dup")
	if tokensIn != 100 {
		t.Errorf("tokens_in = %d, want 100 (redelivery must not double-increment the rollup)", tokensIn)
	}
}

// TestCreateTaskUsageEvent_ConcurrentInsertsSameUsageEventID_ExactlyOneCommits
// pins AC-32's unique-violation branch under a genuine race, not a
// sequential redelivery: two deliveries of the same event, racing each
// other, with the producer's deterministic identifier (spec's "Concurrency"
// scenario). Both goroutines are released together via a start barrier so
// they race for real; SQLITE_BUSY contention is expected and handled by
// CreateTaskUsageEvent's own transient-retry loop, not by this test.
// Exactly one call must succeed and the other must observe
// ErrDuplicateUsageEvent - no other error, and no silent double-drop - with
// the rollup incremented exactly once, matching the loser's increment
// sharing the loser's rolled-back transaction.
func TestCreateTaskUsageEvent_ConcurrentInsertsSameUsageEventID_ExactlyOneCommits(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-race")
	createUsageEventsTestSession(t, repo, "session-race", "task-race")

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			event := newTestUsageEvent("evt-race", "task-race", "session-race")
			<-start
			errs[i] = repo.CreateTaskUsageEvent(context.Background(), event)
		}(i)
	}
	close(start)
	wg.Wait()

	successes, duplicates := 0, 0
	for _, err := range errs {
		switch err {
		case nil:
			successes++
		case ErrDuplicateUsageEvent:
			duplicates++
		default:
			t.Fatalf("unexpected error from a racing insert: %v", err)
		}
	}
	if successes != 1 || duplicates != 1 {
		t.Fatalf("successes=%d duplicates=%d, want exactly one of each", successes, duplicates)
	}

	if got := countTaskUsageEventRows(t, repo); got != 1 {
		t.Fatalf("row count = %d, want 1 (the losing insert must not leave a row behind)", got)
	}
	tokensIn, _, _, _ := readTaskSessionRollup(t, repo, "session-race")
	if tokensIn != 100 {
		t.Errorf("tokens_in = %d, want 100 (the rollup must be incremented exactly once, not twice or zero times)", tokensIn)
	}
}

// TestCreateTaskUsageEvent_SessionForeignKeyViolation_RetriesOnceWithSessionCleared
// pins AC-32(b): an insert naming a session that does not exist is retried
// exactly once with session_id cleared, and that retry does not increment
// any session's rollup.
func TestCreateTaskUsageEvent_SessionForeignKeyViolation_RetriesOnceWithSessionCleared(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-fk-retry")

	event := newTestUsageEvent("evt-fk-retry", "task-fk-retry", "session-does-not-exist")
	if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateTaskUsageEvent: %v", err)
	}

	if got := countTaskUsageEventRows(t, repo); got != 1 {
		t.Fatalf("row count = %d, want 1", got)
	}
	var sessionID *string
	if err := repo.db.Get(&sessionID, `SELECT session_id FROM task_usage_events WHERE usage_event_id = ?`, "evt-fk-retry"); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if sessionID != nil {
		t.Errorf("session_id = %v, want nil (FK retry must clear it)", *sessionID)
	}
}

// TestCreateTaskUsageEvent_SecondForeignKeyFailure_ReturnsError pins AC-32(b)'s
// tail: a second failure after the one allotted FK retry is returned as-is,
// not retried again.
func TestCreateTaskUsageEvent_SecondForeignKeyFailure_ReturnsError(t *testing.T) {
	repo := newUsageEventsTestRepo(t)

	event := newTestUsageEvent("evt-fk-double", "task-does-not-exist", "session-does-not-exist")
	err := repo.CreateTaskUsageEvent(context.Background(), event)
	if err == nil {
		t.Fatal("expected an error when both task_id and session_id are invalid, got nil")
	}
	if err == ErrDuplicateUsageEvent {
		t.Fatalf("error = ErrDuplicateUsageEvent, want a foreign-key failure")
	}
	if got := countTaskUsageEventRows(t, repo); got != 0 {
		t.Errorf("row count = %d, want 0 (both attempts must roll back)", got)
	}
}

// TestCreateTaskUsageEvent_SessionBelongsToDifferentTask_ClearsSessionNoRollup
// pins AC-33: a session that exists but belongs to a different task is
// recorded under the payload's task_id with session_id cleared, the same
// terminal handling as the foreign-key retry case.
func TestCreateTaskUsageEvent_SessionBelongsToDifferentTask_ClearsSessionNoRollup(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-owner")
	createUsageEventsTestSession(t, repo, "session-owner", "task-owner")
	createUsageEventsTestTask(t, repo, "task-other")

	event := newTestUsageEvent("evt-mismatch", "task-other", "session-owner")
	if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateTaskUsageEvent: %v", err)
	}

	var gotTaskID string
	var sessionID *string
	if err := repo.db.QueryRowx(`SELECT task_id, session_id FROM task_usage_events WHERE usage_event_id = ?`, "evt-mismatch").
		Scan(&gotTaskID, &sessionID); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if gotTaskID != "task-other" {
		t.Errorf("task_id = %q, want %q", gotTaskID, "task-other")
	}
	if sessionID != nil {
		t.Errorf("session_id = %v, want nil (mismatched session must be cleared)", *sessionID)
	}

	tokensIn, _, _, _ := readTaskSessionRollup(t, repo, "session-owner")
	if tokensIn != 0 {
		t.Errorf("session-owner tokens_in = %d, want 0 (mismatched session must not be rolled up into)", tokensIn)
	}
}

// TestCreateTaskUsageEvent_SessionOwnershipLookupHardError_ReturnsImmediately
// pins R5-F1: a session-ownership lookup failure that is not "no such
// session" is returned immediately, without attempting any insert.
func TestCreateTaskUsageEvent_SessionOwnershipLookupHardError_ReturnsImmediately(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-hard-error")
	if _, err := repo.db.Exec(`DROP TABLE task_sessions`); err != nil {
		t.Fatalf("drop task_sessions to force a hard ownership-lookup error: %v", err)
	}

	event := newTestUsageEvent("evt-hard-error", "task-hard-error", "session-whatever")
	err := repo.CreateTaskUsageEvent(context.Background(), event)
	if err == nil {
		t.Fatal("expected an error when the ownership lookup itself fails, got nil")
	}
	if err == ErrDuplicateUsageEvent {
		t.Fatalf("error = ErrDuplicateUsageEvent, want the raw ownership-lookup error")
	}

	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM task_usage_events`); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 0 {
		t.Errorf("row count = %d, want 0 (a hard ownership-lookup error must not attempt an insert)", count)
	}
}

// TestCreateTaskUsageEvent_NullableFieldsRoundTripThroughList pins AC-30: a
// nil token pointer round-trips as nil (not a measured zero), and a
// measured-zero pointer round-trips distinctly as a non-nil zero.
func TestCreateTaskUsageEvent_NullableFieldsRoundTripThroughList(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-roundtrip")
	createUsageEventsTestSession(t, repo, "session-roundtrip", "task-roundtrip")

	event := newTestUsageEvent("evt-roundtrip", "task-roundtrip", "session-roundtrip")
	measuredZero := int64(0)
	event.TokensThought = nil
	event.TokensOut = &measuredZero
	event.PricingCatalogVersion = "catalog-v3"
	event.TurnID = "turn-1"

	if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateTaskUsageEvent: %v", err)
	}

	events, err := repo.ListTaskUsageEvents(context.Background(), "task-roundtrip", 0)
	if err != nil {
		t.Fatalf("ListTaskUsageEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	got := events[0]
	if got.TokensThought != nil {
		t.Errorf("TokensThought = %v, want nil (not recorded)", *got.TokensThought)
	}
	if got.TokensOut == nil || *got.TokensOut != 0 {
		t.Errorf("TokensOut = %v, want a non-nil measured zero", got.TokensOut)
	}
	if got.PricingCatalogVersion != "catalog-v3" {
		t.Errorf("PricingCatalogVersion = %q, want %q", got.PricingCatalogVersion, "catalog-v3")
	}
	if got.TurnID != "turn-1" {
		t.Errorf("TurnID = %q, want %q", got.TurnID, "turn-1")
	}
	if got.SessionID != "session-roundtrip" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "session-roundtrip")
	}
}
