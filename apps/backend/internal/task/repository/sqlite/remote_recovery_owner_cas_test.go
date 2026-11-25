package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

func TestRepairExecutorRunningDeadIfOwnerCurrentValidatesOwnerBeforeRepair(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	called := false
	err := repo.RepairExecutorRunningDeadIfOwnerCurrent(ctx, record.SessionID, record.AgentExecutionID,
		record.UpdatedAt, owner, func(context.Context) error {
			called = true
			return errors.New("owner changed")
		})
	require.ErrorContains(t, err, "owner changed")
	require.True(t, called)
	current, err := repo.GetExecutorRunningBySessionID(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusRunning, current.Status)

	require.NoError(t, repo.RepairExecutorRunningDeadIfOwnerCurrent(ctx, record.SessionID, record.AgentExecutionID,
		record.UpdatedAt, owner, func(context.Context) error { return nil }))
	current, err = repo.GetExecutorRunningBySessionID(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, current.Status)
	require.Equal(t, record.ResumeToken, current.ResumeToken)
}

func TestSettleStoppedRemoteExecutionIfOwnerCurrentCommitsRuntimeSessionAndTurnTogether(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	startedAt := time.Now().UTC().Add(-time.Minute)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, record.SessionID, models.TaskSessionStateRunning, ""))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "remote-recovery-owner-turn", TaskSessionID: record.SessionID, TaskID: record.TaskID,
		StartedAt: startedAt, CreatedAt: startedAt, UpdatedAt: startedAt,
	}))
	active, err := repo.GetActiveTurnBySessionID(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, "remote-recovery-owner-turn", active.ID)
	tool := &models.Message{
		ID: "remote-recovery-owner-tool", TaskID: record.TaskID, TaskSessionID: record.SessionID,
		TurnID: active.ID, Type: models.MessageTypeToolCall,
		Metadata: map[string]interface{}{"tool_call_id": "remote-recovery-call", "status": "running"},
	}
	require.NoError(t, repo.CreateMessage(ctx, tool))

	changed, updatedAt, err := repo.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, record.SessionID, record.AgentExecutionID, record.UpdatedAt,
		models.TaskSessionStateRunning, active.ID, "", nil, owner, func(context.Context) error { return nil },
	)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, updatedAt.IsZero())
	runtime, err := repo.GetExecutorRunningBySessionID(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, runtime.Status)
	session, err := repo.GetTaskSession(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	require.Equal(t, updatedAt, session.UpdatedAt)
	turn, err := repo.GetTurn(ctx, active.ID)
	require.NoError(t, err)
	require.NotNil(t, turn.CompletedAt)
	require.Equal(t, turn.StartedAt, *turn.CompletedAt, "interrupted turn must have zero duration")
	settledTool, err := repo.GetMessage(ctx, tool.ID)
	require.NoError(t, err)
	require.Equal(t, "complete", settledTool.Metadata["status"], "interrupted-turn cleanup must settle its nonterminal tool calls")
}

func TestSettleStoppedRemoteExecutionIfOwnerCurrentCompletesProvenTerminalTurn(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	require.NoError(t, repo.UpdateTaskSessionState(ctx, record.SessionID, models.TaskSessionStateRunning, ""))
	startedAt := time.Now().UTC().Add(-time.Minute)
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "remote-recovery-owner-terminal-turn", TaskSessionID: record.SessionID, TaskID: record.TaskID,
		StartedAt: startedAt, CreatedAt: startedAt, UpdatedAt: startedAt,
	}))

	changed, _, err := repo.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, record.SessionID, record.AgentExecutionID, record.UpdatedAt,
		models.TaskSessionStateRunning, "remote-recovery-owner-terminal-turn", models.DeliverySubmissionFailed,
		nil, owner, func(context.Context) error { return nil },
	)
	require.NoError(t, err)
	require.True(t, changed)
	turn, err := repo.GetTurn(ctx, "remote-recovery-owner-terminal-turn")
	require.NoError(t, err)
	require.NotNil(t, turn.CompletedAt)
	require.True(t, turn.CompletedAt.After(turn.StartedAt), "proven terminal output completes its original turn")
}

func TestSettleStoppedRemoteExecutionIfOwnerCurrentWritesWorkflowOutcomeAtomically(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	const (
		streamID     = "remote-recovery-terminal-stream"
		submissionID = "remote-recovery-terminal-submission"
		turnID       = "remote-recovery-terminal-turn"
	)
	session, err := repo.GetTaskSession(ctx, record.SessionID)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET workflow_id = '', workflow_step_id = '' WHERE id = ?`), record.TaskID)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, record.SessionID, models.TaskSessionStateRunning, ""))
	startedAt := time.Now().UTC().Add(-time.Minute)
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskSessionID: record.SessionID, TaskID: record.TaskID,
		StartedAt: startedAt, CreatedAt: startedAt, UpdatedAt: startedAt,
	}))
	now := time.Now().UTC()
	submission := &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: record.SessionID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "terminal-payload-hash",
		Payload: []byte("terminal prompt"), State: models.DeliverySubmissionDispatching,
	}
	_, err = repo.PrepareAgentDeliverySubmission(ctx, submission)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_submissions
		SET state = ?, outcome = ?, terminal_stream_id = ?, terminal_sequence = ?, terminal_turn_id = ?
		WHERE id = ?`), models.DeliverySubmissionFailed, models.DeliverySubmissionFailed,
		streamID, 1, turnID, submissionID)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`INSERT INTO agent_delivery_cursors
		(stream_id, session_id, incarnation_id, harness_generation, received_sequence, projected_sequence, remote_high_water, updated_at)
		VALUES (?, ?, ?, ?, 1, 1, 1, ?)`), streamID, record.SessionID, session.QueueIncarnationID, 1, now)
	require.NoError(t, err)
	terminal := &models.AgentDeliveryEffect{
		EffectKey: "agent_delivery.terminal:" + streamID + ":1", StreamID: streamID, Sequence: 1,
		EffectType: models.DeliveryTerminalEffectType, State: models.DeliveryEffectCompleted,
		SessionID: record.SessionID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
		SubmissionID: submissionID, TurnID: turnID, Outcome: string(models.DeliverySubmissionFailed),
	}
	_, err = insertDeliveryEffectTx(ctx, repo.db, repo.db.Rebind, terminal)
	require.NoError(t, err)
	completedAt := now
	workflow := &models.AgentDeliveryEffect{
		EffectKey: "workflow.on_turn_complete:" + turnID, EffectType: "workflow.on_turn_complete",
		State: models.DeliveryEffectCompleted, StreamID: streamID, Sequence: 1,
		TaskID: record.TaskID, SessionID: record.SessionID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, SubmissionID: submissionID, TurnID: turnID,
		Outcome: models.DeliveryWorkflowOutcomeStoppedRecovery, CreatedAt: now, CompletedAt: &completedAt,
	}
	ownerChanged := errors.New("owner changed")
	_, _, err = repo.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, record.SessionID, record.AgentExecutionID, record.UpdatedAt,
		models.TaskSessionStateRunning, turnID, models.DeliverySubmissionFailed, workflow, owner,
		func(context.Context) error { return ownerChanged },
	)
	require.ErrorIs(t, err, ownerChanged)
	_, err = repo.GetAgentDeliveryEffect(ctx, workflow.EffectKey)
	require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryEffectNotFound,
		"owner rejection must roll back the workflow receipt with the turn settlement")

	changed, _, err := repo.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, record.SessionID, record.AgentExecutionID, record.UpdatedAt,
		models.TaskSessionStateRunning, turnID, models.DeliverySubmissionFailed, workflow, owner,
		func(context.Context) error { return nil },
	)
	require.NoError(t, err)
	require.True(t, changed)
	stored, err := repo.GetAgentDeliveryEffect(ctx, workflow.EffectKey)
	require.NoError(t, err)
	require.Equal(t, workflow.Outcome, stored.Outcome)
}

func TestSettleStoppedRemoteExecutionIfOwnerCurrentRollsBackWhenOwnerChanges(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	require.NoError(t, repo.UpdateTaskSessionState(ctx, record.SessionID, models.TaskSessionStateRunning, ""))
	startedAt := time.Now().UTC().Add(-time.Minute)
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "remote-recovery-owner-stale-turn", TaskSessionID: record.SessionID, TaskID: record.TaskID,
		StartedAt: startedAt, CreatedAt: startedAt, UpdatedAt: startedAt,
	}))
	ownerChanged := errors.New("owner changed")
	changed, _, err := repo.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, record.SessionID, record.AgentExecutionID, record.UpdatedAt,
		models.TaskSessionStateRunning, "remote-recovery-owner-stale-turn", "", nil, owner,
		func(context.Context) error { return ownerChanged },
	)
	require.ErrorIs(t, err, ownerChanged)
	require.False(t, changed)
	runtime, err := repo.GetExecutorRunningBySessionID(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusRunning, runtime.Status)
	session, err := repo.GetTaskSession(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	turn, err := repo.GetTurn(ctx, "remote-recovery-owner-stale-turn")
	require.NoError(t, err)
	require.Nil(t, turn.CompletedAt)
}

func TestSettleStoppedRemoteExecutionIfOwnerCurrentPreservesExistingWaitingState(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	changed, _, err := repo.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, record.SessionID, record.AgentExecutionID, record.UpdatedAt,
		models.TaskSessionStateWaitingForInput, "", "", nil, owner, func(context.Context) error { return nil },
	)
	require.NoError(t, err)
	require.False(t, changed)
	runtime, err := repo.GetExecutorRunningBySessionID(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, runtime.Status)
	session, err := repo.GetTaskSession(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
}

func TestSettleStoppedRemoteExecutionIfOwnerCurrentRejectsChangedTurnAtomically(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	require.NoError(t, repo.UpdateTaskSessionState(ctx, record.SessionID, models.TaskSessionStateRunning, ""))
	err := repo.CreateTurn(ctx, &models.Turn{
		ID: "remote-recovery-owner-replaced-turn", TaskSessionID: record.SessionID, TaskID: record.TaskID,
		StartedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	_, _, err = repo.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, record.SessionID, record.AgentExecutionID, record.UpdatedAt,
		models.TaskSessionStateRunning, "different-turn", "", nil, owner, func(context.Context) error { return nil },
	)
	require.Error(t, err)
	runtime, err := repo.GetExecutorRunningBySessionID(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusRunning, runtime.Status)
	session, err := repo.GetTaskSession(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	turn, err := repo.GetTurn(ctx, "remote-recovery-owner-replaced-turn")
	require.NoError(t, err)
	require.Nil(t, turn.CompletedAt)
}

func TestConfirmExecutorRunningAbsentIfOwnerCurrentRejectsReappearedRow(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	called := false
	err := repo.ConfirmExecutorRunningAbsentIfOwnerCurrent(ctx, record.SessionID, owner, func(context.Context) error {
		called = true
		return nil
	})
	require.ErrorIs(t, err, models.ErrExecutionRotated)
	require.False(t, called, "owner validation must not run while the executor row exists")

	require.NoError(t, repo.DeleteExecutorRunningBySessionID(ctx, record.SessionID))
	require.NoError(t, repo.ConfirmExecutorRunningAbsentIfOwnerCurrent(ctx, record.SessionID, owner, func(context.Context) error {
		return nil
	}))
}

func seedRemoteRecoveryOwnerCAS(t *testing.T) (*Repository, models.RemoteRecoveryOwnerLockIDs, *models.ExecutorRunning) {
	t.Helper()
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID        = "remote-recovery-owner-task"
		workspaceID   = "ws-remote-recovery-owner-task"
		sessionID     = "remote-recovery-owner-session"
		environmentID = "remote-recovery-owner-environment"
		executionID   = "remote-recovery-owner-execution"
	)
	seedExecutorRunningCleanupTask(t, repo, taskID)
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: string(models.ExecutorTypeRemoteDocker),
		ExecutorID: "remote-recovery-owner-executor", WorkspacePath: "/workspace/remote-recovery-owner",
		Status: models.TaskEnvironmentStatusReady,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		State: models.TaskSessionStateWaitingForInput,
	}))
	row := &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		Runtime: agentruntime.RuntimeRemoteDocker, Status: models.ExecutorRunningStatusRunning,
		ResumeToken: "resume-owner-cas", Resumable: true,
	}
	require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
	current, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	require.NoError(t, err)
	return repo, models.RemoteRecoveryOwnerLockIDs{
		TaskID: taskID, SessionID: sessionID, WorkspaceID: workspaceID,
		EnvironmentID: environmentID, EnvironmentOwnerTaskID: taskID,
	}, current
}
