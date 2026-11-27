package orchestrator

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestStartupReconciliationPreservesOnlyExactPendingRemoteOwner(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-pending", "session-pending", models.TaskSessionStateRunning)
	seedTaskAndSession(t, repo, "task-pending-starting", "session-pending-starting", models.TaskSessionStateStarting)
	seedTaskAndSession(t, repo, "task-pending-wfi", "session-pending-wfi", models.TaskSessionStateWaitingForInput)
	seedTaskAndSession(t, repo, "task-unrelated", "session-unrelated", models.TaskSessionStateRunning)

	rows := []*models.ExecutorRunning{
		{
			ID: "running-pending", SessionID: "session-pending", TaskID: "task-pending",
			Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusRunning,
			AgentExecutionID: "execution-pending", ResumeToken: "resume-pending",
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		},
		{
			ID: "running-pending-starting", SessionID: "session-pending-starting", TaskID: "task-pending-starting",
			Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusStarting,
			AgentExecutionID: "execution-pending-starting", ResumeToken: "resume-pending-starting",
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		},
		{
			ID: "running-pending-wfi", SessionID: "session-pending-wfi", TaskID: "task-pending-wfi",
			Runtime: agentruntime.RuntimeKubernetes, Status: models.ExecutorRunningStatusRunning,
			AgentExecutionID: "execution-pending-wfi", ResumeToken: "resume-pending-wfi",
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		},
		{
			ID: "running-unrelated", SessionID: "session-unrelated", TaskID: "task-unrelated",
			Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusRunning,
			AgentExecutionID: "execution-unrelated", ResumeToken: "resume-unrelated",
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		},
	}
	for _, row := range rows {
		if err := repo.UpsertExecutorRunning(ctx, row); err != nil {
			t.Fatalf("upsert executor row %s: %v", row.SessionID, err)
		}
		if err := repo.SetTaskMetadataKey(ctx, row.TaskID, "startup_sentinel", row.AgentExecutionID); err != nil {
			t.Fatalf("set task metadata for %s: %v", row.TaskID, err)
		}
		turn := &models.Turn{
			ID: row.SessionID + "-turn", TaskSessionID: row.SessionID, TaskID: row.TaskID,
			StartedAt: time.Now().UTC(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		if err := repo.CreateTurn(ctx, turn); err != nil {
			t.Fatalf("create open turn for %s: %v", row.SessionID, err)
		}
	}
	pendingRowsBefore := make(map[string]*models.ExecutorRunning, len(rows))
	for _, row := range rows {
		persisted, loadErr := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
		if loadErr != nil {
			t.Fatalf("load executor row %s before reconciliation: %v", row.SessionID, loadErr)
		}
		pendingRowsBefore[row.SessionID] = persisted
	}

	pendingTaskBefore, err := repo.GetTask(ctx, "task-pending")
	if err != nil {
		t.Fatalf("load pending task before reconciliation: %v", err)
	}
	pendingTaskMetadata := cloneTaskMetadata(pendingTaskBefore.Metadata)
	pendingWFITaskBefore, err := repo.GetTask(ctx, "task-pending-wfi")
	if err != nil {
		t.Fatalf("load waiting pending task before reconciliation: %v", err)
	}
	pendingWFITaskMetadata := cloneTaskMetadata(pendingWFITaskBefore.Metadata)
	pendingStartingTaskBefore, err := repo.GetTask(ctx, "task-pending-starting")
	if err != nil {
		t.Fatalf("load starting pending task before reconciliation: %v", err)
	}
	pendingStartingTaskMetadata := cloneTaskMetadata(pendingStartingTaskBefore.Metadata)
	pendingSessionsBefore := make(map[string]*models.TaskSession, 3)
	for _, sessionID := range []string{"session-pending", "session-pending-starting", "session-pending-wfi"} {
		session, loadErr := repo.GetTaskSession(ctx, sessionID)
		if loadErr != nil {
			t.Fatalf("load pending session %s before reconciliation: %v", sessionID, loadErr)
		}
		pendingSessionsBefore[sessionID] = session
	}

	agentManager := &mockAgentManager{}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-pending", v1.TaskStateInProgress)
	seedMockTaskState(taskRepo, "task-pending-starting", v1.TaskStateInProgress)
	seedMockTaskState(taskRepo, "task-pending-wfi", v1.TaskStateInProgress)
	seedMockTaskState(taskRepo, "task-unrelated", v1.TaskStateInProgress)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.turnService = &repoTurnService{repo: repo}
	svc.SetRetrackedSessionChecker(func(string) bool { return false })
	checkedOwners := make(map[string]string)
	svc.SetPendingRemoteRecoveryChecker(func(sessionID, executionID string) bool {
		checkedOwners[sessionID] = executionID
		return sessionID == "session-pending" && executionID == "execution-pending" ||
			sessionID == "session-pending-starting" && executionID == "execution-pending-starting" ||
			sessionID == "session-pending-wfi" && executionID == "execution-pending-wfi"
	})

	svc.reconcileSessionsOnStartup(ctx)

	for _, pending := range []struct {
		sessionID   string
		taskID      string
		executionID string
		state       models.TaskSessionState
		metadata    map[string]interface{}
	}{
		{sessionID: "session-pending", taskID: "task-pending", executionID: "execution-pending", state: models.TaskSessionStateRunning, metadata: pendingTaskMetadata},
		{sessionID: "session-pending-starting", taskID: "task-pending-starting", executionID: "execution-pending-starting", state: models.TaskSessionStateStarting, metadata: pendingStartingTaskMetadata},
		{sessionID: "session-pending-wfi", taskID: "task-pending-wfi", executionID: "execution-pending-wfi", state: models.TaskSessionStateWaitingForInput, metadata: pendingWFITaskMetadata},
	} {
		pendingSession, loadErr := repo.GetTaskSession(ctx, pending.sessionID)
		if loadErr != nil {
			t.Fatalf("load pending session %s: %v", pending.sessionID, loadErr)
		}
		if pendingSession.State != pending.state {
			t.Errorf("pending remote session %s state = %q, want %q", pending.sessionID, pendingSession.State, pending.state)
		}
		if !reflect.DeepEqual(pendingSession, pendingSessionsBefore[pending.sessionID]) {
			t.Errorf("pending remote session %s changed: got %#v, want %#v", pending.sessionID, pendingSession, pendingSessionsBefore[pending.sessionID])
		}
		pendingTaskAfter, taskErr := repo.GetTask(ctx, pending.taskID)
		if taskErr != nil {
			t.Fatalf("load pending task %s after reconciliation: %v", pending.taskID, taskErr)
		}
		if state, updated := taskRepo.updatedStates[pending.taskID]; updated {
			t.Errorf("pending remote task %s state was updated to %q", pending.taskID, state)
		}
		if taskRepo.tasks[pending.taskID].State != v1.TaskStateInProgress {
			t.Errorf("pending remote task %s state = %q, want %q", pending.taskID, taskRepo.tasks[pending.taskID].State, v1.TaskStateInProgress)
		}
		if !reflect.DeepEqual(pendingTaskAfter.Metadata, pending.metadata) {
			t.Errorf("pending remote task %s metadata changed: got %#v, want %#v", pending.taskID, pendingTaskAfter.Metadata, pending.metadata)
		}
		if got := openTurnCount(t, repo, pending.sessionID); got != 1 {
			t.Errorf("pending remote open turns for %s = %d, want 1", pending.sessionID, got)
		}
		pendingRow, rowErr := repo.GetExecutorRunningBySessionID(ctx, pending.sessionID)
		if rowErr != nil {
			t.Fatalf("load pending executor row %s: %v", pending.sessionID, rowErr)
		}
		if !reflect.DeepEqual(pendingRow, pendingRowsBefore[pending.sessionID]) {
			t.Errorf("pending remote executor row %s changed: got %#v, want %#v", pending.sessionID, pendingRow, pendingRowsBefore[pending.sessionID])
		}
		if checkedOwners[pending.sessionID] != pending.executionID {
			t.Errorf("pending owner query for %s used execution %q, want %q", pending.sessionID, checkedOwners[pending.sessionID], pending.executionID)
		}
	}

	unrelatedSession, err := repo.GetTaskSession(ctx, "session-unrelated")
	if err != nil {
		t.Fatalf("load unrelated session: %v", err)
	}
	if unrelatedSession.State != models.TaskSessionStateWaitingForInput {
		t.Errorf("unrelated remote session state = %q, want normal reconciliation to %q", unrelatedSession.State, models.TaskSessionStateWaitingForInput)
	}
	if state, updated := taskRepo.updatedStates["task-unrelated"]; !updated || state != v1.TaskStateReview {
		t.Errorf("unrelated remote task state update = %q (updated=%v), want normal reconciliation to %q", state, updated, v1.TaskStateReview)
	}
	if got := openTurnCount(t, repo, "session-unrelated"); got != 0 {
		t.Errorf("unrelated remote open turns = %d, want normal reconciliation to abandon the turn", got)
	}
	if len(agentManager.capturedPrompts) != 0 {
		t.Errorf("startup reconciliation dispatched prompts: %#v", agentManager.capturedPrompts)
	}
}

func TestStartupReconciliationReadsPendingBeforeAdoptionCompletion(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-transition", "session-transition", models.TaskSessionStateRunning)
	row := &models.ExecutorRunning{
		ID: "running-transition", SessionID: "session-transition", TaskID: "task-transition",
		Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusRunning,
		AgentExecutionID: "execution-transition", ResumeToken: "resume-transition",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.UpsertExecutorRunning(ctx, row); err != nil {
		t.Fatalf("upsert executor row: %v", err)
	}
	turn := &models.Turn{
		ID: "turn-transition", TaskSessionID: row.SessionID, TaskID: row.TaskID,
		StartedAt: time.Now().UTC(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		t.Fatalf("create open turn: %v", err)
	}

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, row.TaskID, v1.TaskStateInProgress)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	svc.turnService = &repoTurnService{repo: repo}
	pending := true
	retracked := false
	var reads []string
	svc.SetPendingRemoteRecoveryChecker(func(sessionID, executionID string) bool {
		reads = append(reads, "pending")
		if sessionID != row.SessionID || executionID != row.AgentExecutionID {
			t.Errorf("pending query = (%q, %q), want (%q, %q)", sessionID, executionID, row.SessionID, row.AgentExecutionID)
		}
		// Simulate a completed adoption between the two reads. The manager
		// publishes the success marker before removing the pending generation.
		if pending {
			retracked = true
			pending = false
		}
		return pending
	})
	svc.SetRetrackedSessionChecker(func(sessionID string) bool {
		reads = append(reads, "retracked")
		if sessionID != row.SessionID {
			t.Errorf("retracked query session = %q, want %q", sessionID, row.SessionID)
		}
		// If this is read first, it sees false before the transition below; a
		// following pending read sees false too and would reconcile the session.
		return retracked
	})

	svc.reconcileSessionsOnStartup(ctx)

	if !reflect.DeepEqual(reads, []string{"pending", "retracked"}) {
		t.Errorf("startup ownership reads = %v, want pending before retracked", reads)
	}
	session, err := repo.GetTaskSession(ctx, row.SessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Errorf("session state after adoption completion = %q, want %q", session.State, models.TaskSessionStateRunning)
	}
	if state, updated := taskRepo.updatedStates[row.TaskID]; updated {
		t.Errorf("task state after adoption completion was updated to %q", state)
	}
	if got := openTurnCount(t, repo, row.SessionID); got != 1 {
		t.Errorf("open turns after adoption completion = %d, want 1", got)
	}
}

func TestStartupNoticeRetirementPreservesPendingRemoteEpisode(t *testing.T) {
	ctx := context.Background()
	svc, taskSvc, _ := newPersistentTransientRetryTestService(t)
	repo := svc.repo.(*sqliterepo.Repository)
	row := &models.ExecutorRunning{
		ID: "running-s1", SessionID: "s1", TaskID: "t1",
		Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusRunning,
		AgentExecutionID: "execution-s1", ResumeToken: "resume-s1",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.UpsertExecutorRunning(ctx, row); err != nil {
		t.Fatalf("upsert executor row: %v", err)
	}
	turn := &models.Turn{
		ID: "turn-s1", TaskSessionID: row.SessionID, TaskID: row.TaskID,
		StartedAt: time.Now().UTC(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		t.Fatalf("create open turn: %v", err)
	}
	createPersistedTransientRetryNotice(t, svc)
	taskRepo := svc.taskRepo.(*mockTaskRepo)
	seedMockTaskState(taskRepo, row.TaskID, v1.TaskStateInProgress)
	svc.turnService = &repoTurnService{repo: repo}
	svc.SetPendingRemoteRecoveryChecker(func(sessionID, executionID string) bool {
		return sessionID == row.SessionID && executionID == row.AgentExecutionID
	})

	svc.reconcileSessionsOnStartup(ctx)

	session, err := repo.GetTaskSession(ctx, row.SessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Errorf("pending notice session state = %q, want %q", session.State, models.TaskSessionStateRunning)
	}
	if got := openTurnCount(t, repo, row.SessionID); got != 1 {
		t.Errorf("pending notice open turns = %d, want 1", got)
	}
	if _, updated := taskRepo.updatedStates[row.TaskID]; updated {
		t.Errorf("pending notice task state was updated to %q", taskRepo.updatedStates[row.TaskID])
	}
	messages, err := taskSvc.ListMessages(ctx, row.SessionID)
	if err != nil {
		t.Fatalf("list retry messages: %v", err)
	}
	if len(retryingMessages(t, taskSvc)) != 1 || len(messages) != 1 {
		t.Errorf("pending remote retry episode changed during startup: messages=%#v", messages)
	}
	if len(messages) > 0 && messages[0].Metadata["recovery_disposition"] == "restart_interrupted" {
		t.Errorf("pending remote episode was converted into manual interruption: %#v", messages[0].Metadata)
	}
	agentManager := svc.agentManager.(*mockAgentManager)
	if len(agentManager.capturedPrompts) != 0 || len(agentManager.stopAgentWithReasonArgs) != 0 {
		t.Errorf("pending remote startup dispatched work or stopped the owner: prompts=%#v stops=%#v",
			agentManager.capturedPrompts, agentManager.stopAgentWithReasonArgs)
	}
}

func cloneTaskMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return nil
	}
	clone := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		clone[key] = value
	}
	return clone
}
