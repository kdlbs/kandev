package lifecycle

import (
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/stretchr/testify/require"
)

// @covers AC-EXECUTORS-IDLE-PARKING-001.11
func TestIdleActivityRestoreValidatesClockAndExecution(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	for _, tc := range []struct {
		name, executionID, clock string
		want                     bool
	}{
		{"valid", "execution", past.Format(time.RFC3339Nano), true},
		{"other execution", "other", past.Format(time.RFC3339Nano), false},
		{"malformed", "execution", "yesterday", false},
		{"zero", "execution", time.Time{}.Format(time.RFC3339Nano), false},
		{"future", "execution", time.Now().Add(time.Hour).Format(time.RFC3339Nano), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &AgentExecution{ID: "execution"}
			e.restoreIdleActivity(map[string]interface{}{idleActivityMetadataKey: map[string]interface{}{
				"execution_id": tc.executionID, "activity_at": tc.clock,
			}})
			at, output, epoch := e.promptActivitySnapshot()
			require.False(t, output, "restored clocks are not fresh agent output")
			if tc.want {
				require.True(t, at.Equal(past))
				require.Equal(t, uint64(1), epoch)
			} else {
				require.True(t, at.IsZero())
				require.Zero(t, epoch)
			}
		})
	}
	for _, metadata := range []map[string]interface{}{nil, {}, {idleActivityMetadataKey: "invalid"}} {
		e := &AgentExecution{ID: "execution"}
		e.restoreIdleActivity(metadata)
		at, _, epoch := e.promptActivitySnapshot()
		require.True(t, at.IsZero())
		require.Zero(t, epoch)
	}
}

// @covers AC-EXECUTORS-IDLE-PARKING-001.11
func TestIdleActivityReplayPreservesNewerLiveObservation(t *testing.T) {
	zero, future := time.Time{}, time.Now().Add(time.Hour)
	older := time.Now().Add(-time.Hour)
	for _, replay := range []struct {
		name string
		at   *time.Time
	}{{"unknown", nil}, {"zero", &zero}, {"future", &future}, {"older", &older}} {
		for _, live := range []struct {
			name    string
			observe func(*AgentExecution)
		}{
			{"agent", (*AgentExecution).markAgentActivity},
			{"prompt", (*AgentExecution).armPromptActivity},
			{"lifecycle readiness", (*AgentExecution).markLifecycleActivity},
			{"content", func(e *AgentExecution) {
				newTestManager(t).recordActivity(e, agentctl.AgentEvent{Type: "message_chunk", Text: "live"})
			}},
			{"steer", func(e *AgentExecution) { (&SessionManager{}).recordSteerActivity(e, "steer") }},
		} {
			t.Run(live.name+"/"+replay.name, func(t *testing.T) {
				e := createTestExecution("execution", "task", "session")
				live.observe(e)
				before, _, epoch := e.promptActivitySnapshot()
				require.False(t, before.IsZero(), "positive control: live activity was observed")
				e.markRecoveredAgentActivity(replay.at)
				after, output, nextEpoch := e.promptActivitySnapshot()
				require.True(t, after.Equal(before), "replay erased or changed newer live activity")
				require.True(t, output)
				require.Greater(t, nextEpoch, epoch)
			})
		}
	}
}

func TestIdleActivityReplayAdvancesRecordedClockAndClearsUnknownHistory(t *testing.T) {
	old := time.Now().UTC().Add(-2 * time.Hour)
	newer := old.Add(time.Hour)
	e := &AgentExecution{ID: "execution"}
	e.restoreIdleActivity(map[string]interface{}{idleActivityMetadataKey: map[string]interface{}{
		"execution_id": e.ID, "activity_at": old.Format(time.RFC3339Nano),
	}})
	e.markRecoveredAgentActivity(&newer)
	e.markRecoveredAgentActivity(&old)
	at, _, _ := e.promptActivitySnapshot()
	require.True(t, at.Equal(newer))
	e.markRecoveredAgentActivity(nil)
	at, _, _ = e.promptActivitySnapshot()
	require.True(t, at.IsZero(), "unknown recovery history must receive a full settled interval")
}

func TestIdleActivityContentReplayCannotEraseLiveContent(t *testing.T) {
	mgr := newTestManager(t)
	e := createTestExecution("execution", "task", "session")
	mgr.recordActivity(e, agentctl.AgentEvent{Type: "message_chunk", Text: "live"})
	before, _, epoch := e.promptActivitySnapshot()
	require.False(t, before.IsZero())
	mgr.recordActivity(e, agentctl.AgentEvent{Type: "message_chunk", Text: "replayed", Recovered: true})
	after, output, nextEpoch := e.promptActivitySnapshot()
	require.True(t, after.Equal(before))
	require.True(t, output)
	require.Greater(t, nextEpoch, epoch)
}
