package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.8
func TestSeam1StartPayloadPreservesWorkflowBindingPolicy(t *testing.T) {
	for _, preserve := range []bool{true, false} {
		payload := seam1StartPayload(
			"profile-a", "exec-local", "ep1", "", "helper work", "", false, false, nil,
			startTaskOptions{PreserveWorkflowSourceBinding: preserve},
		)
		require.Equal(t, preserve, payload["preserve_workflow_source_binding"])

		encoded, err := json.Marshal(payload)
		require.NoError(t, err)
		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		require.Equal(t, preserve, boolField(decoded, "preserve_workflow_source_binding"),
			"the persisted payload must round-trip the spawn policy")
	}
}

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.4, AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.8
func TestCeilingReplayPreservesWorkflowSourceBinding(t *testing.T) {
	ctx := context.Background()
	fixture := newSpawnBindingFixture(t)

	payload := seam1StartPayload(
		"profile-a", "exec-local", "ep1", "", "prepare the helper work", "", false, false, nil,
		startTaskOptions{
			PreserveWorkflowSourceBinding: true,
			SpawnOrigin:                   &SpawnOrigin{TaskID: "t1", SessionID: fixture.current.ID, SessionName: "source"},
		},
	)
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &decoded))

	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	result := fixture.svc.replayCeilingLaunchStart(ctx, task, decoded)
	require.Equal(t, ceilingReplaySucceeded, result.outcome)
	require.NotEmpty(t, result.sessionID)
	require.NotEqual(t, fixture.current.ID, result.sessionID)

	fixture.requireLaunchPrecondition(t)
	fixture.requireBindingUnchanged(t)
}
