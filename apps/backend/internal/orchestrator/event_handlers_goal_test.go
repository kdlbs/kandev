package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
)

func testACPGoal(status string, createdAt, updatedAt float64) map[string]any {
	return map[string]any{
		"objective": "Coordinate contributor PR reviews",
		"status":    status,
		"createdAt": createdAt,
		"updatedAt": updatedAt,
	}
}

func TestParseACPGoalAcceptsEverySupportedStatus(t *testing.T) {
	for _, status := range []string{"active", "paused", "blocked", "limited", "complete"} {
		t.Run(status, func(t *testing.T) {
			parsed, ok := parseACPGoal(testACPGoal(status, 10, 20))
			require.True(t, ok)
			require.Equal(t, status, parsed["status"])
		})
	}
}

func TestParseACPGoalRejectsMalformedSnapshots(t *testing.T) {
	cases := []map[string]any{
		{"objective": "missing status", "createdAt": float64(10), "updatedAt": float64(20)},
		{"objective": "unknown status", "status": "running", "createdAt": float64(10), "updatedAt": float64(20)},
		{"objective": "missing created", "status": "active", "updatedAt": float64(20)},
		{"objective": "bad updated", "status": "active", "createdAt": float64(10), "updatedAt": "20"},
		{"objective": "bad optional", "status": "active", "createdAt": float64(10), "updatedAt": float64(20), "tokensUsed": "many"},
	}
	for index, value := range cases {
		t.Run("case", func(t *testing.T) {
			_, ok := parseACPGoal(value)
			require.False(t, ok, "case %d must be rejected", index)
		})
	}
}

func TestMergeACPGoalMetaRejectsOlderSnapshotsAndAllowsNewGoals(t *testing.T) {
	existing := map[string]any{"goal": testACPGoal("complete", 100, 200), "keep": true}
	older := map[string]any{"goal": testACPGoal("active", 100, 199)}
	equal := map[string]any{"goal": testACPGoal("complete", 100, 200)}
	newer := map[string]any{"goal": testACPGoal("active", 100, 201)}

	require.Equal(t, "complete", mergeACPGoalMeta(existing, older, false)[goalKey].(map[string]any)["status"])
	require.Equal(t, "complete", mergeACPGoalMeta(existing, equal, false)[goalKey].(map[string]any)["status"])
	require.Equal(t, "active", mergeACPGoalMeta(existing, newer, false)[goalKey].(map[string]any)["status"])

	newGoal := testACPGoal("active", 101, 1)
	merged := mergeACPGoalMeta(existing, map[string]any{"goal": newGoal}, false)
	require.Equal(t, newGoal, merged[goalKey])
}

func TestMergeACPGoalMetaPreservesOrderedClear(t *testing.T) {
	active := map[string]any{"goal": testACPGoal("active", 100, 200)}
	cleared := mergeACPGoalMeta(active, map[string]any{"goal": nil}, false)
	require.Contains(t, cleared, goalKey)
	require.Nil(t, cleared[goalKey])

	retained := mergeACPGoalMeta(cleared, map[string]any{"codex": map[string]any{"threadStatus": "idle"}}, false)
	require.Contains(t, retained, goalKey)
	require.Nil(t, retained[goalKey])
}

func TestMergeACPGoalMetaResetsOnAttachmentChange(t *testing.T) {
	active := map[string]any{"goal": testACPGoal("active", 100, 200)}
	cleared := mergeACPGoalMeta(active, map[string]any{"codex": "new-attachment"}, true)
	require.Nil(t, cleared[goalKey])

	newGoal := testACPGoal("active", 1, 2)
	attached := mergeACPGoalMeta(active, map[string]any{"goal": newGoal}, true)
	require.Equal(t, newGoal, attached[goalKey])
}

func TestHandleSessionInfoEvent_GoalSurvivesUnrelatedMetadata(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	goal := map[string]any{
		"objective": "Coordinate contributor PR reviews",
		"status":    "active",
		"createdAt": float64(1789079689000),
		"updatedAt": float64(1789287777000),
	}
	svc.handleSessionInfoEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		Data: &lifecycle.AgentStreamEventData{
			ACPSessionID: "acp-1",
			SessionMeta: map[string]any{
				"goal": goal,
			},
		},
	})

	svc.handleSessionInfoEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		Data: &lifecycle.AgentStreamEventData{
			SessionMeta: map[string]any{
				"codex": map[string]any{"threadStatus": "idle"},
			},
		},
	})

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	acp, ok := updated.Metadata["acp"].(map[string]interface{})
	require.True(t, ok)
	meta, ok := acp["meta"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, goal, meta["goal"])

	require.Len(t, eb.events, 2)
	require.Equal(t, events.BuildSessionInfoSubject("s1"), eb.events[1].subject)
	payload, ok := eb.events[1].event.Data.(lifecycle.SessionInfoEventPayload)
	require.True(t, ok)
	require.Equal(t, goal, payload.SessionMeta["goal"])
}
