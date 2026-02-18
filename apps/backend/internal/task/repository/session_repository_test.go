package repository

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TaskSession CRUD tests

func TestSQLiteRepository_TaskSessionCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workflow and task first (required for foreign key constraints)
	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task := &models.Task{ID: "task-123", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)

	// Create agent session
	session := &models.TaskSession{
		TaskID:           "task-123",
		AgentExecutionID: "execution-abc",
		ContainerID:      "container-xyz",
		AgentProfileID:   "profile-123",
		ExecutorID:       "executor-1",
		EnvironmentID:    "env-1",
		State:            models.TaskSessionStateStarting,
		Metadata:         map[string]interface{}{"key": "value"},
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create agent session: %v", err)
	}
	if session.ID == "" {
		t.Error("expected session ID to be set")
	}
	if session.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
	if session.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}

	// Get agent session
	retrieved, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to get agent session: %v", err)
	}
	if retrieved.TaskID != "task-123" {
		t.Errorf("expected TaskID 'task-123', got %s", retrieved.TaskID)
	}
	if retrieved.AgentProfileID != "profile-123" {
		t.Errorf("expected AgentProfileID 'profile-123', got %s", retrieved.AgentProfileID)
	}
	if retrieved.State != models.TaskSessionStateStarting {
		t.Errorf("expected state 'starting', got %s", retrieved.State)
	}
	if retrieved.Metadata["key"] != "value" {
		t.Errorf("expected metadata key 'value', got %v", retrieved.Metadata["key"])
	}

	// Update agent session
	session.State = models.TaskSessionStateRunning
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update agent session: %v", err)
	}
	retrieved, _ = repo.GetTaskSession(ctx, session.ID)
	if retrieved.State != models.TaskSessionStateRunning {
		t.Errorf("expected state 'running', got %s", retrieved.State)
	}

	// Delete agent session
	if err := repo.DeleteTaskSession(ctx, session.ID); err != nil {
		t.Fatalf("failed to delete agent session: %v", err)
	}
	_, err = repo.GetTaskSession(ctx, session.ID)
	if err == nil {
		t.Error("expected agent session to be deleted")
	}
}

func TestSQLiteRepository_TaskSessionNotFound(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_, err := repo.GetTaskSession(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent agent session")
	}

	err = repo.UpdateTaskSession(ctx, &models.TaskSession{ID: "nonexistent", TaskID: "task-123"})
	if err == nil {
		t.Error("expected error for updating nonexistent agent session")
	}

	err = repo.DeleteTaskSession(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent agent session")
	}
}

func TestSQLiteRepository_TaskSessionByTaskID(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workflow and task
	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task := &models.Task{ID: "task-123", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)

	// Create multiple sessions for the same task (simulating session history)
	session1 := &models.TaskSession{
		ID:             "session-1",
		TaskID:         "task-123",
		AgentProfileID: "profile-1",
		State:          models.TaskSessionStateCompleted,
	}
	_ = repo.CreateTaskSession(ctx, session1)

	session2 := &models.TaskSession{
		ID:             "session-2",
		TaskID:         "task-123",
		AgentProfileID: "profile-2",
		State:          models.TaskSessionStateRunning,
	}
	_ = repo.CreateTaskSession(ctx, session2)

	// GetTaskSessionByTaskID should return the most recent session
	retrieved, err := repo.GetTaskSessionByTaskID(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to get agent session by task ID: %v", err)
	}
	if retrieved.ID != "session-2" {
		t.Errorf("expected session-2 (most recent), got %s", retrieved.ID)
	}

	// GetActiveTaskSessionByTaskID should return the active session
	active, err := repo.GetActiveTaskSessionByTaskID(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to get active agent session by task ID: %v", err)
	}
	if active.ID != "session-2" {
		t.Errorf("expected session-2 (active), got %s", active.ID)
	}
	if active.State != models.TaskSessionStateRunning {
		t.Errorf("expected state 'running', got %s", active.State)
	}

	// Test when no active session exists
	session2.State = models.TaskSessionStateCompleted
	_ = repo.UpdateTaskSession(ctx, session2)

	_, err = repo.GetActiveTaskSessionByTaskID(ctx, "task-123")
	if err == nil {
		t.Error("expected error when no active session exists")
	}

	// Test for nonexistent task
	_, err = repo.GetTaskSessionByTaskID(ctx, "nonexistent-task")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestSQLiteRepository_ListTaskSessions(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workflow and tasks
	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task1 := &models.Task{ID: "task-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Task 1"}
	_ = repo.CreateTask(ctx, task1)
	task2 := &models.Task{ID: "task-2", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Task 2"}
	_ = repo.CreateTask(ctx, task2)

	// Create sessions for different tasks
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-1", TaskID: "task-1", AgentProfileID: "profile-1", State: models.TaskSessionStateCompleted})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-2", TaskID: "task-1", AgentProfileID: "profile-1", State: models.TaskSessionStateRunning})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-3", TaskID: "task-2", AgentProfileID: "profile-2", State: models.TaskSessionStateStarting})

	// List sessions for task-1
	sessions, err := repo.ListTaskSessions(ctx, "task-1")
	if err != nil {
		t.Fatalf("failed to list agent sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions for task-1, got %d", len(sessions))
	}

	// List all active sessions
	activeSessions, err := repo.ListActiveTaskSessions(ctx)
	if err != nil {
		t.Fatalf("failed to list active agent sessions: %v", err)
	}
	if len(activeSessions) != 2 {
		t.Errorf("expected 2 active sessions, got %d", len(activeSessions))
	}

	// Verify only active statuses are returned
	for _, s := range activeSessions {
		if s.State != models.TaskSessionStateStarting && s.State != models.TaskSessionStateRunning && s.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("expected active state, got %s", s.State)
		}
	}
}

func TestSQLiteRepository_UpdateTaskSessionState(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workflow and task
	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task := &models.Task{ID: "task-123", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)

	// Create an agent session
	session := &models.TaskSession{
		ID:             "session-123",
		TaskID:         "task-123",
		AgentProfileID: "profile-1",
		State:          models.TaskSessionStateStarting,
	}
	_ = repo.CreateTaskSession(ctx, session)

	// Update to running status
	err := repo.UpdateTaskSessionState(ctx, "session-123", models.TaskSessionStateRunning, "")
	if err != nil {
		t.Fatalf("failed to update agent session status: %v", err)
	}
	retrieved, _ := repo.GetTaskSession(ctx, "session-123")
	if retrieved.State != models.TaskSessionStateRunning {
		t.Errorf("expected state 'running', got %s", retrieved.State)
	}
	if retrieved.CompletedAt != nil {
		t.Error("expected CompletedAt to be nil for running status")
	}

	// Update to completed status (should set CompletedAt)
	err = repo.UpdateTaskSessionState(ctx, "session-123", models.TaskSessionStateCompleted, "")
	if err != nil {
		t.Fatalf("failed to update agent session status to completed: %v", err)
	}
	retrieved, _ = repo.GetTaskSession(ctx, "session-123")
	if retrieved.State != models.TaskSessionStateCompleted {
		t.Errorf("expected state 'completed', got %s", retrieved.State)
	}
	if retrieved.CompletedAt == nil {
		t.Error("expected CompletedAt to be set for completed status")
	}

	// Test failed status with error message
	session2 := &models.TaskSession{
		ID:             "session-456",
		TaskID:         "task-123",
		AgentProfileID: "profile-1",
		State:          models.TaskSessionStateRunning,
	}
	_ = repo.CreateTaskSession(ctx, session2)

	err = repo.UpdateTaskSessionState(ctx, "session-456", models.TaskSessionStateFailed, "connection timeout")
	if err != nil {
		t.Fatalf("failed to update agent session status to failed: %v", err)
	}
	retrieved, _ = repo.GetTaskSession(ctx, "session-456")
	if retrieved.State != models.TaskSessionStateFailed {
		t.Errorf("expected state 'failed', got %s", retrieved.State)
	}
	if retrieved.ErrorMessage != "connection timeout" {
		t.Errorf("expected error message 'connection timeout', got %s", retrieved.ErrorMessage)
	}
	if retrieved.CompletedAt == nil {
		t.Error("expected CompletedAt to be set for failed status")
	}

	// Test stopped status
	session3 := &models.TaskSession{
		ID:             "session-789",
		TaskID:         "task-123",
		AgentProfileID: "profile-1",
		State:          models.TaskSessionStateRunning,
	}
	_ = repo.CreateTaskSession(ctx, session3)

	err = repo.UpdateTaskSessionState(ctx, "session-789", models.TaskSessionStateCancelled, "")
	if err != nil {
		t.Fatalf("failed to update agent session status to stopped: %v", err)
	}
	retrieved, _ = repo.GetTaskSession(ctx, "session-789")
	if retrieved.State != models.TaskSessionStateCancelled {
		t.Errorf("expected state 'cancelled', got %s", retrieved.State)
	}
	if retrieved.CompletedAt == nil {
		t.Error("expected CompletedAt to be set for stopped status")
	}

	// Test nonexistent session
	err = repo.UpdateTaskSessionState(ctx, "nonexistent", models.TaskSessionStateRunning, "")
	if err == nil {
		t.Error("expected error for updating nonexistent session status")
	}
}

func TestSQLiteRepository_CompleteRunningToolCallsForTurn(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Setup
	workflow := &models.Workflow{ID: "wf-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task := &models.Task{ID: "task-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	sessionID := setupSQLiteTestSession(ctx, repo, task.ID, "session-1")
	turnID := setupSQLiteTestTurn(ctx, repo, sessionID, task.ID, "turn-1")

	// Create a tool call message with status "running"
	runningTool := &models.Message{
		ID: "msg-running-1", TaskSessionID: sessionID, TaskID: task.ID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent, Content: "Running tool",
		Type:     models.MessageTypeToolCall,
		Metadata: map[string]interface{}{"tool_call_id": "tc-1", "status": "running"},
	}
	// Create a tool call message already "complete"
	completeTool := &models.Message{
		ID: "msg-complete-1", TaskSessionID: sessionID, TaskID: task.ID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent, Content: "Complete tool",
		Type:     models.MessageTypeToolCall,
		Metadata: map[string]interface{}{"tool_call_id": "tc-2", "status": "complete"},
	}
	// Create a regular message (no tool_call_id) with status "running" — should NOT be affected
	regularMsg := &models.Message{
		ID: "msg-regular-1", TaskSessionID: sessionID, TaskID: task.ID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent, Content: "Regular message",
		Type:     models.MessageTypeMessage,
		Metadata: map[string]interface{}{"status": "running"},
	}
	// Create a second running tool call
	runningTool2 := &models.Message{
		ID: "msg-running-2", TaskSessionID: sessionID, TaskID: task.ID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent, Content: "Running tool 2",
		Type:     models.MessageTypeToolCall,
		Metadata: map[string]interface{}{"tool_call_id": "tc-3", "status": "running"},
	}

	for _, msg := range []*models.Message{runningTool, completeTool, regularMsg, runningTool2} {
		if err := repo.CreateMessage(ctx, msg); err != nil {
			t.Fatalf("failed to create message %s: %v", msg.ID, err)
		}
	}

	// Execute
	affected, err := repo.CompleteRunningToolCallsForTurn(ctx, turnID)
	if err != nil {
		t.Fatalf("CompleteRunningToolCallsForTurn failed: %v", err)
	}

	// Should have updated exactly 2 running tool call messages
	if affected != 2 {
		t.Errorf("expected 2 affected rows, got %d", affected)
	}

	// Verify running tool calls are now "complete"
	msg1, _ := repo.GetMessage(ctx, "msg-running-1")
	if msg1.Metadata["status"] != "complete" {
		t.Errorf("expected msg-running-1 status 'complete', got %v", msg1.Metadata["status"])
	}
	msg2, _ := repo.GetMessage(ctx, "msg-running-2")
	if msg2.Metadata["status"] != "complete" {
		t.Errorf("expected msg-running-2 status 'complete', got %v", msg2.Metadata["status"])
	}

	// Verify already-complete tool call is unchanged
	msg3, _ := repo.GetMessage(ctx, "msg-complete-1")
	if msg3.Metadata["status"] != "complete" {
		t.Errorf("expected msg-complete-1 status 'complete', got %v", msg3.Metadata["status"])
	}

	// Verify regular message (no tool_call_id) was NOT affected
	msg4, _ := repo.GetMessage(ctx, "msg-regular-1")
	if msg4.Metadata["status"] != "running" {
		t.Errorf("expected msg-regular-1 status 'running' (unchanged), got %v", msg4.Metadata["status"])
	}

	// Running again should affect 0 rows
	affected2, err := repo.CompleteRunningToolCallsForTurn(ctx, turnID)
	if err != nil {
		t.Fatalf("second CompleteRunningToolCallsForTurn failed: %v", err)
	}
	if affected2 != 0 {
		t.Errorf("expected 0 affected rows on second call, got %d", affected2)
	}
}
