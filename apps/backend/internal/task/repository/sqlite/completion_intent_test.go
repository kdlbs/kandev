package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCompletionIntentCreateOrGetAndCompareAndSet(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task"}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn", TaskID: "task", TaskSessionID: "session", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}

	intent := &models.CompletionIntent{
		ID: "intent", TaskID: "task", SessionID: "session", TurnID: "turn", WorkflowStepID: "step",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}
	created, got, err := repo.CreateOrGetCompletionIntent(ctx, intent)
	if err != nil || !created || got.ID != intent.ID {
		t.Fatalf("CreateOrGetCompletionIntent = (%v, %+v, %v), want created intent", created, got, err)
	}
	created, got, err = repo.CreateOrGetCompletionIntent(ctx, intent)
	if err != nil || created || got.ID != intent.ID {
		t.Fatalf("duplicate CreateOrGetCompletionIntent = (%v, %+v, %v)", created, got, err)
	}
	byTurn, err := repo.GetCompletionIntentForTurn(ctx, intent.SessionID, intent.TurnID)
	if err != nil || byTurn.ID != intent.ID {
		t.Fatalf("GetCompletionIntentForTurn = (%+v, %v), want %q", byTurn, err, intent.ID)
	}
	settled, err := repo.TransitionCompletionIntent(ctx, intent.ID, models.CompletionIntentStatePending, models.CompletionIntentStateSettling, now)
	if err != nil || !settled {
		t.Fatalf("TransitionCompletionIntent pending->settling = (%v, %v)", settled, err)
	}
	settled, err = repo.TransitionCompletionIntent(ctx, intent.ID, models.CompletionIntentStatePending, models.CompletionIntentStateSettling, now)
	if err != nil || settled {
		t.Fatalf("duplicate compare-and-set = (%v, %v), want false nil", settled, err)
	}
}

// TestClaimCompletionIntentForSettlementSetsLeaseDeadline covers the
// settling-lease mechanism itself: claiming stamps eligible_at with the
// caller-supplied lease deadline (not left at its pre-claim quiet-grace
// value), which is what lets ReclaimAbandonedSettlingCompletionIntents later
// distinguish an abandoned claim from one still within its bounded window.
func TestClaimCompletionIntentForSettlementSetsLeaseDeadline(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	seedCompletionIntentFixture(t, repo, "task", "session", "turn", "step")
	intent := &models.CompletionIntent{
		ID: "intent", TaskID: "task", SessionID: "session", TurnID: "turn", WorkflowStepID: "step",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, intent); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}

	leaseUntil := now.Add(2 * time.Minute)
	claimed, err := repo.ClaimCompletionIntentForSettlement(ctx, intent.ID, now, leaseUntil)
	if err != nil || !claimed {
		t.Fatalf("ClaimCompletionIntentForSettlement = (%v, %v), want true nil", claimed, err)
	}
	stored, err := repo.GetCompletionIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetCompletionIntent: %v", err)
	}
	if stored.State != models.CompletionIntentStateSettling {
		t.Fatalf("state = %q, want settling", stored.State)
	}
	if !stored.EligibleAt.Equal(leaseUntil) {
		t.Fatalf("eligible_at = %v, want lease deadline %v", stored.EligibleAt, leaseUntil)
	}

	// A second claim attempt against an already-settling row must fail: this
	// is the same compare-and-set discipline TransitionCompletionIntent
	// already provides for every other transition.
	claimedAgain, err := repo.ClaimCompletionIntentForSettlement(ctx, intent.ID, now, leaseUntil)
	if err != nil || claimedAgain {
		t.Fatalf("duplicate ClaimCompletionIntentForSettlement = (%v, %v), want false nil", claimedAgain, err)
	}
}

// TestReclaimAbandonedSettlingCompletionIntentsRecoversExpiredLeaseOnly is
// the crash-recovery regression: a process that dies between claiming an
// intent for settlement and finishing it leaves that row permanently
// "settling" (ListDueCompletionIntents only ever selects pending rows)
// unless something reclaims it. Only an intent whose lease has actually
// expired must be recovered; one still within its bounded window represents
// a settlement that may genuinely still be in progress on another instance
// and must not be raced.
func TestReclaimAbandonedSettlingCompletionIntentsRecoversExpiredLeaseOnly(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	seedCompletionIntentFixture(t, repo, "task-abandoned", "session-abandoned", "turn-abandoned", "step")
	abandoned := &models.CompletionIntent{
		ID: "abandoned", TaskID: "task-abandoned", SessionID: "session-abandoned", TurnID: "turn-abandoned",
		WorkflowStepID: "step", State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, abandoned); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent(abandoned): %v", err)
	}
	// Simulates a crash: claimed with a lease that has since expired, and
	// nothing ever transitioned it further.
	if _, err := repo.ClaimCompletionIntentForSettlement(ctx, abandoned.ID, now.Add(-5*time.Minute), now.Add(-3*time.Minute)); err != nil {
		t.Fatalf("claim abandoned: %v", err)
	}

	seedCompletionIntentFixture(t, repo, "task-live", "session-live", "turn-live", "step")
	live := &models.CompletionIntent{
		ID: "live", TaskID: "task-live", SessionID: "session-live", TurnID: "turn-live",
		WorkflowStepID: "step", State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, live); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent(live): %v", err)
	}
	// A genuinely in-progress settlement elsewhere: claimed just now, lease
	// still has minutes left.
	if _, err := repo.ClaimCompletionIntentForSettlement(ctx, live.ID, now, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("claim live: %v", err)
	}

	reclaimed, err := repo.ReclaimAbandonedSettlingCompletionIntents(ctx, now)
	if err != nil {
		t.Fatalf("ReclaimAbandonedSettlingCompletionIntents: %v", err)
	}
	if reclaimed != 1 {
		t.Fatalf("reclaimed = %d, want exactly 1", reclaimed)
	}

	storedAbandoned, err := repo.GetCompletionIntent(ctx, abandoned.ID)
	if err != nil {
		t.Fatalf("GetCompletionIntent(abandoned): %v", err)
	}
	if storedAbandoned.State != models.CompletionIntentStatePending {
		t.Fatalf("abandoned state = %q, want pending", storedAbandoned.State)
	}
	if storedAbandoned.EligibleAt.After(now) {
		t.Fatalf("abandoned eligible_at = %v, want <= %v so the next due scan retries it immediately", storedAbandoned.EligibleAt, now)
	}
	// The reclaimed row must be immediately visible to the normal due scan —
	// that is the entire point of resetting eligible_at.
	due, err := repo.ListDueCompletionIntents(ctx, now, 10)
	if err != nil {
		t.Fatalf("ListDueCompletionIntents: %v", err)
	}
	if len(due) != 1 || due[0].ID != abandoned.ID {
		t.Fatalf("ListDueCompletionIntents = %+v, want only the reclaimed abandoned intent", due)
	}

	storedLive, err := repo.GetCompletionIntent(ctx, live.ID)
	if err != nil {
		t.Fatalf("GetCompletionIntent(live): %v", err)
	}
	if storedLive.State != models.CompletionIntentStateSettling {
		t.Fatalf("live state = %q, want still settling (unexpired lease must not be reclaimed)", storedLive.State)
	}
}

// TestReleaseCompletionIntentSettlingClaimResetsEligibility covers the
// in-process retry path (transient turn-completion/task/session lookup
// failure inside reconcileCompletionIntentLocked): the release must reset
// eligible_at to now, not merely flip the state back to pending, or the next
// due scan would wait out the multi-minute settling lease before retrying a
// failure the process already knows about right now.
func TestReleaseCompletionIntentSettlingClaimResetsEligibility(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	seedCompletionIntentFixture(t, repo, "task", "session", "turn", "step")
	intent := &models.CompletionIntent{
		ID: "intent", TaskID: "task", SessionID: "session", TurnID: "turn", WorkflowStepID: "step",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, intent); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}
	if _, err := repo.ClaimCompletionIntentForSettlement(ctx, intent.ID, now, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("claim: %v", err)
	}

	released, err := repo.ReleaseCompletionIntentSettlingClaim(ctx, intent.ID, now)
	if err != nil || !released {
		t.Fatalf("ReleaseCompletionIntentSettlingClaim = (%v, %v), want true nil", released, err)
	}
	stored, err := repo.GetCompletionIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetCompletionIntent: %v", err)
	}
	if stored.State != models.CompletionIntentStatePending {
		t.Fatalf("state = %q, want pending", stored.State)
	}
	if !stored.EligibleAt.Equal(now) {
		t.Fatalf("eligible_at = %v, want reset to %v", stored.EligibleAt, now)
	}
}

// TestCreateOrGetCompletionIntentRejectsMismatchedTaskIdentity covers the
// data-integrity gap: individual foreign keys on task_id/session_id/turn_id
// each reference a valid row, but nothing previously required those rows to
// agree with each other. An intent claiming an unrelated task must be
// rejected rather than silently recorded.
func TestCreateOrGetCompletionIntentRejectsMismatchedTaskIdentity(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	seedCompletionIntentFixture(t, repo, "task-real", "session-real", "turn-real", "step")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-unrelated", Title: "Unrelated"}); err != nil {
		t.Fatalf("CreateTask(unrelated): %v", err)
	}

	_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-mismatch", TaskID: "task-unrelated", SessionID: "session-real", TurnID: "turn-real",
		WorkflowStepID: "step", State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	})
	if err == nil {
		t.Fatal("expected an error when the intent's TaskID does not match its turn's actual task")
	}
}

// TestCreateOrGetCompletionIntentRejectsNonPendingInitialState covers the
// other half of the same gap: an intent created directly in a terminal (or
// settling) state would be immediately ineligible for reconciliation but
// with no legal way back to pending, permanently ineligible.
func TestCreateOrGetCompletionIntentRejectsNonPendingInitialState(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	seedCompletionIntentFixture(t, repo, "task", "session", "turn", "step")

	_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-non-pending", TaskID: "task", SessionID: "session", TurnID: "turn",
		WorkflowStepID: "step", State: models.CompletionIntentStateSettled, RequestedAt: now, EligibleAt: now,
	})
	if err == nil {
		t.Fatal("expected an error when creating an intent outside the pending state")
	}
}

// TestGetCompletionIntentForTurnReturnsMostRecentAcrossSteps covers the
// obsolete-intent-after-a-step-move gap: the schema's uniqueness constraint
// is (session_id, turn_id, workflow_step_id), so the same turn can
// accumulate more than one intent row if it signals completion again under a
// later step after a workflow move. A caller settling "by turn" must resolve
// to the intent reflecting current reality, not whichever was requested
// first.
func TestGetCompletionIntentForTurnReturnsMostRecentAcrossSteps(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	seedCompletionIntentFixture(t, repo, "task", "session", "turn", "step1")

	older := &models.CompletionIntent{
		ID: "older", TaskID: "task", SessionID: "session", TurnID: "turn", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now.Add(-time.Minute), EligibleAt: now.Add(-time.Minute),
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, older); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent(older): %v", err)
	}
	// The workflow moved and the same turn signaled completion again under
	// the new step, producing a second row for the same (session, turn).
	newer := &models.CompletionIntent{
		ID: "newer", TaskID: "task", SessionID: "session", TurnID: "turn", WorkflowStepID: "step2",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, newer); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent(newer): %v", err)
	}

	resolved, err := repo.GetCompletionIntentForTurn(ctx, "session", "turn")
	if err != nil {
		t.Fatalf("GetCompletionIntentForTurn: %v", err)
	}
	if resolved.ID != newer.ID {
		t.Fatalf("GetCompletionIntentForTurn = %q, want the most recent intent %q", resolved.ID, newer.ID)
	}
}

// seedCompletionIntentFixture creates the task/session/turn triple a
// completion intent's identity validation resolves against.
func seedCompletionIntentFixture(t *testing.T, repo *Repository, taskID, sessionID, turnID, _ string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Task"}); err != nil {
		t.Fatalf("CreateTask(%s): %v", taskID, err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskID: taskID, TaskSessionID: sessionID, StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn(%s): %v", turnID, err)
	}
}

// TestTransitionCompletionIntentWithControlEventCommitsBothFactsTogether
// covers the atomicity guarantee itself: a successful call must durably
// record both the intent's terminal state and its audit event.
func TestTransitionCompletionIntentWithControlEventCommitsBothFactsTogether(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	seedCompletionIntentFixture(t, repo, "task", "session", "turn", "step")
	intent := &models.CompletionIntent{
		ID: "intent", TaskID: "task", SessionID: "session", TurnID: "turn", WorkflowStepID: "step",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, intent); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}
	if _, err := repo.ClaimCompletionIntentForSettlement(ctx, intent.ID, now, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// The actor identity's foreign keys reference real task/session rows too;
	// reuse the target task/session here since the actor's own identity is
	// not under test in this atomicity check.
	event := &models.SessionControlEvent{
		ActorTaskID: "task", ActorSessionID: "session",
		TargetTaskID: "task", TargetSessionID: "session", TargetTurnID: "turn",
		AuthorityBasis: "same_task_peer", EvidenceCode: "eligible_completion_intent", Result: "settled",
	}
	committed, err := repo.TransitionCompletionIntentWithControlEvent(
		ctx, intent.ID, models.CompletionIntentStateSettling, models.CompletionIntentStateSettled, now, event,
	)
	if err != nil || !committed {
		t.Fatalf("TransitionCompletionIntentWithControlEvent = (%v, %v), want true nil", committed, err)
	}

	stored, err := repo.GetCompletionIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetCompletionIntent: %v", err)
	}
	if stored.State != models.CompletionIntentStateSettled {
		t.Fatalf("state = %q, want settled", stored.State)
	}
	var auditCount int
	if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM session_control_events WHERE target_turn_id = ? AND result = ?`, "turn", "settled").Scan(&auditCount); err != nil {
		t.Fatalf("query session_control_events: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("session_control_events rows = %d, want exactly 1", auditCount)
	}
}

// TestTransitionCompletionIntentWithControlEventRollsBackTogetherOnAuditFailure
// covers the other half of atomicity: if the audit half of the write cannot
// succeed, the intent's state transition must not be left committed either.
// An invalid event fails the repository's own validation before either
// statement executes, but that validation is exactly what stands between
// this method and silently letting a caller commit a settlement with no
// audit content — assert the transition never lands when it does.
func TestTransitionCompletionIntentWithControlEventRollsBackTogetherOnAuditFailure(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	seedCompletionIntentFixture(t, repo, "task", "session", "turn", "step")
	intent := &models.CompletionIntent{
		ID: "intent", TaskID: "task", SessionID: "session", TurnID: "turn", WorkflowStepID: "step",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, intent); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}
	if _, err := repo.ClaimCompletionIntentForSettlement(ctx, intent.ID, now, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Missing Result: the repository requires every audit field non-empty.
	invalidEvent := &models.SessionControlEvent{
		ActorTaskID: "actor-task", ActorSessionID: "actor-session",
		TargetTaskID: "task", TargetSessionID: "session", TargetTurnID: "turn",
		AuthorityBasis: "same_task_peer", EvidenceCode: "eligible_completion_intent",
	}
	if _, err := repo.TransitionCompletionIntentWithControlEvent(
		ctx, intent.ID, models.CompletionIntentStateSettling, models.CompletionIntentStateSettled, now, invalidEvent,
	); err == nil {
		t.Fatal("expected an error for an incomplete audit event")
	}

	stored, err := repo.GetCompletionIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetCompletionIntent: %v", err)
	}
	if stored.State != models.CompletionIntentStateSettling {
		t.Fatalf("state = %q, want still settling — a failed audit write must not leave a half-applied transition", stored.State)
	}
}

// TestCompletionIntentReopenedRecordsTerminalTimestamp covers Reopened, a
// terminal state (CanTransitionTo has no outgoing case for it) that must get
// the same durable settled_at stamp as Settled/Superseded/Rejected so audit
// and terminal-state reporting can distinguish a reopened intent from one
// still pending indefinitely.
func TestCompletionIntentReopenedRecordsTerminalTimestamp(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task"}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn", TaskID: "task", TaskSessionID: "session", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}

	intent := &models.CompletionIntent{
		ID: "intent-reopen", TaskID: "task", SessionID: "session", TurnID: "turn", WorkflowStepID: "step",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, intent); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}

	reopened, err := repo.TransitionCompletionIntent(ctx, intent.ID, models.CompletionIntentStatePending, models.CompletionIntentStateReopened, now)
	if err != nil || !reopened {
		t.Fatalf("TransitionCompletionIntent pending->reopened = (%v, %v)", reopened, err)
	}

	stored, err := repo.GetCompletionIntentForTurn(ctx, intent.SessionID, intent.TurnID)
	if err != nil {
		t.Fatalf("GetCompletionIntentForTurn: %v", err)
	}
	if stored.State != models.CompletionIntentStateReopened {
		t.Fatalf("stored state = %q, want reopened", stored.State)
	}
	if stored.SettledAt == nil {
		t.Fatal("reopened intent has no settled_at; terminal state is missing its audit timestamp")
	}
}

func TestCompletionIntentListDueOrdersAndLimitsPendingIntents(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, candidate := range []struct {
		id       string
		eligible time.Time
		state    models.CompletionIntentState
	}{
		{id: "first", eligible: now.Add(-2 * time.Minute), state: models.CompletionIntentStatePending},
		{id: "second", eligible: now.Add(-time.Minute), state: models.CompletionIntentStatePending},
		{id: "future", eligible: now.Add(time.Minute), state: models.CompletionIntentStatePending},
		{id: "settled", eligible: now.Add(-3 * time.Minute), state: models.CompletionIntentStateSettled},
	} {
		taskID := "task-" + candidate.id
		sessionID := "session-" + candidate.id
		turnID := "turn-" + candidate.id
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Task"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", candidate.id, err)
		}
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", candidate.id, err)
		}
		if err := repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskID: taskID, TaskSessionID: sessionID, StartedAt: now}); err != nil {
			t.Fatalf("CreateTurn(%s): %v", candidate.id, err)
		}
		// CreateOrGetCompletionIntent only admits a new intent in the pending
		// state; seed a non-pending row (the "settled" exclusion case) by
		// creating it pending and then transitioning it, matching how
		// production code actually reaches a terminal state.
		_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
			ID: candidate.id, TaskID: taskID, SessionID: sessionID, TurnID: turnID, WorkflowStepID: "step",
			State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: candidate.eligible,
		})
		if err != nil {
			t.Fatalf("CreateOrGetCompletionIntent(%s): %v", candidate.id, err)
		}
		if candidate.state != models.CompletionIntentStatePending {
			if _, err := repo.TransitionCompletionIntent(ctx, candidate.id, models.CompletionIntentStatePending, models.CompletionIntentStateSettling, now); err != nil {
				t.Fatalf("claim %s for settlement: %v", candidate.id, err)
			}
			if _, err := repo.TransitionCompletionIntent(ctx, candidate.id, models.CompletionIntentStateSettling, candidate.state, now); err != nil {
				t.Fatalf("transition %s to %s: %v", candidate.id, candidate.state, err)
			}
		}
	}

	due, err := repo.ListDueCompletionIntents(ctx, now, 1)
	if err != nil {
		t.Fatalf("ListDueCompletionIntents: %v", err)
	}
	if len(due) != 1 || due[0].ID != "first" {
		t.Fatalf("ListDueCompletionIntents = %+v, want only first", due)
	}
}

func TestCompletionIntentRearmDefersOnlyPendingIntent(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task"}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn", TaskID: "task", TaskSessionID: "session", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent", TaskID: "task", SessionID: "session", TurnID: "turn", WorkflowStepID: "step",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}

	activityAt := now.Add(time.Second)
	rearmed, err := repo.RearmCompletionIntent(ctx, "intent", activityAt, activityAt.Add(time.Minute))
	if err != nil || !rearmed {
		t.Fatalf("RearmCompletionIntent = (%v, %v), want true nil", rearmed, err)
	}
	due, err := repo.ListDueCompletionIntents(ctx, activityAt, 10)
	if err != nil {
		t.Fatalf("ListDueCompletionIntents: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("ListDueCompletionIntents after activity = %+v, want no due intents", due)
	}
}
