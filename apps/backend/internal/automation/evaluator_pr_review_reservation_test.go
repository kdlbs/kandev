package automation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// failProcessedObservationWrites makes every observation update that changes
// the requested reviewers fail, as if the process stopped or the write failed
// after a review-request run was committed.
func failProcessedObservationWrites(t *testing.T, f *prFollowUpFixture) func() {
	t.Helper()
	db := f.svc.Store().db
	_, err := db.Exec(`CREATE TRIGGER fail_processed_observation
		BEFORE UPDATE ON automation_pr_observations
		WHEN NEW.requested <> OLD.requested
		BEGIN SELECT RAISE(ABORT, 'injected observation write failure'); END`)
	require.NoError(t, err)
	return func() {
		_, err := db.Exec(`DROP TRIGGER fail_processed_observation`)
		require.NoError(t, err)
	}
}

// A review request whose run was committed but whose observation was never
// advanced reuses its dedup key on the next poll instead of dispatching again.
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.4
func TestReviewRequestedRetryAfterLostObservationDoesNotDispatchTwice(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	trigger := f.addTrigger(t, reviewRequestedEvents)
	f.evaluate()

	restore := failProcessedObservationWrites(t, f)
	f.setPRs(followUpPR{Number: 17, Head: "aaa", Reviewers: []string{"kandev-bot"}})
	f.evaluate()
	f.evaluate()
	runs := f.runs(t)
	require.Len(t, runs, 1, "the retry reuses the committed run's key")
	require.Equal(t, "pr:acme/widget#17/review-requested/1", runs[0].DedupKey)

	restore()
	f.evaluate()
	require.Len(t, f.runs(t), 1)
	obs := f.observation(t, trigger, 17)
	require.Equal(t, []string{"kandev-bot"}, obs.Requested)
	require.EqualValues(t, 1, obs.ReviewGeneration)
	require.Zero(t, obs.PendingReviewGeneration, "processing releases the reservation")

	// The next request after a processed one still gets a fresh key.
	f.setPRs(followUpPR{Number: 17, Head: "aaa"})
	f.evaluate()
	f.setPRs(followUpPR{Number: 17, Head: "aaa", Reviewers: []string{"kandev-bot"}})
	f.evaluate()
	runs = f.runs(t)
	require.Len(t, runs, 2)
	require.Equal(t, "pr:acme/widget#17/review-requested/2", runs[1].DedupKey)
}

// A reservation whose request disappears before dispatch is released, so a
// later request cannot inherit a key that may already be held.
func TestReviewRequestedReservationClearsWhenRequestDisappears(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	trigger := f.addTrigger(t, reviewRequestedEvents)
	f.evaluate()

	restore := failProcessedObservationWrites(t, f)
	f.setPRs(followUpPR{Number: 17, Head: "aaa", Reviewers: []string{"kandev-bot"}})
	f.evaluate()
	restore()
	require.EqualValues(t, 1, f.observation(t, trigger, 17).PendingReviewGeneration)

	f.setPRs(followUpPR{Number: 17, Head: "aaa"})
	f.evaluate()
	require.Zero(t, f.observation(t, trigger, 17).PendingReviewGeneration)

	f.setPRs(followUpPR{Number: 17, Head: "aaa", Reviewers: []string{"kandev-bot"}})
	f.evaluate()
	runs := f.runs(t)
	require.Len(t, runs, 2, "a new request after the reservation was released fires")
	require.Equal(t, "pr:acme/widget#17/review-requested/2", runs[1].DedupKey)
}

// A reviewer requested after a reserved request was committed is not covered
// by that reservation, so it gets its own key and dispatches.
func TestReviewRequestedReservationCoversOnlyItsReviewers(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	f.addTrigger(t, reviewRequestedEvents)
	f.evaluate()

	restore := failProcessedObservationWrites(t, f)
	f.setPRs(followUpPR{Number: 17, Head: "aaa", Reviewers: []string{"alice-reviewer"}})
	f.evaluate()
	restore()

	f.setPRs(followUpPR{Number: 17, Head: "aaa", Reviewers: []string{"alice-reviewer", "bob-reviewer"}})
	f.evaluate()
	runs := f.runs(t)
	require.Len(t, runs, 2, "the later reviewer is not swallowed by the reserved key")
	require.Equal(t, "pr:acme/widget#17/review-requested/1", runs[0].DedupKey)
	require.Equal(t, "pr:acme/widget#17/review-requested/2", runs[1].DedupKey)
}
