package automation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const reviewRequestedEvents = `,"events":["review_requested"]`

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.1
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.4
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.4
func TestReviewRequestedFiresOnNewAndReRequest(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	trigger := f.addTrigger(t, reviewRequestedEvents)
	f.evaluate()
	require.Empty(t, f.runs(t), "baseline never fires")

	f.setPRs(followUpPR{Number: 17, Head: "aaa", Reviewers: []string{"kandev-bot"}})
	f.evaluate()
	f.evaluate()
	runs := f.runs(t)
	require.Len(t, runs, 1)
	require.Equal(t, "pr:acme/widget#17/review-requested/1", runs[0].DedupKey)
	data := runData(t, runs[0])
	require.Equal(t, PREventReviewRequested, data["event"])
	require.Equal(t, "kandev-bot", data["requested_reviewers"])
	require.Equal(t, "aaa", data["previous_head_sha"])

	// Submitting a review removes the request; a re-request adds it back.
	f.setPRs(followUpPR{Number: 17, Head: "aaa"})
	f.evaluate()
	require.Len(t, f.runs(t), 1, "a removal does not fire")
	require.Empty(t, f.observation(t, trigger, 17).Requested)

	f.setPRs(followUpPR{Number: 17, Head: "aaa", Reviewers: []string{"kandev-bot"}})
	f.evaluate()
	runs = f.runs(t)
	require.Len(t, runs, 2)
	require.Equal(t, "pr:acme/widget#17/review-requested/2", runs[1].DedupKey)
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.2
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.3
func TestReviewRequestedMatchesFilterOncePerPoll(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	f.addTrigger(t, reviewRequestedEvents+`,"reviewers":["Kandev-Bot","ACME/Reviewers"]`)
	f.evaluate()

	f.setPRs(followUpPR{Number: 17, Head: "aaa", Reviewers: []string{"someone-else"}})
	f.evaluate()
	require.Empty(t, f.runs(t), "an unmatched reviewer does not fire")

	f.setPRs(followUpPR{Number: 17, Head: "aaa",
		Reviewers: []string{"someone-else", "kandev-bot"}, Teams: []string{"reviewers"}})
	f.evaluate()
	runs := f.runs(t)
	require.Len(t, runs, 1, "several reviewers in one poll fire once")
	require.Equal(t, "acme/reviewers, kandev-bot", runData(t, runs[0])["requested_reviewers"])
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.7
func TestNewCommitsTakesPrecedenceOverReviewRequested(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	trigger := f.addTrigger(t, `,"events":["new_commits","review_requested"]`)
	f.evaluate()

	f.setPRs(followUpPR{Number: 17, Head: "bbb", Reviewers: []string{"kandev-bot"}})
	f.evaluate()
	f.evaluate()

	runs := f.runs(t)
	require.Len(t, runs, 1, "one firing per PR per poll, and the covered request is processed")
	require.Equal(t, PREventNewCommits, runData(t, runs[0])["event"])
	obs := f.observation(t, trigger, 17)
	require.Equal(t, []string{"kandev-bot"}, obs.Requested)
	require.Equal(t, "bbb", obs.HeadSHA)
}

func TestReviewRequestedWithPendingQuietPeriod(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	f.addTrigger(t, `,"events":["new_commits","review_requested"],"quiet_period_minutes":30`)
	f.evaluate()

	f.setPRs(followUpPR{Number: 17, Head: "bbb", Reviewers: []string{"kandev-bot"}})
	f.evaluate()

	runs := f.runs(t)
	require.Len(t, runs, 1, "an unsettled head does not block a review request")
	require.Equal(t, PREventReviewRequested, runData(t, runs[0])["event"])
	require.Equal(t, "bbb", runData(t, runs[0])["head_sha"])

	f.clock = f.clock.Add(31 * time.Minute)
	f.evaluate()
	require.Len(t, f.runs(t), 1, "the review firing already covered the new head")
}

// A reopened PR, or a recreated trigger, starts from a new baseline; its next
// review request must not reuse a dedup key an earlier request already holds.
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.4
func TestReviewRequestedAfterReopenGetsFreshDedupKey(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	trigger := f.addTrigger(t, reviewRequestedEvents)
	f.evaluate()
	f.setPRs(followUpPR{Number: 17, Head: "aaa", Reviewers: []string{"kandev-bot"}})
	f.evaluate()
	require.Len(t, f.runs(t), 1)

	f.setPRs()
	f.evaluate()
	require.Nil(t, f.observation(t, trigger, 17), "closed PR observation is pruned")

	f.setPRs(followUpPR{Number: 17, Head: "aaa"})
	f.evaluate()
	f.setPRs(followUpPR{Number: 17, Head: "aaa", Reviewers: []string{"kandev-bot"}})
	f.evaluate()

	runs := f.runs(t)
	require.Len(t, runs, 2, "the request after reopening fires")
	require.Equal(t, "pr:acme/widget#17/review-requested/2", runs[1].DedupKey)
}

// Two triggers with different reviewer filters share the automation's dedup
// scope; the second request must not collide with the first trigger's key.
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.4
func TestReviewRequestedTwoTriggersDoNotCollide(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	f.addTrigger(t, reviewRequestedEvents+`,"reviewers":["alice-reviewer"]`)
	f.addTrigger(t, reviewRequestedEvents+`,"reviewers":["bob-reviewer"]`)
	f.evaluate()

	f.setPRs(followUpPR{Number: 17, Head: "aaa", Reviewers: []string{"alice-reviewer"}})
	f.evaluate()
	f.setPRs(followUpPR{Number: 17, Head: "aaa", Reviewers: []string{"alice-reviewer", "bob-reviewer"}})
	f.evaluate()

	runs := f.runs(t)
	require.Len(t, runs, 2)
	require.Equal(t, "pr:acme/widget#17/review-requested/1", runs[0].DedupKey)
	require.Equal(t, "pr:acme/widget#17/review-requested/2", runs[1].DedupKey)
	require.Equal(t, "bob-reviewer", runData(t, runs[1])["requested_reviewers"])
}
