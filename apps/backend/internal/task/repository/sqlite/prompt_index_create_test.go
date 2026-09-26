package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestLiveCreateAdvancesCollidingTimestamps covers the atomic per-session
// create boundary for live user messages: with a fixed clock, every create
// starts with the same timestamp, and the boundary advances each new message
// by one microsecond tick so reread order stays distinct and consistent. The
// one-tick advance stays within the bounded lead window.
func TestLiveCreateAdvancesCollidingTimestamps(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-LIVE", "sess-LIVE", "turn-LIVE")

	fixed := time.Date(2026, 8, 19, 10, 0, 0, 123456789, time.UTC)
	repo.clockNow = func() time.Time { return fixed }

	// Reverse-ordered IDs: the later-created message has the smaller id.
	first := &models.Message{ID: "live-zzz", TaskSessionID: "sess-LIVE", TurnID: "turn-LIVE", AuthorType: models.MessageAuthorUser, Content: "first"}
	if err := repo.CreateMessage(ctx, first); err != nil {
		t.Fatalf("first create: %v", err)
	}
	second := &models.Message{ID: "live-aaa", TaskSessionID: "sess-LIVE", TurnID: "turn-LIVE", AuthorType: models.MessageAuthorUser, Content: "second"}
	if err := repo.CreateMessage(ctx, second); err != nil {
		t.Fatalf("second create: %v", err)
	}

	if first.PromptIndex != 1 || second.PromptIndex != 2 {
		t.Fatalf("ordinals = (%d, %d), want (1, 2)", first.PromptIndex, second.PromptIndex)
	}
	// The boundary advanced the second message by one ordering-key tick (the
	// first row retains its full nanosecond precision, so the raw gap may be
	// less than a microsecond; the normalized keys differ by exactly one
	// microsecond) and the timestamps stay strictly ordered.
	if !second.CreatedAt.After(first.CreatedAt) {
		t.Errorf("second.CreatedAt = %v, want strictly after first %v", second.CreatedAt, first.CreatedAt)
	}
	if got := formatPromptKey(second.CreatedAt); got != formatPromptKey(first.CreatedAt.Add(time.Microsecond)) {
		t.Errorf("key advance = %q, want %q (one microsecond tick)", got, formatPromptKey(first.CreatedAt.Add(time.Microsecond)))
	}

	// Reread order and indexed reads agree with the assigned ordinals.
	got1, err := repo.GetMessageWithPromptIndex(ctx, "live-zzz")
	if err != nil {
		t.Fatalf("reread first: %v", err)
	}
	got2, err := repo.GetMessageWithPromptIndex(ctx, "live-aaa")
	if err != nil {
		t.Fatalf("reread second: %v", err)
	}
	if got1.PromptIndex != 1 || got2.PromptIndex != 2 {
		t.Errorf("reread ordinals = (%d, %d), want (1, 2)", got1.PromptIndex, got2.PromptIndex)
	}
}

// TestLiveCreateBackwardClockClampsInsteadOfBlocking pins the durable-ordinal
// regression: when the host clock is behind the session's newest user message
// (here by more than the old one-minute lead window), a live create must
// still succeed. The ordering timestamp clamps forward by one tick — the
// ordinal comes from the session's durable sequence counter, never from the
// timestamp — so a backward clock correction cannot block new prompts.
func TestLiveCreateBackwardClockClampsInsteadOfBlocking(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-SKEW", "sess-SKEW", "turn-SKEW")

	fixed := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	repo.clockNow = func() time.Time { return fixed }

	// An explicit seed two minutes in the future (relative to the fixed
	// clock) is valid on its own: it is strictly after the empty session max.
	seed := &models.Message{
		ID: "skew-seed", TaskSessionID: "sess-SKEW", TurnID: "turn-SKEW",
		AuthorType: models.MessageAuthorUser, Content: "future seed",
		CreatedAt: fixed.Add(2 * time.Minute),
	}
	if err := repo.CreateMessage(ctx, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if seed.PromptIndex != 1 {
		t.Fatalf("seed ordinal = %d, want 1", seed.PromptIndex)
	}

	// Backward clock correction: a live create at a clock ten minutes behind
	// the seed must not be blocked; its ordering timestamp clamps to one tick
	// after the seed and it receives the next durable ordinal.
	repo.clockNow = func() time.Time { return fixed.Add(-10 * time.Minute) }
	live := &models.Message{
		ID: "skew-live", TaskSessionID: "sess-SKEW", TurnID: "turn-SKEW",
		AuthorType: models.MessageAuthorUser, Content: "live",
	}
	if err := repo.CreateMessage(ctx, live); err != nil {
		t.Fatalf("backward-clock create must not block: %v", err)
	}
	if live.PromptIndex != 2 {
		t.Errorf("backward-clock ordinal = %d, want 2", live.PromptIndex)
	}
	if got := formatPromptKey(live.CreatedAt); got != formatPromptKey(seed.CreatedAt.Add(time.Microsecond)) {
		t.Errorf("clamped key = %q, want %q (one tick after seed)", got, formatPromptKey(seed.CreatedAt.Add(time.Microsecond)))
	}
	got, err := repo.GetMessageWithPromptIndex(ctx, live.ID)
	if err != nil {
		t.Fatalf("reread live: %v", err)
	}
	if got.PromptIndex != 2 {
		t.Errorf("reread ordinal = %d, want 2", got.PromptIndex)
	}
}

// TestPromptIndexDurableAcrossDeletion covers the delete-after-publish
// regression: ordinals already delivered to clients must not change when an
// earlier prompt is deleted, and the session's durable sequence must never
// reuse a deleted prompt's ordinal.
func TestPromptIndexDurableAcrossDeletion(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-DEL", "sess-DEL", "turn-DEL")
	repo.clockNow = func() time.Time { return time.Date(2026, 8, 19, 11, 0, 0, 0, time.UTC) }

	a := &models.Message{ID: "del-a", TaskSessionID: "sess-DEL", TurnID: "turn-DEL", AuthorType: models.MessageAuthorUser, Content: "A"}
	if err := repo.CreateMessage(ctx, a); err != nil {
		t.Fatalf("create A: %v", err)
	}
	b := &models.Message{ID: "del-b", TaskSessionID: "sess-DEL", TurnID: "turn-DEL", AuthorType: models.MessageAuthorUser, Content: "B"}
	if err := repo.CreateMessage(ctx, b); err != nil {
		t.Fatalf("create B: %v", err)
	}
	if a.PromptIndex != 1 || b.PromptIndex != 2 {
		t.Fatalf("initial ordinals = (%d, %d), want (1, 2)", a.PromptIndex, b.PromptIndex)
	}

	// Delete the earlier prompt A after B (#2) was published.
	if err := repo.DeleteMessage(ctx, a.ID); err != nil {
		t.Fatalf("delete A: %v", err)
	}
	gotB, err := repo.GetMessageWithPromptIndex(ctx, b.ID)
	if err != nil {
		t.Fatalf("reread B: %v", err)
	}
	if gotB.PromptIndex != 2 {
		t.Errorf("B ordinal after deleting A = %d, want 2 (published ordinal must not renumber)", gotB.PromptIndex)
	}

	// A new prompt must continue the sequence, never reuse #1.
	c := &models.Message{ID: "del-c", TaskSessionID: "sess-DEL", TurnID: "turn-DEL", AuthorType: models.MessageAuthorUser, Content: "C"}
	if err := repo.CreateMessage(ctx, c); err != nil {
		t.Fatalf("create C: %v", err)
	}
	if c.PromptIndex != 3 {
		t.Errorf("C ordinal = %d, want 3 (no reuse after delete)", c.PromptIndex)
	}
	gotC, err := repo.GetMessageWithPromptIndex(ctx, c.ID)
	if err != nil {
		t.Fatalf("reread C: %v", err)
	}
	if gotC.PromptIndex != 3 {
		t.Errorf("reread C ordinal = %d, want 3", gotC.PromptIndex)
	}
}

// TestExplicitUserTimestampMustBeStrictlyAfterMax covers the zero-vs-explicit
// CreatedAt branch: explicit imports preserve only valid strictly-after-max
// timestamps; an explicit timestamp at or before the session's newest user
// message is rejected and leaves the model untouched.
func TestExplicitUserTimestampMustBeStrictlyAfterMax(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-EXP", "sess-EXP", "turn-EXP")

	base := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	first := &models.Message{
		ID: "exp-1", TaskSessionID: "sess-EXP", TurnID: "turn-EXP",
		AuthorType: models.MessageAuthorUser, Content: "first", CreatedAt: base,
	}
	if err := repo.CreateMessage(ctx, first); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if first.PromptIndex != 1 {
		t.Fatalf("first ordinal = %d, want 1", first.PromptIndex)
	}

	// Equal timestamp: not strictly after max → rejected.
	dup := &models.Message{
		ID: "exp-dup", TaskSessionID: "sess-EXP", TurnID: "turn-EXP",
		AuthorType: models.MessageAuthorUser, Content: "duplicate", CreatedAt: base,
	}
	err := repo.CreateMessage(ctx, dup)
	if !errors.Is(err, ErrMessageTimestampNotAfterNewest) {
		t.Fatalf("equal-timestamp create error = %v, want ErrMessageTimestampNotAfterNewest", err)
	}
	if !dup.CreatedAt.Equal(base) || dup.PromptIndex != 0 {
		t.Errorf("rejected create mutated model: CreatedAt=%v PromptIndex=%d", dup.CreatedAt, dup.PromptIndex)
	}

	// Backward timestamp: also rejected.
	older := &models.Message{
		ID: "exp-older", TaskSessionID: "sess-EXP", TurnID: "turn-EXP",
		AuthorType: models.MessageAuthorUser, Content: "older", CreatedAt: base.Add(-time.Second),
	}
	if err := repo.CreateMessage(ctx, older); !errors.Is(err, ErrMessageTimestampNotAfterNewest) {
		t.Fatalf("older-timestamp create error = %v, want ErrMessageTimestampNotAfterNewest", err)
	}

	// Strictly-after timestamp: preserved verbatim with the next ordinal.
	second := &models.Message{
		ID: "exp-2", TaskSessionID: "sess-EXP", TurnID: "turn-EXP",
		AuthorType: models.MessageAuthorUser, Content: "second", CreatedAt: base.Add(time.Second),
	}
	if err := repo.CreateMessage(ctx, second); err != nil {
		t.Fatalf("second create: %v", err)
	}
	if second.PromptIndex != 2 {
		t.Fatalf("second ordinal = %d, want 2", second.PromptIndex)
	}
	if !second.CreatedAt.Equal(base.Add(time.Second)) {
		t.Errorf("explicit timestamp not preserved: %v", second.CreatedAt)
	}
}

// TestFailedCreateAttemptKeepsModelClean proves a failed repository attempt
// (here: a duplicate primary key insert after the boundary computed the
// ordinal) does not leave CreatedAt/PromptIndex on the retried model.
func TestFailedCreateAttemptKeepsModelClean(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-RETRY", "sess-RETRY", "turn-RETRY")

	winner := &models.Message{
		ID: "retry-id", TaskSessionID: "sess-RETRY", TurnID: "turn-RETRY",
		AuthorType: models.MessageAuthorUser, Content: "winner",
	}
	if err := repo.CreateMessage(ctx, winner); err != nil {
		t.Fatalf("winner create: %v", err)
	}
	if winner.PromptIndex != 1 {
		t.Fatalf("winner ordinal = %d, want 1", winner.PromptIndex)
	}

	// Same ID: the boundary computes an ordinal, then the insert fails on the
	// primary key. The caller-visible fields must be restored so the retry
	// sees a clean model.
	retry := &models.Message{
		ID: "retry-id", TaskSessionID: "sess-RETRY", TurnID: "turn-RETRY",
		AuthorType: models.MessageAuthorUser, Content: "retry",
	}
	if err := repo.CreateMessage(ctx, retry); err == nil {
		t.Fatal("duplicate create succeeded, want failure")
	}
	if !retry.CreatedAt.IsZero() || retry.PromptIndex != 0 || !retry.UpdatedAt.IsZero() {
		t.Errorf("failed attempt mutated model: CreatedAt=%v UpdatedAt=%v PromptIndex=%d",
			retry.CreatedAt, retry.UpdatedAt, retry.PromptIndex)
	}

	// The winning row's ordinal is stable.
	got, err := repo.GetMessageWithPromptIndex(ctx, "retry-id")
	if err != nil {
		t.Fatalf("reread winner: %v", err)
	}
	if got.PromptIndex != 1 {
		t.Errorf("winner ordinal after failed retry = %d, want 1", got.PromptIndex)
	}
}
