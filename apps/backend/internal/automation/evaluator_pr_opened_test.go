package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

const openedEvents = `,"events":["opened"]`

func openedRunKeys(t *testing.T, f *prFollowUpFixture) []string {
	t.Helper()
	var keys []string
	for _, run := range f.runs(t) {
		keys = append(keys, run.DedupKey)
	}
	return keys
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.10
func TestOpenedSkipsPullRequestsOpenBeforeTheTrigger(t *testing.T) {
	now := time.Now().UTC()
	f := newPRFollowUpFixture(t,
		followUpPR{Number: 1, Head: "a1", CreatedAt: now.Add(-time.Hour)},
		followUpPR{Number: 2, Head: "b1"},
	)
	f.addTrigger(t, openedEvents)

	f.evaluate()
	require.Equal(t, []string{"pr:acme/widget#2"}, openedRunKeys(t, f),
		"a PR created before the trigger is skipped; an unknown creation time counts as new")

	f.setPRs(
		followUpPR{Number: 1, Head: "a1", CreatedAt: now.Add(-time.Hour)},
		followUpPR{Number: 2, Head: "b1"},
		followUpPR{Number: 3, Head: "c1", CreatedAt: now.Add(time.Minute)},
	)
	f.evaluate()
	f.evaluate()
	require.Equal(t, []string{"pr:acme/widget#2", "pr:acme/widget#3"}, openedRunKeys(t, f))
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.10
func TestOpenedIncludeExistingPullRequestsBackfills(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 1, Head: "a1", CreatedAt: time.Now().Add(-48 * time.Hour)})
	f.addTrigger(t, `,"events":["opened"],"include_existing_prs":true`)

	f.evaluate()
	require.Equal(t, []string{"pr:acme/widget#1"}, openedRunKeys(t, f))
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.10
func TestOpenedAddedLaterSkipsPullRequestsAlreadyObserved(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 5, Head: "e1"})
	trigger := f.addTrigger(t, newCommitsEvents)
	f.evaluate()
	require.Empty(t, f.runs(t))

	enabled := true
	config := json.RawMessage(`{` + eventSelectionRepo + `,"events":["opened","new_commits"]}`)
	require.NoError(t, f.svc.UpdateTrigger(context.Background(), trigger.ID, &UpdateTriggerRequest{
		Enabled: &enabled, Config: &config,
	}))
	f.evaluate()
	require.Empty(t, f.runs(t), "PR #5 was open before opened was selected")
	require.False(t, f.observation(t, trigger, 5).tracks(PREventOpened), "opened stays untracked for a PR open before it was selected")

	f.setPRs(followUpPR{Number: 5, Head: "e1"}, followUpPR{Number: 6, Head: "f1"})
	f.evaluate()
	require.Equal(t, []string{"pr:acme/widget#6"}, openedRunKeys(t, f))
}

// Replacing every follow-up event with opened alone still honors the PRs the
// trigger observed before, rather than backfilling them.
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.10
func TestOpenedOnlyAfterFollowUpEventsSkipsPullRequestsAlreadyObserved(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 5, Head: "e1"})
	trigger := f.addTrigger(t, newCommitsEvents)
	f.evaluate()
	require.NotNil(t, f.observation(t, trigger, 5))

	enabled := true
	config := json.RawMessage(`{` + eventSelectionRepo + openedEvents + `}`)
	require.NoError(t, f.svc.UpdateTrigger(context.Background(), trigger.ID, &UpdateTriggerRequest{
		Enabled: &enabled, Config: &config,
	}))
	f.evaluate()
	require.Empty(t, f.runs(t), "PR #5 was observed before opened was selected")

	f.setPRs(followUpPR{Number: 5, Head: "e1"}, followUpPR{Number: 6, Head: "f1"})
	f.evaluate()
	require.Equal(t, []string{"pr:acme/widget#6"}, openedRunKeys(t, f))
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.10
func TestOpenedDoesNotResubmitAHeldKey(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 7, Head: "g1"})
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	f.svc.logger = log
	f.addTrigger(t, openedEvents)

	f.evaluate()
	f.evaluate()
	f.evaluate()
	require.Equal(t, []string{"pr:acme/widget#7"}, openedRunKeys(t, f))
	require.Zero(t, logs.FilterMessage("failed to record duplicate-skipped run").Len(),
		"steady-state polling must not resubmit the held opened key")
}
