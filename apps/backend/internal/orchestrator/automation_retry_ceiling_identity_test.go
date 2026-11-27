package orchestrator

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestRetryCeilingAdmissionComparesImmutableLaunchRatherThanClaim(t *testing.T) {
	original := &automationRunLaunch{RunID: "run", ThreadAction: automation.ThreadActionCreated, ThreadReason: "original", Launch: &automation.RetryLaunch{
		RunID: "run", AutomationID: "automation", Generation: 7, TaskID: "task", SessionID: "session", TurnID: "turn", SubmissionID: "submission",
		Token: "old-claim", State: automation.RetryLaunchPreparing, UpdatedAt: time.Now().UTC(),
	}}
	encoded, err := json.Marshal(original)
	require.NoError(t, err)
	var legacy map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &legacy))
	stored := models.CeilingDeferral{Kind: models.CeilingLaunchStart, Payload: map[string]interface{}{ceilingPayloadAutomationRunKey: legacy, metaKeyPrompt: "prompt"}}
	replayed := *original
	claim := *original.Launch
	claim.Token, claim.State, claim.ExecutionID = "fresh-claim", automation.RetryLaunchQueued, "execution"
	claim.CancelRequested = true
	claim.UpdatedAt = claim.UpdatedAt.Add(time.Hour)
	replayed.Launch = &claim
	incoming := models.CeilingDeferral{Kind: stored.Kind, Payload: map[string]interface{}{ceilingPayloadAutomationRunKey: &replayed, metaKeyPrompt: "prompt"}}
	equal, err := ceilingDeferralsEquivalentForAdmission(stored, incoming)
	require.NoError(t, err)
	require.True(t, equal, "comparison is not permission to launch; the durable claim still gates dispatch")
	require.Equal(t, "old-claim", original.Launch.Token)
	require.Equal(t, "fresh-claim", replayed.Launch.Token)
	require.Equal(t, "old-claim", legacy["launch"].(map[string]interface{})["token"], "comparison must preserve stored input")
	for _, field := range []string{"run", "automation", "generation", "task", "session", "turn", "submission"} {
		changed := *original.Launch
		switch field {
		case "run":
			changed.RunID = "other"
		case "automation":
			changed.AutomationID = "other"
		case "generation":
			changed.Generation++
		case "task":
			changed.TaskID = "other"
		case "session":
			changed.SessionID = "other"
		case "turn":
			changed.TurnID = "other"
		case "submission":
			changed.SubmissionID = "other"
		}
		replayed.Launch = &changed
		equal, err = ceilingDeferralsEquivalentForAdmission(stored, incoming)
		require.NoError(t, err)
		require.False(t, equal, field)
	}
	replayed.Launch = original.Launch
	replayed.ThreadReason = "changed"
	equal, err = ceilingDeferralsEquivalentForAdmission(stored, incoming)
	require.NoError(t, err)
	require.False(t, equal, "the original thread disposition remains immutable")
}
