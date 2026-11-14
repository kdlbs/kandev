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
