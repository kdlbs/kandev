package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresSettleStoppedRemoteExecutionIfOwnerCurrent(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const (
		taskID        = "task-stopped-remote-settlement-pg"
		workspaceID   = "ws-task-stopped-remote-settlement-pg"
		sessionID     = "session-stopped-remote-settlement-pg"
		environmentID = "environment-stopped-remote-settlement-pg"
		executionID   = "execution-stopped-remote-settlement-pg"
		turnID        = "turn-stopped-remote-settlement-pg"
		toolID        = "tool-stopped-remote-settlement-pg"
	)
	seedExecutorRunningCleanupTask(t, repo, taskID)
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: string(models.ExecutorTypeRemoteDocker),
		ExecutorID: "executor-stopped-remote-settlement-pg", WorkspacePath: "/workspace/stopped-remote-settlement-pg",
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		Runtime: agentruntime.RuntimeRemoteDocker, Status: models.ExecutorRunningStatusReady,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	startedAt := time.Now().UTC().Add(-time.Minute)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: startedAt, CreatedAt: startedAt, UpdatedAt: startedAt,
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: toolID, TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID,
		Type: models.MessageTypeToolCall, Metadata: map[string]interface{}{
			"tool_call_id": "stopped-remote-tool-call", "status": "running",
		},
	}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	observed, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID: %v", err)
	}
	owner := models.RemoteRecoveryOwnerLockIDs{
		TaskID: taskID, SessionID: sessionID, WorkspaceID: workspaceID,
		EnvironmentID: environmentID, EnvironmentOwnerTaskID: taskID,
	}
	changed, _, err := repo.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, sessionID, executionID, observed.UpdatedAt, models.TaskSessionStateRunning,
		turnID, "", nil, owner, func(context.Context) error { return nil },
	)
	if err != nil {
		t.Fatalf("SettleStoppedRemoteExecutionIfOwnerCurrent: %v", err)
	}
	if !changed {
		t.Fatal("active session did not transition to waiting")
	}
	runtime, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || runtime.Status != models.ExecutorRunningStatusStopped {
		t.Fatalf("executor status = %v, err=%v; want stopped", runtime, err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil || session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %v, err=%v; want waiting", session, err)
	}
	turn, err := repo.GetTurn(ctx, turnID)
	if err != nil || turn.CompletedAt == nil || !turn.CompletedAt.Equal(turn.StartedAt) {
		t.Fatalf("interrupted turn = %v, err=%v; want zero-duration completion", turn, err)
	}
	message, err := repo.GetMessage(ctx, toolID)
	if err != nil || message.Metadata["status"] != "complete" {
		t.Fatalf("tool message = %v, err=%v; want complete", message, err)
	}
}

func TestPostgresRepairExecutorRunningDeadIfOwnerCurrentSerializesOwnerAndDeliveryChanges(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	observerDB := testutil.OpenIsolatedPostgres(t, dsn)
	seedRepo, err := NewWithDB(observerDB, observerDB, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const (
		taskID        = "task-remote-recovery-owner-cas-pg"
		workspaceID   = "ws-task-remote-recovery-owner-cas-pg"
		sessionID     = "session-remote-recovery-owner-cas-pg"
		environmentID = "environment-remote-recovery-owner-cas-pg"
		executionID   = "execution-remote-recovery-owner-cas-pg"
	)
	seedExecutorRunningCleanupTask(t, seedRepo, taskID)
	if err := seedRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: string(models.ExecutorTypeRemoteDocker),
		ExecutorID: "executor-remote-recovery-owner-cas-pg", WorkspacePath: "/workspace/remote-recovery-owner-cas-pg",
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := seedRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	session, err := seedRepo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if err := seedRepo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		Runtime: agentruntime.RuntimeRemoteDocker, Status: models.ExecutorRunningStatusRunning,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	const (
		submissionID = "submission-remote-recovery-owner-cas-pg"
		streamID     = "stream-remote-recovery-owner-cas-pg"
	)
	if _, err := seedRepo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: sessionID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "owner-cas-pg-payload",
		Payload: []byte("pending prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	}); err != nil {
		t.Fatalf("PrepareAgentDeliverySubmission: %v", err)
	}
	recovery := models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryReconnecting, SessionID: sessionID,
		AgentExecutionID: executionID, SubmissionID: submissionID, StreamID: streamID,
		IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, PromptGeneration: 1,
	}
	block := &models.SessionRecoveryBlock{
		ID: "block-remote-recovery-owner-cas-pg", SessionID: sessionID,
		IncarnationID: session.QueueIncarnationID, ExpectedGeneration: 1,
		Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: submissionID, DeliveryStreamID: streamID,
	}
	if stored, err := seedRepo.UpsertAgentDeliveryRecovery(ctx, &recovery, block); err != nil || !stored {
		t.Fatalf("UpsertAgentDeliveryRecovery: stored=%v, err=%v", stored, err)
	}
	observed, err := seedRepo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID: %v", err)
	}
	owner := models.RemoteRecoveryOwnerLockIDs{
		TaskID: taskID, SessionID: sessionID, WorkspaceID: workspaceID,
		EnvironmentID: environmentID, EnvironmentOwnerTaskID: taskID,
	}

	moverWriterDB := openSecondPostgresConnection(t, dsn, observerDB)
	moverReaderDB := openSecondPostgresConnection(t, dsn, observerDB)
	ownerChangerDB := openSecondPostgresConnection(t, dsn, observerDB)
	deliveryChangerDB := openSecondPostgresConnection(t, dsn, observerDB)
	moverRepo, err := NewWithDB(moverWriterDB, moverReaderDB, nil)
	if err != nil {
		t.Fatalf("init mover repository: %v", err)
	}
	var ownerChangerPID int
	if err := ownerChangerDB.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&ownerChangerPID); err != nil {
		t.Fatalf("read owner changer backend PID: %v", err)
	}
	var deliveryChangerPID int
	if err := deliveryChangerDB.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&deliveryChangerPID); err != nil {
		t.Fatalf("read delivery changer backend PID: %v", err)
	}
	deliveryChangerRepo, err := NewWithDB(deliveryChangerDB, deliveryChangerDB, nil)
	if err != nil {
		t.Fatalf("init delivery changer repository: %v", err)
	}

	validated := make(chan struct{})
	releaseValidation := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseValidation) }) }
	moverDone := make(chan struct{})
	var moverErr error
	go func() {
		defer close(moverDone)
		moverErr = moverRepo.RepairExecutorRunningDeadIfOwnerCurrent(ctx, sessionID, executionID, observed.UpdatedAt, owner, func(context.Context) error {
			close(validated)
			<-releaseValidation
			return nil
		})
	}()
	select {
	case <-validated:
	case <-time.After(5 * time.Second):
		t.Fatal("owner validator did not run")
	}

	ownerChangeDone := make(chan struct{})
	var ownerChangeErr error
	go func() {
		defer close(ownerChangeDone)
		_, ownerChangeErr = ownerChangerDB.ExecContext(ctx,
			`UPDATE tasks SET archived_at = $1 WHERE id = $2`, time.Now().UTC(), taskID)
	}()
	deliveryChangeDone := make(chan struct{})
	deliveryChangeErr := make(chan error, 1)
	go func() {
		defer close(deliveryChangeDone)
		changed := recovery
		changed.Phase = models.AgentDeliveryRecoveryUncertain
		changed.Revision = 0
		changed.UpdatedAt = time.Now().UTC()
		_, updateErr := deliveryChangerRepo.UpsertAgentDeliveryRecovery(ctx, &changed, block)
		deliveryChangeErr <- updateErr
	}()
	t.Cleanup(func() {
		release()
		select {
		case <-moverDone:
		case <-time.After(5 * time.Second):
			t.Errorf("timed out waiting for owner-locked compare-and-set")
		}
		select {
		case <-ownerChangeDone:
		case <-time.After(5 * time.Second):
			t.Errorf("timed out waiting for concurrent owner change")
		}
		select {
		case <-deliveryChangeDone:
		case <-time.After(5 * time.Second):
			t.Errorf("timed out waiting for concurrent delivery recovery change")
		}
	})
	if err := waitForPostgresLock(ctx, observerDB, ownerChangerPID, ownerChangeDone); err != nil {
		t.Fatal(err)
	}
	if err := waitForPostgresLock(ctx, observerDB, deliveryChangerPID, deliveryChangeDone); err != nil {
		t.Fatal(err)
	}
	select {
	case <-moverDone:
		t.Fatal("owner-locked compare-and-set returned before the validated owner row was unlocked")
	default:
	}
	release()
	select {
	case <-moverDone:
		if moverErr != nil {
			t.Fatalf("RepairExecutorRunningDeadIfOwnerCurrent: %v", moverErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for owner-locked compare-and-set")
	}
	select {
	case <-ownerChangeDone:
		if ownerChangeErr != nil {
			t.Fatalf("concurrent owner change: %v", ownerChangeErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for concurrent owner change after compare-and-set")
	}
	select {
	case <-deliveryChangeDone:
		if err := <-deliveryChangeErr; err != nil {
			t.Fatalf("concurrent delivery recovery change: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for delivery recovery change after compare-and-set")
	}
	current, err := seedRepo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID after owner change: %v", err)
	}
	if current.Status != models.ExecutorRunningStatusStopped {
		t.Fatalf("executor status = %q, want stopped before owner change", current.Status)
	}
	currentSession, err := seedRepo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession after delivery recovery change: %v", err)
	}
	currentRecovery, ok := models.LoadAgentDeliveryRecovery(currentSession.Metadata)
	if !ok || currentRecovery.Phase != models.AgentDeliveryRecoveryUncertain || currentRecovery.Revision != 2 {
		t.Fatalf("delivery recovery after owner-locked compare-and-set = %+v, want uncertain revision 2", currentRecovery)
	}
}

func TestPostgresSettleStoppedRemoteExecutionIfOwnerCurrentSerializesOwnerChange(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	observerDB := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(observerDB, observerDB, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const (
		taskID        = "task-stopped-remote-cas-lock-pg"
		workspaceID   = "ws-task-stopped-remote-cas-lock-pg"
		sessionID     = "session-stopped-remote-cas-lock-pg"
		environmentID = "environment-stopped-remote-cas-lock-pg"
		executionID   = "execution-stopped-remote-cas-lock-pg"
		turnID        = "turn-stopped-remote-cas-lock-pg"
	)
	seedExecutorRunningCleanupTask(t, repo, taskID)
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: string(models.ExecutorTypeRemoteDocker),
		ExecutorID: "executor-stopped-remote-cas-lock-pg", WorkspacePath: "/workspace/stopped-remote-cas-lock-pg",
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		Runtime: agentruntime.RuntimeRemoteDocker, Status: models.ExecutorRunningStatusReady,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	startedAt := time.Now().UTC().Add(-time.Minute)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: startedAt, CreatedAt: startedAt, UpdatedAt: startedAt,
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	observed, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID: %v", err)
	}
	owner := models.RemoteRecoveryOwnerLockIDs{
		TaskID: taskID, SessionID: sessionID, WorkspaceID: workspaceID,
		EnvironmentID: environmentID, EnvironmentOwnerTaskID: taskID,
	}
	ownerChangerDB := openSecondPostgresConnection(t, dsn, observerDB)
	lockObserverDB := openSecondPostgresConnection(t, dsn, observerDB)
	var ownerChangerPID int
	if err := ownerChangerDB.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&ownerChangerPID); err != nil {
		t.Fatalf("read owner changer backend PID: %v", err)
	}
	validated := make(chan struct{})
	releaseValidation := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseValidation) }) }
	settlementDone := make(chan struct{})
	var settlementErr error
	go func() {
		defer close(settlementDone)
		_, _, settlementErr = repo.SettleStoppedRemoteExecutionIfOwnerCurrent(
			ctx, sessionID, executionID, observed.UpdatedAt, models.TaskSessionStateRunning,
			turnID, "", nil, owner, func(context.Context) error {
				close(validated)
				<-releaseValidation
				return nil
			},
		)
	}()
	select {
	case <-validated:
	case <-time.After(5 * time.Second):
		t.Fatal("settlement owner validator did not run")
	}
	ownerChangeDone := make(chan struct{})
	var ownerChangeErr error
	go func() {
		defer close(ownerChangeDone)
		_, ownerChangeErr = ownerChangerDB.ExecContext(ctx,
			`UPDATE tasks SET archived_at = $1 WHERE id = $2`, time.Now().UTC(), taskID)
	}()
	t.Cleanup(func() {
		release()
		select {
		case <-settlementDone:
		case <-time.After(5 * time.Second):
			t.Errorf("timed out waiting for stopped settlement")
		}
		select {
		case <-ownerChangeDone:
		case <-time.After(5 * time.Second):
			t.Errorf("timed out waiting for concurrent owner update")
		}
	})
	if err := waitForPostgresLock(ctx, lockObserverDB, ownerChangerPID, ownerChangeDone); err != nil {
		t.Fatal(err)
	}
	select {
	case <-settlementDone:
		t.Fatal("stopped settlement returned before the validated owner row was unlocked")
	default:
	}
	release()
	select {
	case <-settlementDone:
		if settlementErr != nil {
			t.Fatalf("SettleStoppedRemoteExecutionIfOwnerCurrent: %v", settlementErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for stopped settlement")
	}
	select {
	case <-ownerChangeDone:
		if ownerChangeErr != nil {
			t.Fatalf("concurrent owner change: %v", ownerChangeErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("owner change did not continue after stopped settlement")
	}
	current, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || current.Status != models.ExecutorRunningStatusStopped {
		t.Fatalf("executor status = %v, err=%v; want stopped", current, err)
	}
	settledSession, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil || settledSession.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %v, err=%v; want waiting", settledSession, err)
	}
}
