package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.2
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.3
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.4
func TestPRPlaceholdersHeadAndEvent(t *testing.T) {
	data := json.RawMessage(`{"head_sha":"bbb","previous_head_sha":"aaa","event":"review_requested",
		"requested_reviewers":"kandev-bot, acme/reviewers"}`)
	got := InterpolatePrompt("{{pr.event}} {{pr.previous_head_sha}}..{{pr.head_sha}} by {{pr.requested_reviewers}}",
		TriggerTypeGitHubPR, data)
	require.Equal(t, "review_requested aaa..bbb by kandev-bot, acme/reviewers", got)

	opened := InterpolatePrompt("[{{pr.previous_head_sha}}][{{pr.requested_reviewers}}]", TriggerTypeGitHubPR,
		json.RawMessage(`{"event":"opened","previous_head_sha":"","requested_reviewers":""}`))
	require.Equal(t, "[][]", opened)
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.5
func TestGitHubPRPlaceholdersIncludeHeadKeys(t *testing.T) {
	keys := map[string]bool{}
	for _, p := range GetTriggerTypeInfo(TriggerTypeGitHubPR).Placeholders {
		keys[p.Key] = true
	}
	for _, key := range []string{"pr.head_sha", "pr.previous_head_sha", "pr.event", "pr.requested_reviewers"} {
		require.True(t, keys[key], "missing placeholder %s", key)
	}
}

func TestPRObservationStoreRoundTripAndTriggerDelete(t *testing.T) {
	svc := newTestService(t)
	a := createTestAutomation(t, svc, "ws-1")
	trigger := addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPR, map[string]any{"events": []string{"new_commits"}})
	ctx := context.Background()
	since := time.Date(2026, 11, 24, 9, 30, 0, 0, time.UTC)
	obs := &PRObservation{TriggerID: trigger.ID, RepoOwner: "acme", RepoName: "widget", PRNumber: 7,
		HeadSHA: "aaa", Requested: []string{"zed", "amy"}, ReviewGeneration: 2,
		PendingHeadSHA: "bbb", PendingSince: &since}
	require.NoError(t, svc.Store().UpsertPRObservation(ctx, obs))

	got, err := svc.Store().ListPRObservations(ctx, trigger.ID, "acme", "widget")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, []string{"amy", "zed"}, got[7].Requested, "requested reviewers are stored sorted")
	require.Equal(t, int64(2), got[7].ReviewGeneration)
	require.Equal(t, "bbb", got[7].PendingHeadSHA)
	require.True(t, since.Equal(*got[7].PendingSince))

	obs.HeadSHA, obs.PendingHeadSHA, obs.PendingSince = "ccc", "", nil
	require.NoError(t, svc.Store().UpsertPRObservation(ctx, obs))
	got, err = svc.Store().ListPRObservations(ctx, trigger.ID, "acme", "widget")
	require.NoError(t, err)
	require.Equal(t, "ccc", got[7].HeadSHA)
	require.Nil(t, got[7].PendingSince)

	require.NoError(t, svc.DeleteTrigger(ctx, trigger.ID))
	got, err = svc.Store().ListPRObservations(ctx, trigger.ID, "acme", "widget")
	require.NoError(t, err)
	require.Empty(t, got, "deleting a trigger clears its observations")
}
