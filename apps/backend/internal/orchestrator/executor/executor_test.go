package executor

import (
	"context"
	"fmt"
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
	agentManager := &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launchCalled = true
			if req.SessionID != "session-123" {
				t.Errorf("Expected session ID session-123, got %s", req.SessionID)
			}
			if req.TaskID != "task-123" {
				t.Errorf("Expected task ID task-123, got %s", req.TaskID)
			}
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

	// Session already has an AgentExecutionID (workspace previously launched)
	session := &models.TaskSession{
		ID:               "session-123",
		TaskID:           "task-123",
		AgentProfileID:   "profile-123",
		AgentExecutionID: "exec-existing",
		State:            models.TaskSessionStateCreated,
		StartedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	repo.sessions[session.ID] = session

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

func TestLaunchPreparedSession_StaleExecutionID_CorrectedFromLiveStore(t *testing.T) {
	repo := newMockRepository()

	// Session has a stale AgentExecutionID from a previous backend run.
	// After restart, EnsureWorkspaceExecutionForSession created a new execution
	// with a different ID, but the database still holds the old one.
	session := &models.TaskSession{
		ID:               "session-123",
		TaskID:           "task-123",
		AgentProfileID:   "profile-123",
		AgentExecutionID: "stale-exec-id", // stale ID from DB
		State:            models.TaskSessionStateCreated,
		StartedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	repo.sessions[session.ID] = session

	var startedWithID atomic.Value
	agentManager := &mockAgentManager{
		startAgentProcessFunc: func(ctx context.Context, id string) error {
			startedWithID.Store(id)
			return nil
		},
		// Simulate the live execution store having a different ID
		getExecutionIDForSessionFunc: func(ctx context.Context, sessionID string) (string, error) {
			if sessionID == "session-123" {
				return "live-exec-id", nil
			}
			return "", fmt.Errorf("not found")
		},
	}

	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Title:       "Test Task",
	}

	execution, err := executor.LaunchPreparedSession(context.Background(), task, "session-123", LaunchOptions{AgentProfileID: "profile-123", Prompt: "do work", StartAgent: true})
	if err != nil {
		t.Fatalf("LaunchPreparedSession failed: %v", err)
	}

	// Should use the live execution ID, not the stale one
	if execution.AgentExecutionID != "live-exec-id" {
		t.Errorf("Expected live execution ID 'live-exec-id', got %s", execution.AgentExecutionID)
	}

	// Wait for async goroutine
	time.Sleep(100 * time.Millisecond)

	// StartAgentProcess should be called with the live ID
	got, _ := startedWithID.Load().(string)
	if got != "live-exec-id" {
		t.Errorf("Expected StartAgentProcess called with 'live-exec-id', got %q", got)
	}

	// Database should be updated with the corrected ID
	updatedSession := repo.sessions["session-123"]
	if updatedSession.AgentExecutionID != "live-exec-id" {
		t.Errorf("Expected DB AgentExecutionID to be corrected to 'live-exec-id', got %s", updatedSession.AgentExecutionID)
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

	// Use runAgentProcessAsync with a no-op onSuccess that should never be called
	exec.runAgentProcessAsync(context.Background(), "task-123", "session-123", "exec-456", func(ctx context.Context) {
		t.Error("onSuccess should not be called when StartAgentProcess fails")
		close(done)
	})

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

		resp := &LaunchAgentResponse{AgentExecutionID: "exec-1"}
		executor.persistResumeState(context.Background(), "task-1", session, resp, true, executorConfig{}, nil)

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

		resp := &LaunchAgentResponse{AgentExecutionID: "exec-2"}
		executor.persistResumeState(context.Background(), "task-1", session, resp, false, executorConfig{}, nil)

		if session.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("expected state WAITING_FOR_INPUT, got %s", session.State)
		}
	})
}
