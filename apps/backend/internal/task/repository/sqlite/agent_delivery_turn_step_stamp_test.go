package sqlite

import (
	"encoding/json"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001.4
func TestCanonicalDeliveryTurnWorkflowStamp(t *testing.T) {
	repo := newTurnStepStampTestRepo(t)
	seedTurnStepStampSession(t, repo, "audit-task", "audit-step", "audit-session")
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	id, err := repo.canonicalDeliveryTurnTx(t.Context(), tx, "audit-session", "audit-task", "", &models.AgentDeliveryEvent{StreamID: "audit-stream", Sequence: 1})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	turn, err := repo.GetTurn(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, "audit-step", turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart])
	_, _, entry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "audit-task")
	require.NoError(t, err)
	require.EqualValues(t, entry, turn.Metadata[models.TurnMetaKeyWorkflowStepTransitionIDAtStart])
}

func TestCanonicalDeliveryTurnReuseAndReplay(t *testing.T) {
	for _, kind := range []string{"requested", "open", "closed-conflict", "no-step"} {
		t.Run(kind, func(t *testing.T) {
			repo := newTurnStepStampTestRepo(t)
			step := "step-a"
			if kind == "no-step" {
				step = ""
			}
			seedTurnStepStampSession(t, repo, "task", step, "session")
			id := "original"
			requested := ""
			if kind == "requested" {
				requested = id
			}
			if kind == "closed-conflict" {
				id = "agent-delivery-turn-stream-1"
			}
			if kind != "no-step" {
				turn := &models.Turn{ID: id, TaskID: "task", TaskSessionID: "session", Metadata: map[string]interface{}{"original": "preserved"}}
				if kind == "closed-conflict" {
					now := time.Now().UTC()
					turn.CompletedAt = &now
				}
				require.NoError(t, repo.CreateTurn(t.Context(), turn))
			}
			tx, err := repo.db.BeginTxx(t.Context(), nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			got, err := repo.canonicalDeliveryTurnTx(t.Context(), tx, "session", "task", requested, &models.AgentDeliveryEvent{StreamID: "stream", Sequence: 1})
			require.NoError(t, err)
			require.NoError(t, tx.Commit())
			if kind == "no-step" {
				id = "agent-delivery-turn-stream-1"
			}
			require.Equal(t, id, got)
			turn, err := repo.GetTurn(t.Context(), got)
			require.NoError(t, err)
			if kind == "no-step" {
				require.Empty(t, turn.Metadata)
			} else {
				require.Equal(t, map[string]interface{}{"original": "preserved"}, turn.Metadata)
			}
		})
	}
}

func canonicalStampEvent(t *testing.T, repo *Repository, kind string, sequence int64) *models.AgentDeliveryEvent {
	t.Helper()
	agent := streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "self wake", CanonicalMessageID: "message"}
	switch kind {
	case "thinking":
		agent.Type = streams.EventTypeReasoning
		agent.ReasoningText = "reasoning"
	case "shell":
		agent.Type = streams.EventTypeToolCall
		agent.ToolCallID = "tool"
		agent.ToolTitle = "command"
		agent.ToolStatus = "pending"
		agent.NormalizedPayload = streams.NewShellExec("cmd", "/tmp", "", 0, false)
	}
	raw, err := json.Marshal(agent)
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", Sequence: sequence, EventType: agent.Type, Payload: raw}
	_, err = repo.ReceiveAgentDeliveryEvent(t.Context(), event, sequence)
	require.NoError(t, err)
	return event
}

func TestCanonicalDeliveryTurnProjectedStamp(t *testing.T) {
	for _, kind := range []string{"text", "thinking", "shell", "batch"} {
		t.Run(kind, func(t *testing.T) {
			repo := newTurnStepStampTestRepo(t)
			seedTurnStepStampSession(t, repo, "task", "step-a", "session")
			seedAgentDeliveryTestSession(t, repo, "session", "incarnation")
			events := []*models.AgentDeliveryEvent{canonicalStampEvent(t, repo, kind, 1)}
			effects := []*models.AgentDeliveryEffect{completedRetentionEventEffect(events[0])}
			if kind == "batch" {
				event := canonicalStampEvent(t, repo, kind, 2)
				events = append(events, event)
				effects = append(effects, completedRetentionEventEffect(event))
			}
			for range 2 {
				_, err := repo.ProjectCanonicalAgentDeliveryEvents(t.Context(), events, effects)
				require.NoError(t, err)
			}
			turns, err := repo.ListTurnsBySession(t.Context(), "session")
			require.NoError(t, err)
			require.Len(t, turns, 1)
			require.Equal(t, "step-a", turns[0].Metadata[models.TurnMetaKeyWorkflowStepIDAtStart])
			_, _, entry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
			require.NoError(t, err)
			require.EqualValues(t, entry, turns[0].Metadata[models.TurnMetaKeyWorkflowStepTransitionIDAtStart])
			task, err := repo.GetTask(t.Context(), "task")
			require.NoError(t, err)
			task.WorkflowStepID = "step-b"
			require.NoError(t, repo.UpdateTask(t.Context(), task))
			_, err = repo.ProjectCanonicalAgentDeliveryEvents(t.Context(), events, effects)
			require.NoError(t, err)
			stored, err := repo.GetTurn(t.Context(), turns[0].ID)
			require.NoError(t, err)
			require.Equal(t, turns[0].Metadata, stored.Metadata)
		})
	}
}

func TestCanonicalDeliveryTurnRollbackAndRetry(t *testing.T) {
	for _, target := range []string{"turn", "message", "ledger", "commit"} {
		t.Run(target, func(t *testing.T) {
			repo := newTurnStepStampTestRepo(t)
			seedTurnStepStampSession(t, repo, "task", "step-a", "session")
			seedAgentDeliveryTestSession(t, repo, "session", "incarnation")
			event := canonicalStampEvent(t, repo, "text", 1)
			effect := completedRetentionEventEffect(event)
			switch target {
			case "commit":
				_, err := repo.db.Exec(`CREATE TABLE commit_guard (task_id TEXT REFERENCES tasks(id) DEFERRABLE INITIALLY DEFERRED)`)
				require.NoError(t, err)
				_, err = repo.db.Exec(`CREATE TRIGGER fail_stamp BEFORE INSERT ON task_session_turns BEGIN INSERT INTO commit_guard(task_id) VALUES('missing'); END`)
				require.NoError(t, err)
			case "ledger":
				_, err := repo.db.Exec(`ALTER TABLE task_step_transitions RENAME TO hidden_transitions`)
				require.NoError(t, err)
			default:
				table := "task_session_turns"
				if target == "message" {
					table = "task_session_messages"
				}
				_, err := repo.db.Exec(`CREATE TRIGGER fail_stamp BEFORE INSERT ON ` + table + ` BEGIN SELECT RAISE(ABORT,'injected failure'); END`)
				require.NoError(t, err)
			}
			_, err := repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), event, effect)
			require.Error(t, err)
			for _, table := range []string{"task_session_turns", "task_session_messages", "agent_delivery_effects"} {
				var count int
				require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM `+table))
				require.Zero(t, count)
			}
			cursor, err := repo.GetAgentDeliveryCursor(t.Context(), "stream")
			require.NoError(t, err)
			require.EqualValues(t, 1, cursor.ReceivedSequence)
			require.Zero(t, cursor.ProjectedSequence)
			if target == "ledger" {
				_, err = repo.db.Exec(`ALTER TABLE hidden_transitions RENAME TO task_step_transitions`)
			} else {
				_, err = repo.db.Exec(`DROP TRIGGER fail_stamp`)
			}
			require.NoError(t, err)
			for range 2 {
				_, err = repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), event, effect)
				require.NoError(t, err)
			}
			turns, err := repo.ListTurnsBySession(t.Context(), "session")
			require.NoError(t, err)
			require.Len(t, turns, 1)
		})
	}
}

func TestCanonicalDeliveryTurnShellEligibility(t *testing.T) {
	for _, background := range []bool{true, false} {
		t.Run(map[bool]string{true: "background", false: "terminal-without-turn"}[background], func(t *testing.T) {
			repo := newTurnStepStampTestRepo(t)
			seedTurnStepStampSession(t, repo, "task", "step-a", "session")
			seedAgentDeliveryTestSession(t, repo, "session", "incarnation")
			status := "pending"
			if !background {
				status = "completed"
			}
			raw, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeToolCall, ToolCallID: "tool", ToolTitle: "command", ToolStatus: status, NormalizedPayload: streams.NewShellExec("cmd", "/tmp", "", 0, background)})
			require.NoError(t, err)
			event := &models.AgentDeliveryEvent{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", Sequence: 1, EventType: streams.EventTypeToolCall, Payload: raw}
			_, err = repo.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
			require.NoError(t, err)
			_, err = repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), event, completedRetentionEventEffect(event))
			if background {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			turns, err := repo.ListTurnsBySession(t.Context(), "session")
			require.NoError(t, err)
			require.Empty(t, turns)
		})
	}
}
