package coordinator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestCoordinator(t *testing.T, store *Store, workspaceID string) *Coordinator {
	t.Helper()
	c := &Coordinator{WorkspaceID: workspaceID, Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := store.CreateCoordinator(context.Background(), c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	return c
}

func sampleSpec() ProposalSpec {
	return ProposalSpec{
		Title:        "Do the thing",
		Description:  "A description",
		Rationale:    "Because",
		WorkflowID:   "wf-1",
		StepID:       "step-1",
		RepositoryID: "repo-1",
		SourceTaskID: "task-0",
	}
}

func TestInsertProposal_AssignsIDAndPendingStatus(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")

	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, p, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	if p.ID == "" {
		t.Fatal("InsertProposal did not assign an id")
	}
	if p.Status != ProposalStatusPending {
		t.Fatalf("p.Status = %q, want %q", p.Status, ProposalStatusPending)
	}
	if p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
		t.Fatal("InsertProposal did not stamp timestamps")
	}

	got, err := store.GetProposal(ctx, "ws-1", c.ID, p.ID, false)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.Spec != sampleSpec() {
		t.Fatalf("GetProposal.Spec = %+v, want %+v", got.Spec, sampleSpec())
	}
	if got.FinalSpec != nil {
		t.Fatalf("GetProposal.FinalSpec = %+v, want nil", got.FinalSpec)
	}
}

func TestInsertProposal_UnknownCoordinatorIsNotFound(t *testing.T) {
	store := newTestStore(t)
	p := &Proposal{CoordinatorID: "missing", WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(context.Background(), p, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("InsertProposal(missing coordinator): err = %v, want ErrNotFound", err)
	}
}

func TestInsertProposal_WrongWorkspaceIsNotFound(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-2", Spec: sampleSpec()}
	if err := store.InsertProposal(context.Background(), p, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("InsertProposal(wrong workspace): err = %v, want ErrNotFound", err)
	}
}

func TestInsertProposal_CapReachedAt25(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")

	for i := 0; i < maxOpenProposals; i++ {
		p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
		if err := store.InsertProposal(ctx, p, false); err != nil {
			t.Fatalf("InsertProposal #%d: %v", i, err)
		}
	}

	overflow := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, overflow, false); !errors.Is(err, ErrCoordinatorProposalCapReached) {
		t.Fatalf("InsertProposal #%d: err = %v, want ErrCoordinatorProposalCapReached", maxOpenProposals, err)
	}

	count, err := store.CountOpenProposals(ctx, c.ID, false)
	if err != nil {
		t.Fatalf("CountOpenProposals: %v", err)
	}
	if count != maxOpenProposals {
		t.Fatalf("CountOpenProposals = %d, want %d", count, maxOpenProposals)
	}
}

// TestInsertProposal_ConcurrentCapEnforcement is Build decision 11's mandated
// test: 30 concurrent proposes against a coordinator with 0 open proposals
// must produce exactly 25 rows and 5 refusals.
// TestCountOpenProposalsByWorkspace_GroupsByCoordinatorAndExcludesClosed is
// the batch counterpart to CountOpenProposals: ListCoordinators uses it to
// fetch every coordinator's open count in one query instead of one query per
// coordinator. It must group by coordinator, count only the open statuses,
// scope to the given workspace, and omit a coordinator with no open
// proposals from the returned map (the caller reads a missing key as its Go
// zero value, 0).
func TestCountOpenProposalsByWorkspace_GroupsByCoordinatorAndExcludesClosed(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	busy := newTestCoordinator(t, store, "ws-1")
	idle := newTestCoordinator(t, store, "ws-1")
	other := newTestCoordinator(t, store, "ws-2")

	// busy: two open (pending), one closed (rejected) -> count 2, closed excluded.
	if err := store.InsertProposal(ctx, &Proposal{CoordinatorID: busy.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	if err := store.InsertProposal(ctx, &Proposal{CoordinatorID: busy.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	closed := &Proposal{CoordinatorID: busy.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, closed); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	if matched, err := store.RejectProposal(ctx, closed.ID, "", "", time.Now().UTC()); err != nil || !matched {
		t.Fatalf("RejectProposal: matched=%v err=%v", matched, err)
	}

	// idle: no proposals at all -> absent from the map.

	// other: a different workspace's open proposal must not leak into ws-1's counts.
	if err := store.InsertProposal(ctx, &Proposal{CoordinatorID: other.ID, WorkspaceID: "ws-2", Spec: sampleSpec()}, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}

	counts, err := store.CountOpenProposalsByWorkspace(ctx, "ws-1", false)
	if err != nil {
		t.Fatalf("CountOpenProposalsByWorkspace: %v", err)
	}
	if counts[busy.ID] != 2 {
		t.Errorf("counts[busy] = %d, want 2", counts[busy.ID])
	}
	if _, ok := counts[idle.ID]; ok {
		t.Errorf("counts[idle] = %d, want absent", counts[idle.ID])
	}
	if _, ok := counts[other.ID]; ok {
		t.Errorf("ws-2's coordinator leaked into ws-1's counts: %d", counts[other.ID])
	}
	if len(counts) != 1 {
		t.Errorf("len(counts) = %d, want 1", len(counts))
	}
}

func TestInsertProposal_ConcurrentCapEnforcement(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")

	const attempts = 30
	start := make(chan struct{})
	var wg sync.WaitGroup
	var succeeded, capped, other int64
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			<-start
			p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
			err := store.InsertProposal(ctx, p, false)
			switch {
			case err == nil:
				atomic.AddInt64(&succeeded, 1)
			case errors.Is(err, ErrCoordinatorProposalCapReached):
				atomic.AddInt64(&capped, 1)
			default:
				atomic.AddInt64(&other, 1)
				t.Errorf("InsertProposal: unexpected error %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if other != 0 {
		t.Fatalf("unexpected errors: %d", other)
	}
	if succeeded != maxOpenProposals {
		t.Fatalf("succeeded = %d, want %d", succeeded, maxOpenProposals)
	}
	if capped != attempts-maxOpenProposals {
		t.Fatalf("capped = %d, want %d", capped, attempts-maxOpenProposals)
	}

	count, err := store.CountOpenProposals(ctx, c.ID, false)
	if err != nil {
		t.Fatalf("CountOpenProposals: %v", err)
	}
	if count != maxOpenProposals {
		t.Fatalf("CountOpenProposals = %d, want %d", count, maxOpenProposals)
	}
}

func TestClaimProposal_ClaimsPendingNotOthers(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, p, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}

	now := time.Now().UTC()
	final := sampleSpec()
	final.Title = "Edited title"
	matched, err := store.ClaimProposal(ctx, p.ID, "token-1", final, "user-1", now)
	if err != nil {
		t.Fatalf("ClaimProposal: %v", err)
	}
	if !matched {
		t.Fatal("ClaimProposal did not match the pending row")
	}

	got, err := store.GetProposal(ctx, "ws-1", c.ID, p.ID, false)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.Status != ProposalStatusApproving {
		t.Fatalf("Status = %q, want %q", got.Status, ProposalStatusApproving)
	}
	if got.ClaimToken == nil || *got.ClaimToken != "token-1" {
		t.Fatalf("ClaimToken = %v, want %q", got.ClaimToken, "token-1")
	}
	if got.FinalSpec == nil || *got.FinalSpec != final {
		t.Fatalf("FinalSpec = %+v, want %+v", got.FinalSpec, final)
	}
	if got.DecidedBy == nil || *got.DecidedBy != "user-1" {
		t.Fatalf("DecidedBy = %v, want %q", got.DecidedBy, "user-1")
	}

	// Claiming again (now approving) must not match.
	matched, err = store.ClaimProposal(ctx, p.ID, "token-2", final, "user-2", now)
	if err != nil {
		t.Fatalf("ClaimProposal (already approving): %v", err)
	}
	if matched {
		t.Fatal("ClaimProposal matched an already-approving row")
	}
}

func TestClaimProposal_NoMatchReturnsFalseNotError(t *testing.T) {
	store := newTestStore(t)
	matched, err := store.ClaimProposal(context.Background(), "missing", "token", sampleSpec(), "user-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimProposal(missing): %v", err)
	}
	if matched {
		t.Fatal("ClaimProposal matched a nonexistent row")
	}
}

func TestReclaimStale_OnlyWhenClaimedBeforeThreshold(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, p, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	claimedAt := time.Now().UTC().Add(-3 * time.Minute)
	if _, err := store.ClaimProposal(ctx, p.ID, "token-1", sampleSpec(), "user-1", claimedAt); err != nil {
		t.Fatalf("ClaimProposal: %v", err)
	}

	staleBefore := time.Now().UTC().Add(-2 * time.Minute)
	now := time.Now().UTC()

	// Not yet stale relative to a threshold before the original claim.
	tooRecentThreshold := claimedAt.Add(-time.Minute)
	matched, err := store.ReclaimStale(ctx, p.ID, "token-2", now, tooRecentThreshold, false)
	if err != nil {
		t.Fatalf("ReclaimStale (not stale): %v", err)
	}
	if matched {
		t.Fatal("ReclaimStale matched a claim that is not stale relative to the threshold")
	}

	matched, err = store.ReclaimStale(ctx, p.ID, "token-2", now, staleBefore, false)
	if err != nil {
		t.Fatalf("ReclaimStale (stale): %v", err)
	}
	if !matched {
		t.Fatal("ReclaimStale did not match a stale claim")
	}

	got, err := store.GetProposal(ctx, "ws-1", c.ID, p.ID, false)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.ClaimToken == nil || *got.ClaimToken != "token-2" {
		t.Fatalf("ClaimToken = %v, want %q", got.ClaimToken, "token-2")
	}
	if got.ClaimedAt == nil || !got.ClaimedAt.Equal(now) {
		t.Fatalf("ClaimedAt = %v, want %v", got.ClaimedAt, now)
	}
	if got.DecidedBy == nil || *got.DecidedBy != "user-1" {
		t.Fatalf("DecidedBy = %v, want unchanged %q (stale re-claim never rewrites it)", got.DecidedBy, "user-1")
	}
}

// TestClaimProposal_NonUTCClaimedAtStillOrdersCorrectly proves ClaimProposal
// normalizes claimed_at to UTC before binding, so a genuinely-stale claim is
// still found by ListApprovingClaimedBefore's cutoff comparison even when the
// caller's time.Time carries a positive UTC offset
// (docs/specs/coordinator/system-design/proposals.md#recovery). SQLite
// compares DATETIME columns as TEXT: without normalization, a claim written
// as "16:xx+08:00" (a UTC instant of 08:xx) sorts as greater than a UTC
// cutoff of "08:yy+00:00" even when yy > xx, because the digits are compared
// as text, not as instants.
func TestClaimProposal_NonUTCClaimedAtStillOrdersCorrectly(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")

	cutoff := time.Now().UTC()
	staleInstant := cutoff.Add(-10 * time.Minute)
	positiveOffsetZone := time.FixedZone("test+08:00", 8*60*60)
	staleClaimedAt := staleInstant.In(positiveOffsetZone)

	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, p, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	if _, err := store.ClaimProposal(ctx, p.ID, "tok", sampleSpec(), "user-1", staleClaimedAt); err != nil {
		t.Fatalf("ClaimProposal: %v", err)
	}

	got, err := store.ListApprovingClaimedBefore(ctx, cutoff, false)
	if err != nil {
		t.Fatalf("ListApprovingClaimedBefore: %v", err)
	}
	if len(got) != 1 || got[0].ID != p.ID {
		t.Fatalf("ListApprovingClaimedBefore returned %+v, want [%s] (the stale row, correctly ordered despite its non-UTC claimedAt)", got, p.ID)
	}
	if got[0].ClaimedAt == nil || !got[0].ClaimedAt.Equal(staleInstant) {
		t.Fatalf("ClaimedAt = %v, want the same instant as %v", got[0].ClaimedAt, staleInstant)
	}
}

// TestReclaimStale_NonUTCStaleBeforeStillMatches proves ReclaimStale
// normalizes staleBefore to UTC before binding, so a genuinely-stale claim
// (correctly stored in UTC by ClaimProposal) is still recognized as stale
// when the caller's staleBefore carries a negative UTC offset — the
// direction that makes the cutoff's wall-clock digits look earlier than they
// truly are, which would otherwise make a genuinely-earlier claimed_at
// compare as "not less than" it (docs/specs/coordinator/system-design/
// proposals.md#stale-re-claim).
func TestReclaimStale_NonUTCStaleBeforeStillMatches(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, p, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}

	claimedInstant := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := store.ClaimProposal(ctx, p.ID, "token-1", sampleSpec(), "user-1", claimedInstant); err != nil {
		t.Fatalf("ClaimProposal: %v", err)
	}

	negativeOffsetZone := time.FixedZone("test-05:00", -5*60*60)
	staleBeforeInstant := time.Now().UTC().Add(-2 * time.Minute)
	nowInstant := time.Now().UTC()
	matched, err := store.ReclaimStale(ctx, p.ID, "token-2", nowInstant, staleBeforeInstant.In(negativeOffsetZone), false)
	if err != nil {
		t.Fatalf("ReclaimStale: %v", err)
	}
	if !matched {
		t.Fatal("ReclaimStale did not match a genuinely-stale claim against a staleBefore carrying a non-UTC offset")
	}

	got, err := store.GetProposal(ctx, "ws-1", c.ID, p.ID, false)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.ClaimToken == nil || *got.ClaimToken != "token-2" {
		t.Fatalf("ClaimToken = %v, want %q", got.ClaimToken, "token-2")
	}
	if got.ClaimedAt == nil || !got.ClaimedAt.Equal(nowInstant) {
		t.Fatalf("ClaimedAt = %v, want the same instant as %v", got.ClaimedAt, nowInstant)
	}
}

// TestReclaimStale_NonUTCNowStillOrdersCorrectlyLater proves ReclaimStale
// normalizes now to UTC before writing claimed_at, so a row it re-claims
// remains correctly comparable by a later UTC cutoff (for example the
// startup pass's T0) even when the caller's now carries a positive UTC
// offset — mirroring TestClaimProposal_NonUTCClaimedAtStillOrdersCorrectly
// for the re-claim write path.
func TestReclaimStale_NonUTCNowStillOrdersCorrectlyLater(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, p, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	if _, err := store.ClaimProposal(ctx, p.ID, "token-1", sampleSpec(), "user-1", time.Now().UTC().Add(-10*time.Minute)); err != nil {
		t.Fatalf("ClaimProposal: %v", err)
	}

	positiveOffsetZone := time.FixedZone("test+08:00", 8*60*60)
	reclaimInstant := time.Now().UTC()
	matched, err := store.ReclaimStale(ctx, p.ID, "token-2", reclaimInstant.In(positiveOffsetZone), time.Now().UTC().Add(-2*time.Minute), false)
	if err != nil {
		t.Fatalf("ReclaimStale: %v", err)
	}
	if !matched {
		t.Fatal("ReclaimStale: no row matched")
	}

	laterCutoff := reclaimInstant.Add(time.Minute)
	got, err := store.ListApprovingClaimedBefore(ctx, laterCutoff, false)
	if err != nil {
		t.Fatalf("ListApprovingClaimedBefore: %v", err)
	}
	if len(got) != 1 || got[0].ID != p.ID {
		t.Fatalf("ListApprovingClaimedBefore returned %+v, want [%s] (the re-claimed row, correctly ordered despite ReclaimStale's non-UTC now)", got, p.ID)
	}
}

func TestCompleteProposal_FencedByToken(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, p, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.ClaimProposal(ctx, p.ID, "token-1", sampleSpec(), "user-1", now); err != nil {
		t.Fatalf("ClaimProposal: %v", err)
	}

	matched, err := store.CompleteProposal(ctx, p.ID, "wrong-token", "task-1", now)
	if err != nil {
		t.Fatalf("CompleteProposal (wrong token): %v", err)
	}
	if matched {
		t.Fatal("CompleteProposal matched with the wrong claim token")
	}

	matched, err = store.CompleteProposal(ctx, p.ID, "token-1", "task-1", now)
	if err != nil {
		t.Fatalf("CompleteProposal: %v", err)
	}
	if !matched {
		t.Fatal("CompleteProposal did not match the claimed row")
	}

	got, err := store.GetProposal(ctx, "ws-1", c.ID, p.ID, false)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved {
		t.Fatalf("Status = %q, want %q", got.Status, ProposalStatusApproved)
	}
	if got.TaskID == nil || *got.TaskID != "task-1" {
		t.Fatalf("TaskID = %v, want %q", got.TaskID, "task-1")
	}
	if got.ClaimToken != nil {
		t.Fatalf("ClaimToken = %v, want nil", got.ClaimToken)
	}
}

func TestFailProposal_TruncatesErrorAndFences(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, p, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.ClaimProposal(ctx, p.ID, "token-1", sampleSpec(), "user-1", now); err != nil {
		t.Fatalf("ClaimProposal: %v", err)
	}

	longMsg := make([]rune, 1500)
	for i := range longMsg {
		longMsg[i] = 'x'
	}
	matched, err := store.FailProposal(ctx, p.ID, "token-1", string(longMsg), now)
	if err != nil {
		t.Fatalf("FailProposal: %v", err)
	}
	if !matched {
		t.Fatal("FailProposal did not match the claimed row")
	}

	got, err := store.GetProposal(ctx, "ws-1", c.ID, p.ID, false)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.Status != ProposalStatusFailed {
		t.Fatalf("Status = %q, want %q", got.Status, ProposalStatusFailed)
	}
	if got.Error == nil || len([]rune(*got.Error)) != 1000 {
		t.Fatalf("Error length = %d, want 1000", len([]rune(*got.Error)))
	}
	if got.ClaimToken != nil {
		t.Fatalf("ClaimToken = %v, want nil", got.ClaimToken)
	}

	// A failed proposal can be re-claimed (approve retries from failed).
	matched, err = store.ClaimProposal(ctx, p.ID, "token-2", sampleSpec(), "user-1", now)
	if err != nil {
		t.Fatalf("ClaimProposal (from failed): %v", err)
	}
	if !matched {
		t.Fatal("ClaimProposal did not match a failed row")
	}
}

// TestFailProposal_FencedByToken is CompleteProposal's fencing test
// (TestCompleteProposal_FencedByToken) for FailProposal: a wrong claim token
// is a genuine zero-row CAS race and must return matched=false with the row
// untouched, not an error (task-07 Verification: "genuine zero-row CAS races
// on every mutating store method").
func TestFailProposal_FencedByToken(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, p, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.ClaimProposal(ctx, p.ID, "token-1", sampleSpec(), "user-1", now); err != nil {
		t.Fatalf("ClaimProposal: %v", err)
	}

	matched, err := store.FailProposal(ctx, p.ID, "wrong-token", "boom", now)
	if err != nil {
		t.Fatalf("FailProposal (wrong token): %v", err)
	}
	if matched {
		t.Fatal("FailProposal matched with the wrong claim token")
	}

	got, err := store.GetProposal(ctx, "ws-1", c.ID, p.ID, false)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.Status != ProposalStatusApproving {
		t.Fatalf("Status = %q, want approving (unchanged by the fenced-out call)", got.Status)
	}
	if got.ClaimToken == nil || *got.ClaimToken != "token-1" {
		t.Fatalf("ClaimToken = %v, want unchanged %q", got.ClaimToken, "token-1")
	}
}

func TestRejectProposal_TrimsAndNullsEmptyReason(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, p, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}

	now := time.Now().UTC()
	matched, err := store.RejectProposal(ctx, p.ID, "   ", "user-1", now)
	if err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
	if !matched {
		t.Fatal("RejectProposal did not match the pending row")
	}

	got, err := store.GetProposal(ctx, "ws-1", c.ID, p.ID, false)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.Status != ProposalStatusRejected {
		t.Fatalf("Status = %q, want %q", got.Status, ProposalStatusRejected)
	}
	if got.RejectReason != nil {
		t.Fatalf("RejectReason = %v, want nil for a whitespace-only reason", got.RejectReason)
	}
	if got.DecidedBy == nil || *got.DecidedBy != "user-1" {
		t.Fatalf("DecidedBy = %v, want %q", got.DecidedBy, "user-1")
	}
}

func TestRejectProposal_TruncatesLongReason(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, p, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}

	longReason := make([]rune, 800)
	for i := range longReason {
		longReason[i] = 'r'
	}
	if _, err := store.RejectProposal(ctx, p.ID, string(longReason), "user-1", time.Now().UTC()); err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}

	got, err := store.GetProposal(ctx, "ws-1", c.ID, p.ID, false)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.RejectReason == nil || len([]rune(*got.RejectReason)) != 500 {
		t.Fatalf("RejectReason length = %v, want 500", got.RejectReason)
	}
}

func TestRejectProposal_OnlyFromPendingOrFailed(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, p, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.ClaimProposal(ctx, p.ID, "token-1", sampleSpec(), "user-1", now); err != nil {
		t.Fatalf("ClaimProposal: %v", err)
	}

	matched, err := store.RejectProposal(ctx, p.ID, "no thanks", "user-2", now)
	if err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
	if matched {
		t.Fatal("RejectProposal matched an approving row")
	}
}

func TestListProposals_PendingOrderedAscending(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")

	var ids []string
	for i := 0; i < 3; i++ {
		p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
		if err := store.InsertProposal(ctx, p, false); err != nil {
			t.Fatalf("InsertProposal: %v", err)
		}
		ids = append(ids, p.ID)
	}
	// A rejected proposal must not appear in the pending list.
	rejected := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, rejected, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	if _, err := store.RejectProposal(ctx, rejected.ID, "", "user-1", time.Now().UTC()); err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}

	list, err := store.ListProposals(ctx, "ws-1", c.ID, ListProposalsPending, false)
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("ListProposals returned %d rows, want 3", len(list))
	}
	for i, p := range list {
		if p.ID != ids[i] {
			t.Fatalf("ListProposals[%d].ID = %q, want %q (order mismatch)", i, p.ID, ids[i])
		}
	}
}

func TestListProposals_AllOrderedDescendingLimited(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")

	var ids []string
	for i := 0; i < 3; i++ {
		p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
		if err := store.InsertProposal(ctx, p, false); err != nil {
			t.Fatalf("InsertProposal: %v", err)
		}
		ids = append(ids, p.ID)
	}

	list, err := store.ListProposals(ctx, "ws-1", c.ID, ListProposalsAll, false)
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("ListProposals returned %d rows, want 3", len(list))
	}
	for i, p := range list {
		want := ids[len(ids)-1-i]
		if p.ID != want {
			t.Fatalf("ListProposals[%d].ID = %q, want %q (descending order mismatch)", i, p.ID, want)
		}
	}
}

// TestListProposals_AllRespectsFiftyRowCap verifies the LIMIT 50 clause
// (RV-006): status=all must never return more than 50 rows, newest first,
// even when more than 50 proposals exist. Each proposal is rejected right
// after insertion so the open-proposal count never approaches the unrelated
// 25-cap that InsertProposal enforces.
func TestListProposals_AllRespectsFiftyRowCap(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")

	const total = 55
	ids := make([]string, 0, total)
	for i := 0; i < total; i++ {
		p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
		if err := store.InsertProposal(ctx, p, false); err != nil {
			t.Fatalf("InsertProposal[%d]: %v", i, err)
		}
		ids = append(ids, p.ID)
		if _, err := store.RejectProposal(ctx, p.ID, "", "user-1", time.Now().UTC()); err != nil {
			t.Fatalf("RejectProposal[%d]: %v", i, err)
		}
	}

	list, err := store.ListProposals(ctx, "ws-1", c.ID, ListProposalsAll, false)
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(list) != 50 {
		t.Fatalf("ListProposals(all) returned %d rows, want 50 (the LIMIT 50 cap)", len(list))
	}

	// Newest first: the last 50 inserted ids, most recent first.
	wantNewestFirst := ids[total-50:]
	for i, p := range list {
		want := wantNewestFirst[len(wantNewestFirst)-1-i]
		if p.ID != want {
			t.Fatalf("ListProposals(all)[%d].ID = %q, want %q (newest-first order mismatch)", i, p.ID, want)
		}
	}
}

func TestGetProposal_WrongCoordinatorOrWorkspaceIsNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c1 := newTestCoordinator(t, store, "ws-1")
	c2 := newTestCoordinator(t, store, "ws-1")
	p := &Proposal{CoordinatorID: c1.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, p, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}

	if _, err := store.GetProposal(ctx, "ws-1", c2.ID, p.ID, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProposal(wrong coordinator): err = %v, want ErrNotFound", err)
	}
	if _, err := store.GetProposal(ctx, "ws-2", c1.ID, p.ID, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProposal(wrong workspace): err = %v, want ErrNotFound", err)
	}
}
