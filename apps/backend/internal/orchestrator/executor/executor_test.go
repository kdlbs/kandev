package executor

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Tests

func TestPrepareSession_Success(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Title:       "Test Task",
		Description: "Test description",
	}

	sessionID, err := executor.PrepareSession(context.Background(), task, "profile-123", "executor-123", "", "step-123")
	if err != nil {
		t.Fatalf("PrepareSession failed: %v", err)
	}

	if sessionID == "" {
		t.Error("Expected non-empty session ID")
	}

	// Verify session was created
	if len(repo.createTaskSessionCalls) != 1 {
		t.Errorf("Expected 1 CreateTaskSession call, got %d", len(repo.createTaskSessionCalls))
	}

	createdSession := repo.createTaskSessionCalls[0]
	if createdSession.TaskID != task.ID {
		t.Errorf("Expected task ID %s, got %s", task.ID, createdSession.TaskID)
	}
	if createdSession.AgentProfileID != "profile-123" {
		t.Errorf("Expected agent profile ID profile-123, got %s", createdSession.AgentProfileID)
	}
	if createdSession.State != models.TaskSessionStateCreated {
		t.Errorf("Expected state CREATED, got %s", createdSession.State)
	}
	if !createdSession.IsPrimary {
		t.Error("Expected session to be primary")
	}

	// Verify SetSessionPrimary was called
	if len(repo.setSessionPrimaryCalls) != 1 {
		t.Errorf("Expected 1 SetSessionPrimary call, got %d", len(repo.setSessionPrimaryCalls))
	}
}

func TestPrepareSession_InvokesPrimarySessionCallback(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{}
	exec := newTestExecutor(t, agentManager, repo)

	var callbackTaskID, callbackSessionID string
	exec.SetOnPrimarySessionSet(func(_ context.Context, taskID, sessionID string) {
		callbackTaskID = taskID
		callbackSessionID = sessionID
	})

	task := &v1.Task{
		ID:          "task-456",
		WorkspaceID: "workspace-456",
		Title:       "Callback Test",
	}

	sessionID, err := exec.PrepareSession(context.Background(), task, "profile-1", "executor-1", "", "step-1")
	if err != nil {
		t.Fatalf("PrepareSession failed: %v", err)
	}

	if callbackTaskID != task.ID {
		t.Errorf("callback taskID = %q, want %q", callbackTaskID, task.ID)
	}
	if callbackSessionID != sessionID {
		t.Errorf("callback sessionID = %q, want %q", callbackSessionID, sessionID)
	}
}

func TestPrepareSession_NoAgentProfileID(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
	}

	_, err := executor.PrepareSession(context.Background(), task, "", "executor-123", "", "step-123")
	if err != ErrNoAgentProfileID {
		t.Errorf("Expected ErrNoAgentProfileID, got %v", err)
	}
}

func TestPrepareSession_WithRepository(t *testing.T) {
	repo := newMockRepository()
	repo.taskRepositories["task-repo-1"] = &models.TaskRepository{
		ID:           "task-repo-1",
		TaskID:       "task-123",
		RepositoryID: "repo-123",
		BaseBranch:   "main",
	}
	repo.repositories["repo-123"] = &models.Repository{
		ID:        "repo-123",
		LocalPath: "/path/to/repo",
	}

	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
	}

	sessionID, err := executor.PrepareSession(context.Background(), task, "profile-123", "", "", "")
	if err != nil {
		t.Fatalf("PrepareSession failed: %v", err)
	}

	if sessionID == "" {
		t.Error("Expected non-empty session ID")
	}

	// Verify session has repository info
	createdSession := repo.createTaskSessionCalls[0]
	if createdSession.RepositoryID != "repo-123" {
		t.Errorf("Expected repository ID repo-123, got %s", createdSession.RepositoryID)
	}
	if createdSession.BaseBranch != "main" {
		t.Errorf("Expected base branch main, got %s", createdSession.BaseBranch)
	}
}

func TestLaunchPreparedSession_Success(t *testing.T) {
	repo := newMockRepository()

	// Pre-create session (as PrepareSession would)
	session := &models.TaskSession{
		ID:             "session-123",
		TaskID:         "task-123",
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateCreated,
		StartedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	repo.sessions[session.ID] = session

	launchCalled := false
	launchedEnvID := ""
	agentManager := &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launchCalled = true
			if req.SessionID != "session-123" {
				t.Errorf("Expected session ID session-123, got %s", req.SessionID)
			}
			if req.TaskID != "task-123" {
				t.Errorf("Expected task ID task-123, got %s", req.TaskID)
			}
			if req.TaskEnvironmentID == "" {
				t.Error("Expected non-empty task environment ID")
			}
			launchedEnvID = req.TaskEnvironmentID
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-123",
				ContainerID:      "container-123",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
	}

	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Title:       "Test Task",
		Description: "Test description",
	}

	execution, err := executor.LaunchPreparedSession(context.Background(), task, "session-123", LaunchOptions{AgentProfileID: "profile-123", Prompt: "test prompt", StartAgent: true})
	if err != nil {
		t.Fatalf("LaunchPreparedSession failed: %v", err)
	}

	if !launchCalled {
		t.Error("Expected LaunchAgent to be called")
	}

	if execution.SessionID != "session-123" {
		t.Errorf("Expected session ID session-123, got %s", execution.SessionID)
	}
	if execution.AgentExecutionID != "exec-123" {
		t.Errorf("Expected agent execution ID exec-123, got %s", execution.AgentExecutionID)
	}
	if execution.SessionState != v1.TaskSessionStateStarting {
		t.Errorf("Expected session state STARTING, got %s", execution.SessionState)
	}
	if session.TaskEnvironmentID != launchedEnvID {
		t.Errorf("Expected session TaskEnvironmentID %q, got %q", launchedEnvID, session.TaskEnvironmentID)
	}
	if len(repo.createTaskEnvironmentCalls) != 1 {
		t.Fatalf("Expected 1 CreateTaskEnvironment call, got %d", len(repo.createTaskEnvironmentCalls))
	}
	if repo.createTaskEnvironmentCalls[0].ID != launchedEnvID {
		t.Errorf("Expected persisted task environment ID %q, got %q", launchedEnvID, repo.createTaskEnvironmentCalls[0].ID)
	}
}

func TestAssignLaunchTaskEnvironmentID(t *testing.T) {
	t.Run("reuses existing task environment", func(t *testing.T) {
		session := &models.TaskSession{ID: "session-1"}
		assignLaunchTaskEnvironmentID(session, &models.TaskEnvironment{ID: "env-existing"})

		if session.TaskEnvironmentID != "env-existing" {
			t.Errorf("TaskEnvironmentID = %q, want env-existing", session.TaskEnvironmentID)
		}
	})

	t.Run("allocates id for new task environment", func(t *testing.T) {
		session := &models.TaskSession{ID: "session-1"}
		assignLaunchTaskEnvironmentID(session, nil)

		if session.TaskEnvironmentID == "" {
			t.Fatal("expected non-empty TaskEnvironmentID")
		}
	})
}

func TestLaunchPreparedSession_SessionNotBelongsToTask(t *testing.T) {
	repo := newMockRepository()

	// Pre-create session with different task ID
	session := &models.TaskSession{
		ID:             "session-123",
		TaskID:         "other-task",
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateCreated,
	}
	repo.sessions[session.ID] = session

	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
	}

	_, err := executor.LaunchPreparedSession(context.Background(), task, "session-123", LaunchOptions{AgentProfileID: "profile-123", Prompt: "test prompt", StartAgent: true})
	if err == nil {
		t.Error("Expected error when session doesn't belong to task")
	}
}

func TestLaunchPreparedSession_WorkspaceOnly(t *testing.T) {
	repo := newMockRepository()

	// Pre-create session (as PrepareSession would)
	session := &models.TaskSession{
		ID:             "session-123",
		TaskID:         "task-123",
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateCreated,
		StartedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	repo.sessions[session.ID] = session

	launchCalled := false
	startAgentCalled := false
	agentManager := &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launchCalled = true
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-123",
				ContainerID:      "container-123",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
		startAgentProcessFunc: func(ctx context.Context, id string) error {
			startAgentCalled = true
			return nil
		},
	}

	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Title:       "Test Task",
		Description: "Test description",
	}

	// startAgent=false: should launch workspace but NOT start agent
	execution, err := executor.LaunchPreparedSession(context.Background(), task, "session-123", LaunchOptions{AgentProfileID: "profile-123", StartAgent: false})
	if err != nil {
		t.Fatalf("LaunchPreparedSession(startAgent=false) failed: %v", err)
	}

	if !launchCalled {
		t.Error("Expected LaunchAgent to be called (workspace setup)")
	}

	// Give goroutines a moment to run (there shouldn't be any)
	time.Sleep(50 * time.Millisecond)

	if startAgentCalled {
		t.Error("Expected StartAgentProcess NOT to be called when startAgent=false")
	}

	if execution.SessionState != v1.TaskSessionStateCreated {
		t.Errorf("Expected session state CREATED, got %s", execution.SessionState)
	}

	// Session in DB should remain CREATED
	updatedSession := repo.sessions["session-123"]
	if updatedSession.State != models.TaskSessionStateCreated {
		t.Errorf("Expected DB session state CREATED, got %s", updatedSession.State)
	}
}

func TestLaunchPreparedSession_ExistingWorkspace_StartAgent(t *testing.T) {
	repo := newMockRepository()

	// Session has an existing executors_running row (workspace previously launched).
	// Post-refactor, "is launched" is gauged by HasExecutorRunningRow rather than
	// session.AgentExecutionID — the column was removed from task_sessions.
	session := &models.TaskSession{
		ID:             "session-123",
		TaskID:         "task-123",
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateCreated,
		StartedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	repo.sessions[session.ID] = session
	repo.executorsRunning[session.ID] = &models.ExecutorRunning{
		ID:               session.ID,
		SessionID:        session.ID,
		TaskID:           session.TaskID,
		AgentExecutionID: "exec-existing",
		Status:           "ready",
	}

	var startAgentCalled atomic.Bool
	descriptionSet := ""
	agentManager := &mockAgentManager{
		startAgentProcessFunc: func(ctx context.Context, id string) error {
			startAgentCalled.Store(true)
			if id != "exec-existing" {
				t.Errorf("Expected execution ID exec-existing, got %s", id)
			}
			return nil
		},
		// The execution must exist in the in-memory store for startAgentOnExistingWorkspace to proceed.
		getExecutionIDForSessionFunc: func(ctx context.Context, sessionID string) (string, error) {
			if sessionID == "session-123" {
				return "exec-existing", nil
			}
			return "", fmt.Errorf("not found")
		},
	}
	agentManager.setExecutionDescriptionFunc = func(ctx context.Context, id, desc string) error {
		descriptionSet = desc
		return nil
	}

	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Title:       "Test Task",
	}

	execution, err := executor.LaunchPreparedSession(context.Background(), task, "session-123", LaunchOptions{AgentProfileID: "profile-123", Prompt: "build the feature", StartAgent: true})
	if err != nil {
		t.Fatalf("LaunchPreparedSession(existing workspace) failed: %v", err)
	}

	// Should use the existing execution ID
	if execution.AgentExecutionID != "exec-existing" {
		t.Errorf("Expected agent execution ID exec-existing, got %s", execution.AgentExecutionID)
	}

	if execution.SessionState != v1.TaskSessionStateStarting {
		t.Errorf("Expected session state STARTING, got %s", execution.SessionState)
	}

	// Description should have been set
	if descriptionSet != "build the feature" {
		t.Errorf("Expected description 'build the feature', got %q", descriptionSet)
	}

	// Wait for async goroutine
	time.Sleep(100 * time.Millisecond)

	if !startAgentCalled.Load() {
		t.Error("Expected StartAgentProcess to be called")
	}
}

// TestLaunchPreparedSession_StaleExecutionID_CorrectedFromLiveStore was removed:
// the "DB has stale ID, in-memory has live ID, correct DB to match" code path no
// longer exists. With executors_running as the single source of truth and lifecycle-
// owned writes, the divergence the test was guarding against is structurally
// impossible. See persistence.go in the lifecycle package for the new ownership
// model. Replaced by TestLaunch_RaceProducesSingleExecution and the recovery
// reconciliation test in lifecycle/.

func TestLaunchPreparedSession_StaleExecution_FallsThroughToLaunchAgent(t *testing.T) {
	repo := newMockRepository()

	// Session has an executors_running row from a previous backend run, but no
	// live execution in memory (e.g. backend restarted since workspace was
	// prepared). HasExecutorRunningRow is true → fast path is tried;
	// GetExecutionIDForSession returns empty → ErrStaleExecution → fall through
	// to the full LaunchAgent path.
	session := &models.TaskSession{
		ID:             "session-123",
		TaskID:         "task-123",
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateCreated,
		StartedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	repo.sessions[session.ID] = session
	repo.executorsRunning[session.ID] = &models.ExecutorRunning{
		ID:               session.ID,
		SessionID:        session.ID,
		TaskID:           session.TaskID,
		AgentExecutionID: "stale-exec-id",
		Status:           "ready",
	}

	var launchCalled atomic.Bool
	agentManager := &mockAgentManager{
		// No live execution — simulates backend restart
		getExecutionIDForSessionFunc: func(ctx context.Context, sessionID string) (string, error) {
			return "", fmt.Errorf("no execution found for session %s", sessionID)
		},
		launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launchCalled.Store(true)
			// Simulate the lifecycle manager's persistExecutorRunning by upserting the row.
			repo.executorsRunning[req.SessionID] = &models.ExecutorRunning{
				ID:               req.SessionID,
				SessionID:        req.SessionID,
				TaskID:           req.TaskID,
				AgentExecutionID: "new-exec-id",
				Status:           "starting",
			}
			return &LaunchAgentResponse{
				AgentExecutionID: "new-exec-id",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
	}

	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Title:       "Test Task",
	}

	execution, err := executor.LaunchPreparedSession(context.Background(), task, "session-123", LaunchOptions{
		AgentProfileID: "profile-123",
		Prompt:         "review this PR",
		StartAgent:     true,
	})
	if err != nil {
		t.Fatalf("LaunchPreparedSession should fall through to LaunchAgent, got error: %v", err)
	}

	if !launchCalled.Load() {
		t.Error("Expected LaunchAgent to be called after stale execution fallthrough")
	}

	if execution.AgentExecutionID != "new-exec-id" {
		t.Errorf("Expected new execution ID 'new-exec-id', got %s", execution.AgentExecutionID)
	}

	// executors_running should now hold the new ID (lifecycle manager owns this
	// write; the test simulates that via launchAgentFunc above).
	updatedRunning := repo.executorsRunning["session-123"]
	if updatedRunning == nil || updatedRunning.AgentExecutionID != "new-exec-id" {
		got := ""
		if updatedRunning != nil {
			got = updatedRunning.AgentExecutionID
		}
		t.Errorf("Expected executors_running.agent_execution_id to be 'new-exec-id', got %q", got)
	}
}

func TestExecuteWithProfile_UsesPrepareThenLaunch(t *testing.T) {
	repo := newMockRepository()

	launchCalled := false
	agentManager := &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launchCalled = true
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-123",
				ContainerID:      "container-123",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
	}

	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Title:       "Test Task",
		Description: "Test description",
	}

	execution, err := executor.ExecuteWithProfile(context.Background(), task, "profile-123", "", "test prompt", "")
	if err != nil {
		t.Fatalf("ExecuteWithProfile failed: %v", err)
	}

	// Verify session was created (PrepareSession was called)
	if len(repo.createTaskSessionCalls) != 1 {
		t.Errorf("Expected 1 CreateTaskSession call (from PrepareSession), got %d", len(repo.createTaskSessionCalls))
	}

	// Verify agent was launched (LaunchPreparedSession was called)
	if !launchCalled {
		t.Error("Expected LaunchAgent to be called (from LaunchPreparedSession)")
	}

	if execution.TaskID != task.ID {
		t.Errorf("Expected task ID %s, got %s", task.ID, execution.TaskID)
	}
}

func TestShouldUseWorktree(t *testing.T) {
	tests := []struct {
		executorType string
		want         bool
	}{
		{"worktree", true},
		{"local", false},
		{"local_docker", false},
		{"remote_docker", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := shouldUseWorktree(tt.executorType); got != tt.want {
			t.Errorf("shouldUseWorktree(%q) = %v, want %v", tt.executorType, got, tt.want)
		}
	}
}

func TestApplyPreferredShellEnv(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)

	t.Run("local executor injects shell env", func(t *testing.T) {
		got := executor.applyPreferredShellEnv(context.Background(), string(models.ExecutorTypeLocal), map[string]string{})
		if got["AGENTCTL_SHELL_COMMAND"] != "/bin/bash" {
			t.Fatalf("expected AGENTCTL_SHELL_COMMAND=/bin/bash, got %q", got["AGENTCTL_SHELL_COMMAND"])
		}
		if got["SHELL"] != "/bin/bash" {
			t.Fatalf("expected SHELL=/bin/bash, got %q", got["SHELL"])
		}
	})

	t.Run("sprites executor does not inject shell env", func(t *testing.T) {
		got := executor.applyPreferredShellEnv(context.Background(), string(models.ExecutorTypeSprites), map[string]string{})
		if _, ok := got["AGENTCTL_SHELL_COMMAND"]; ok {
			t.Fatal("did not expect AGENTCTL_SHELL_COMMAND for sprites executor")
		}
		if _, ok := got["SHELL"]; ok {
			t.Fatal("did not expect SHELL for sprites executor")
		}
	})
}

func TestRunAgentProcessAsync_CleansUpOnStartFailure(t *testing.T) {
	repo := newMockRepository()

	// Pre-create session so state updates work
	repo.sessions["session-123"] = &models.TaskSession{
		ID:     "session-123",
		TaskID: "task-123",
		State:  models.TaskSessionStateStarting,
	}

	var stopCalled atomic.Bool
	var stopForce atomic.Bool
	var stoppedExecutionID atomic.Value

	agentManager := &mockAgentManager{
		startAgentProcessFunc: func(ctx context.Context, agentExecutionID string) error {
			return fmt.Errorf("ACP initialize handshake failed: context deadline exceeded")
		},
		stopAgentFunc: func(ctx context.Context, agentExecutionID string, force bool) error {
			stopCalled.Store(true)
			stopForce.Store(force)
			stoppedExecutionID.Store(agentExecutionID)
			return nil
		},
	}

	exec := newTestExecutor(t, agentManager, repo)

	done := make(chan struct{})
	exec.SetOnSessionStateChange(func(ctx context.Context, taskID, sessionID string, state models.TaskSessionState, errorMessage string) error {
		return repo.UpdateTaskSessionState(ctx, sessionID, state, errorMessage)
	})
	exec.SetOnTaskStateChange(func(ctx context.Context, taskID string, state v1.TaskState) error {
		return repo.UpdateTaskState(ctx, taskID, state)
	})

	// Use runAgentProcessAsync with a no-op onSuccess that should never be called.
	// escalateTaskOnFailure=true mirrors the fresh-start path.
	exec.runAgentProcessAsync(context.Background(), "task-123", "session-123", "exec-456", func(ctx context.Context) {
		t.Error("onSuccess should not be called when StartAgentProcess fails")
		close(done)
	}, true, false)

	// Wait for the async goroutine to finish
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for StopAgent to be called")
		case <-tick.C:
			if stopCalled.Load() {
				goto verified
			}
		}
	}

verified:
	if !stopForce.Load() {
		t.Error("expected StopAgent to be called with force=true")
	}
	if id, ok := stoppedExecutionID.Load().(string); !ok || id != "exec-456" {
		t.Errorf("expected StopAgent called with execution ID exec-456, got %v", stoppedExecutionID.Load())
	}

	// Verify session was marked as FAILED
	session := repo.sessions["session-123"]
	if session.State != models.TaskSessionStateFailed {
		t.Errorf("expected session state FAILED, got %s", session.State)
	}
}

// runAgentProcessAsyncFailureFixture builds an Executor configured to fail
// StartAgentProcess, with task/session-state-change recorders for assertions.
// stopCh is closed when StopAgent is invoked — since the failure path calls
// StopAgent last, blocking on stopCh provides a happens-before barrier for all
// other recorded fields, removing the need for atomicity or polling.
type runAgentProcessAsyncFailureFixture struct {
	exec              *Executor
	taskStateUpdates  []string
	sessionFailedSeen bool
	startFailedCalls  int
	lastFromResume    bool
	stopCh            chan struct{}
}

func newRunAgentProcessAsyncFailureFixture(t *testing.T) *runAgentProcessAsyncFailureFixture {
	t.Helper()
	repo := newMockRepository()
	repo.sessions["session-123"] = &models.TaskSession{
		ID: "session-123", TaskID: "task-123", State: models.TaskSessionStateStarting,
	}
	f := &runAgentProcessAsyncFailureFixture{stopCh: make(chan struct{})}
	agentManager := &mockAgentManager{
		startAgentProcessFunc: func(ctx context.Context, agentExecutionID string) error {
			return fmt.Errorf("ACP initialize handshake failed: context deadline exceeded")
		},
		stopAgentFunc: func(ctx context.Context, agentExecutionID string, force bool) error {
			close(f.stopCh)
			return nil
		},
	}
	f.exec = newTestExecutor(t, agentManager, repo)
	f.exec.SetOnTaskStateChange(func(ctx context.Context, taskID string, state v1.TaskState) error {
		f.taskStateUpdates = append(f.taskStateUpdates, string(state))
		return nil
	})
	f.exec.SetOnSessionStateChange(func(ctx context.Context, taskID, sessionID string, state models.TaskSessionState, errorMessage string) error {
		if state == models.TaskSessionStateFailed {
			f.sessionFailedSeen = true
		}
		return repo.UpdateTaskSessionState(ctx, sessionID, state, errorMessage)
	})
	f.exec.SetOnAgentStartFailed(func(ctx context.Context, taskID, sessionID, agentExecutionID string, err error, fromResume bool) bool {
		f.startFailedCalls++
		f.lastFromResume = fromResume
		return false
	})
	return f
}

// awaitStop blocks until the failure-path goroutine calls StopAgent.
// Closing stopCh is the last side-effect, so all other field writes are
// guaranteed visible by the channel-receive happens-before edge.
func (f *runAgentProcessAsyncFailureFixture) awaitStop(t *testing.T) {
	t.Helper()
	select {
	case <-f.stopCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for StopAgent to be called")
	}
}

func TestRunAgentProcessAsync_ResumeDoesNotEscalateTaskState(t *testing.T) {
	f := newRunAgentProcessAsyncFailureFixture(t)
	f.exec.runAgentProcessAsync(context.Background(), "task-123", "session-123", "exec-456",
		func(ctx context.Context) { t.Error("onSuccess should not run on failure") },
		false, true) // resume path: no escalation, fromResume=true
	f.awaitStop(t)
	if !f.sessionFailedSeen {
		t.Error("expected session state FAILED")
	}
	if len(f.taskStateUpdates) != 0 {
		t.Errorf("expected no task state updates on resume failure, got %v", f.taskStateUpdates)
	}
	if f.startFailedCalls != 1 {
		t.Errorf("expected onAgentStartFailed called once, got %d", f.startFailedCalls)
	}
	if !f.lastFromResume {
		t.Error("expected fromResume=true to be propagated to onAgentStartFailed")
	}
}

func TestRunAgentProcessAsync_FreshStartEscalatesTaskState(t *testing.T) {
	f := newRunAgentProcessAsyncFailureFixture(t)
	f.exec.runAgentProcessAsync(context.Background(), "task-123", "session-123", "exec-456",
		func(ctx context.Context) { t.Error("onSuccess should not run on failure") },
		true, false) // fresh-start path: escalate, fromResume=false
	f.awaitStop(t)
	if !f.sessionFailedSeen {
		t.Error("expected session state FAILED")
	}
	if len(f.taskStateUpdates) != 1 || f.taskStateUpdates[0] != string(v1.TaskStateFailed) {
		t.Errorf("expected fresh-start failure to set task state FAILED, got %v", f.taskStateUpdates)
	}
	if f.lastFromResume {
		t.Error("expected fromResume=false on fresh-start path")
	}
}

func TestRepositoryCloneURL(t *testing.T) {
	tests := []struct {
		name string
		repo *models.Repository
		want string
	}{
		{
			name: "github repo",
			repo: &models.Repository{Provider: "github", ProviderOwner: "acme", ProviderName: "app"},
			want: "https://github.com/acme/app.git",
		},
		{
			name: "gitlab repo",
			repo: &models.Repository{Provider: "gitlab", ProviderOwner: "acme", ProviderName: "app"},
			want: "https://gitlab.com/acme/app.git",
		},
		{
			name: "bitbucket repo",
			repo: &models.Repository{Provider: "bitbucket", ProviderOwner: "acme", ProviderName: "app"},
			want: "https://bitbucket.org/acme/app.git",
		},
		{
			name: "unknown provider returns empty",
			repo: &models.Repository{Provider: "custom", ProviderOwner: "acme", ProviderName: "app"},
			want: "",
		},
		{
			name: "empty provider defaults to github",
			repo: &models.Repository{ProviderOwner: "acme", ProviderName: "app"},
			want: "https://github.com/acme/app.git",
		},
		{
			name: "missing owner returns empty",
			repo: &models.Repository{Provider: "github", ProviderName: "app"},
			want: "",
		},
		{
			name: "missing name returns empty",
			repo: &models.Repository{Provider: "github", ProviderOwner: "acme"},
			want: "",
		},
		{
			name: "both missing returns empty",
			repo: &models.Repository{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := repositoryCloneURL(tt.repo); got != tt.want {
				t.Errorf("repositoryCloneURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRepositoryCloneURL_LocalPathFallback covers the second code path
// added to support Docker E2E tests: when the repository has no
// ProviderOwner/ProviderName, the function shells out to
// `git -C LocalPath remote get-url origin` and returns the result.
// Without these tests the file://-remote E2E fixture would have no
// regression guard.
func TestRepositoryCloneURL_LocalPathFallback(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	t.Run("local-only repo with valid git remote", func(t *testing.T) {
		dir := t.TempDir()
		mustRunGit(t, dir, "init", "--quiet")
		mustRunGit(t, dir, "remote", "add", "origin", "file:///some/path.git")

		got := repositoryCloneURL(&models.Repository{LocalPath: dir})
		if got != "file:///some/path.git" {
			t.Errorf("got %q, want file:///some/path.git", got)
		}
	})

	t.Run("local-only repo without remote returns empty", func(t *testing.T) {
		dir := t.TempDir()
		mustRunGit(t, dir, "init", "--quiet")

		if got := repositoryCloneURL(&models.Repository{LocalPath: dir}); got != "" {
			t.Errorf("repo without origin remote should return empty, got %q", got)
		}
	})

	t.Run("local-only non-git dir returns empty", func(t *testing.T) {
		if got := repositoryCloneURL(&models.Repository{LocalPath: t.TempDir()}); got != "" {
			t.Errorf("non-git dir should return empty, got %q", got)
		}
	})

	t.Run("no provider and no local path returns empty", func(t *testing.T) {
		if got := repositoryCloneURL(&models.Repository{}); got != "" {
			t.Errorf("empty repository should return empty, got %q", got)
		}
	})
}

// mustRunGit fails the test if `git -C dir <args...>` exits non-zero.
// Kept tiny on purpose — only used by the LocalPath-fallback tests.
func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestPersistResumeState_SetsStartingState(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)
	now := time.Now().UTC()
	completedAt := now.Add(-time.Hour)

	t.Run("sets STARTING when startAgent is true", func(t *testing.T) {
		session := &models.TaskSession{
			ID:          "session-1",
			TaskID:      "task-1",
			State:       models.TaskSessionStateWaitingForInput,
			CompletedAt: &completedAt,
			UpdatedAt:   now,
		}
		repo.sessions[session.ID] = session

		executor.persistResumeState(context.Background(), "task-1", session, true)

		if session.State != models.TaskSessionStateStarting {
			t.Errorf("expected state STARTING, got %s", session.State)
		}
		if session.CompletedAt != nil {
			t.Error("expected CompletedAt to be nil")
		}
	})

	t.Run("does not change state when startAgent is false", func(t *testing.T) {
		session := &models.TaskSession{
			ID:        "session-2",
			TaskID:    "task-1",
			State:     models.TaskSessionStateWaitingForInput,
			UpdatedAt: now,
		}
		repo.sessions[session.ID] = session

		executor.persistResumeState(context.Background(), "task-1", session, false)

		if session.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("expected state WAITING_FOR_INPUT, got %s", session.State)
		}
	})
}

// Regression: PrepareTaskSession launches the workspace in a background
// goroutine while StartTaskWithSession runs a foreground launch when the
// agent is started. Both call LaunchPreparedSession on the same session.
// Without per-session serialisation the two launches both reach
// agentManager.LaunchAgent and the second one errors with
// "race resolved during register" after running env prep — surfacing as
// "Environment setup failed" in the UI. Multi-repo amplifies the window
// because env prep runs sequentially per repo.
func TestLaunchPreparedSession_SerialisesConcurrentLaunches(t *testing.T) {
	repo := newMockRepository()
	session := &models.TaskSession{
		ID:             "session-race",
		TaskID:         "task-race",
		AgentProfileID: "profile-1",
		State:          models.TaskSessionStateCreated,
		StartedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	repo.sessions[session.ID] = session

	// `entered` fires every time LaunchAgent is hit. `gate` blocks the
	// caller until the test releases it, keeping the first launch holding
	// the per-session lock so a parallel call would otherwise race in.
	// Channel-based coordination per CLAUDE.md (no time.Sleep).
	var launchCount int64
	entered := make(chan struct{}, 2)
	gate := make(chan struct{})
	agentManager := &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			atomic.AddInt64(&launchCount, 1)
			entered <- struct{}{}
			<-gate
			// Simulate the lifecycle manager's persistExecutorRunning: the row
			// must exist after the first launch so the second caller's
			// HasExecutorRunningRow check returns true and routes to the
			// fast path (startAgentOnExistingWorkspace) instead of launching again.
			repo.executorsRunning[req.SessionID] = &models.ExecutorRunning{
				ID:               req.SessionID,
				SessionID:        req.SessionID,
				TaskID:           req.TaskID,
				AgentExecutionID: "exec-race",
				Status:           "starting",
			}
			return &LaunchAgentResponse{AgentExecutionID: "exec-race", Status: v1.AgentStatusStarting}, nil
		},
		// Fast path lookup must succeed for the second caller; mirror what
		// the live store would return after the first caller registered.
		getExecutionIDForSessionFunc: func(ctx context.Context, sessionID string) (string, error) {
			return "exec-race", nil
		},
	}
	executor := newTestExecutor(t, agentManager, repo)
	task := &v1.Task{ID: "task-race", WorkspaceID: "ws-1"}

	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := executor.LaunchPreparedSession(context.Background(), task, "session-race", LaunchOptions{
				AgentProfileID: "profile-1",
				StartAgent:     true,
			})
			results[idx] = err
		}(i)
	}

	// Wait for the first launch to enter LaunchAgent and block on the
	// gate. With the per-session lock the second goroutine cannot proceed
	// past the lock acquisition, so a second `entered` send must NOT
	// happen within a bounded window. Without the lock, both goroutines
	// race in well within the timeout (~ms).
	<-entered
	select {
	case <-entered:
		close(gate)
		wg.Wait()
		t.Fatalf("LaunchAgent entered twice — second caller raced past the per-session lock")
	case <-time.After(100 * time.Millisecond):
		// No second entry = the lock is holding the second caller.
	}
	if got := atomic.LoadInt64(&launchCount); got != 1 {
		close(gate)
		wg.Wait()
		t.Fatalf("LaunchAgent call count before gate release = %d, want 1 (lock should have serialised both)", got)
	}
	close(gate)
	wg.Wait()

	// First call ran LaunchAgent; second call took the fast path so total
	// stays at 1. Both return non-error (the second is a no-op start).
	if got := atomic.LoadInt64(&launchCount); got != 1 {
		t.Errorf("LaunchAgent total calls = %d, want 1", got)
	}
	for i, err := range results {
		if err != nil {
			t.Errorf("results[%d] = %v, want nil", i, err)
		}
	}
}
