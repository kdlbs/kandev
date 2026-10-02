package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestMockRepositoryMissingWorkspaceBindingSessionReturnsNoChange(t *testing.T) {
	repo := newMockRepository()
	changed, updatedAt, err := repo.UpdateTaskSessionWorkspaceBindingIfCurrentAttempt(
		context.Background(), &models.TaskSession{ID: "missing", TaskID: "task"},
		models.TaskSessionStateStarting, "attempt",
	)
	if err != nil {
		t.Fatalf("UpdateTaskSessionWorkspaceBindingIfCurrentAttempt: %v", err)
	}
	if changed {
		t.Fatal("changed = true, want false for a missing session")
	}
	if !updatedAt.IsZero() {
		t.Fatalf("updatedAt = %s, want zero time for a missing session", updatedAt)
	}
}

func TestResumeSessionPersistsMaterializedWorkspaceBinding(t *testing.T) {
	const (
		workspacePath = "/tasks/task-1/materialized"
		environmentID = "env-task-1"
	)

	for _, startAgent := range []bool{true, false} {
		name := "workspace-only"
		if startAgent {
			name = "agent-resume"
		}
		t.Run(name, func(t *testing.T) {
			repo := newMockRepository()
			setupLiveResumeTestFixture(repo)
			processStart := make(chan error, 1)
			agentManager := &mockAgentManager{
				launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
					return &LaunchAgentResponse{
						AgentExecutionID: "exec-resumed",
						WorkspacePath:    workspacePath,
						Status:           v1.AgentStatusStarting,
					}, nil
				},
				startAgentProcessFunc: func(context.Context, string) error {
					if len(repo.workspaceBindingWrites) == 0 {
						processStart <- errors.New("agent process started before workspace binding persistence")
						return nil
					}
					write := repo.workspaceBindingWrites[len(repo.workspaceBindingWrites)-1]
					if write.TaskEnvironmentID != environmentID || write.WorkspacePath != workspacePath {
						processStart <- errors.New("agent process started with an incomplete workspace binding")
						return nil
					}
					processStart <- nil
					return nil
				},
			}
			exec := newTestExecutor(t, agentManager, repo)

			if _, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], startAgent); err != nil {
				t.Fatalf("ResumeSession(%t): %v", startAgent, err)
			}

			if len(repo.workspaceBindingWrites) != 1 {
				t.Fatalf("workspace binding writes = %d, want one immutable final write", len(repo.workspaceBindingWrites))
			}
			persisted := repo.workspaceBindingWrites[0]
			if persisted.TaskEnvironmentID != environmentID || persisted.WorkspacePath != workspacePath {
				t.Fatalf("persisted session binding = (%q, %q), want (%q, %q)",
					persisted.TaskEnvironmentID, persisted.WorkspacePath, environmentID, workspacePath)
			}
			if startAgent {
				select {
				case err := <-processStart:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("agent process did not reach the start boundary")
				}
			}
		})
	}
}

func TestResumeSessionPersistsWorkspaceBindingAfterOwnedAttemptAdvances(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	agentManager := &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			current, err := repo.GetTaskSession(ctx, req.SessionID)
			if err != nil {
				return nil, err
			}
			current.State = models.TaskSessionStateWaitingForInput
			if err := repo.UpdateTaskSession(ctx, current); err != nil {
				return nil, err
			}
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-resumed",
				WorkspacePath:    "/tasks/task-1/materialized",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)

	if _, err := exec.ResumeSession(context.Background(), cloneMockTaskSession(repo.sessions["sess-1"]), true); err != nil {
		t.Fatalf("ResumeSession after the owned attempt advanced: %v", err)
	}
	if got := repo.sessions["sess-1"].State; got != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %s, want the owned attempt's WAITING_FOR_INPUT state", got)
	}
	if got := repo.sessions["sess-1"].WorkspacePath; got != "/tasks/task-1/materialized" {
		t.Fatalf("persisted workspace path = %q, want materialized workspace", got)
	}
}

func TestResumeSessionWorkspaceBindingWriteFailureRollsBackBeforeAgentStart(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	attachManagedGitHubRepositoryForResume(t, repo)
	oldSnapshot := models.GitCredentialSnapshot{Version: 1, Source: "executor", Transport: "executor_selected"}
	repo.sessions["sess-1"].Metadata = map[string]interface{}{
		models.SessionMetaKeyGitCredentialSnapshot: oldSnapshot,
	}
	persistErr := errors.New("workspace binding write failed")
	repo.updateTaskSessionWorkspaceBindingFunc = func(
		context.Context,
		*models.TaskSession,
		models.TaskSessionState,
		string,
	) (bool, time.Time, error) {
		return false, time.Time{}, persistErr
	}
	var processStarts, executionStops int
	var reservationReleases int
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-resumed",
				WorkspacePath:    "/tasks/task-1/materialized",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error {
			processStarts++
			return nil
		},
		stopAgentFunc: func(context.Context, string, bool) error {
			executionStops++
			return nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)
	issuer := &resumeCredentialStateIssuer{repo: repo}
	exec.SetGitHubCredentialBroker(issuer, "http://localhost:8080/api/github/credentials/resolve")
	exec.SetOnCeilingReservationRelease(func(string) { reservationReleases++ })

	if _, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true); !errors.Is(err, persistErr) {
		t.Fatalf("ResumeSession error = %v, want %v", err, persistErr)
	}
	if processStarts != 0 {
		t.Fatalf("agent process starts = %d, want 0", processStarts)
	}
	if executionStops != 1 {
		t.Fatalf("unstarted execution stops = %d, want 1", executionStops)
	}
	if got := repo.sessions["sess-1"].State; got != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %s, want rollback to WAITING_FOR_INPUT", got)
	}
	if got := repo.sessions["sess-1"].Metadata[models.SessionMetaKeyGitCredentialSnapshot]; got != oldSnapshot {
		t.Fatalf("credential snapshot after current-attempt rollback = %#v, want %#v", got, oldSnapshot)
	}
	if reservationReleases != 1 {
		t.Fatalf("reservation releases = %d, want one for the owned rollback", reservationReleases)
	}
}

func TestResumeSessionWorkspaceBindingSupersededAttemptDoesNotRollBackSuccessor(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	repo.updateTaskSessionWorkspaceBindingFunc = func(
		_ context.Context,
		session *models.TaskSession,
		_ models.TaskSessionState,
		_ string,
	) (bool, time.Time, error) {
		current := repo.sessions[session.ID]
		current.State = models.TaskSessionStateStarting
		current.Metadata = map[string]interface{}{models.SessionMetaKeyAgentStartAttemptID: "successor-attempt"}
		return false, time.Time{}, nil
	}
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-resumed",
				WorkspacePath:    "/tasks/task-1/materialized",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)

	if _, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true); err == nil {
		t.Fatal("ResumeSession succeeded after its startup attempt was replaced")
	}
	current := repo.sessions["sess-1"]
	if current.State != models.TaskSessionStateStarting ||
		models.StringFromAny(current.Metadata[models.SessionMetaKeyAgentStartAttemptID]) != "successor-attempt" {
		t.Fatalf("successor session was changed by stale resume: %+v", current)
	}
	if len(repo.workspaceBindingWrites) != 0 {
		t.Fatalf("workspace binding writes = %d, want none for superseded attempt", len(repo.workspaceBindingWrites))
	}
}

func TestResumeSessionWorkspaceBindingReadFailureDoesNotRollBackSuccessorAttempt(t *testing.T) {
	repo, issuer, successorSnapshot := setupResumeWorkspaceBindingSuccessor(t)
	readErr := errors.New("one-shot session read failure")
	var successorInstalled, diagnosticReadFailed bool
	repo.getTaskSessionFunc = func(_ context.Context, sessionID string) (*models.TaskSession, error) {
		if successorInstalled && !diagnosticReadFailed {
			diagnosticReadFailed = true
			return nil, readErr
		}
		return cloneMockTaskSession(repo.sessions[sessionID]), nil
	}
	repo.updateTaskSessionWorkspaceBindingFunc = func(
		_ context.Context,
		session *models.TaskSession,
		_ models.TaskSessionState,
		_ string,
	) (bool, time.Time, error) {
		installSuccessorResumeAttempt(repo, successorSnapshot)
		successorInstalled = true
		return false, time.Time{}, nil
	}
	var starts, releases int
	var stopped []string
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-stale-resume",
				WorkspacePath:    "/tasks/task-1/materialized",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error {
			starts++
			return nil
		},
		stopAgentFunc: func(_ context.Context, executionID string, _ bool) error {
			stopped = append(stopped, executionID)
			return nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)
	exec.SetGitHubCredentialBroker(issuer, "http://localhost:8080/api/github/credentials/resolve")
	exec.SetOnCeilingReservationRelease(func(string) { releases++ })

	_, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true)
	if !errors.Is(err, ErrSessionStateSuperseded) {
		t.Fatalf("ResumeSession error = %v, want superseded attempt even after read error", err)
	}
	if !diagnosticReadFailed || !errors.Is(err, readErr) {
		t.Fatalf("diagnostic read error = %v, failed=%v; want wrapped %v", err, diagnosticReadFailed, readErr)
	}
	assertResumeSuccessorUnchanged(t, repo, successorSnapshot, starts, releases, stopped)
}

func TestResumeSessionWorkspaceBindingWriteErrorDoesNotRollBackSuccessorAttempt(t *testing.T) {
	repo, issuer, successorSnapshot := setupResumeWorkspaceBindingSuccessor(t)
	persistErr := errors.New("workspace binding write failed")
	repo.updateTaskSessionWorkspaceBindingFunc = func(
		_ context.Context,
		_ *models.TaskSession,
		_ models.TaskSessionState,
		_ string,
	) (bool, time.Time, error) {
		installSuccessorResumeAttempt(repo, successorSnapshot)
		return false, time.Time{}, persistErr
	}
	var starts, releases int
	var stopped []string
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-stale-resume",
				WorkspacePath:    "/tasks/task-1/materialized",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error {
			starts++
			return nil
		},
		stopAgentFunc: func(_ context.Context, executionID string, _ bool) error {
			stopped = append(stopped, executionID)
			return nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)
	exec.SetGitHubCredentialBroker(issuer, "http://localhost:8080/api/github/credentials/resolve")
	exec.SetOnCeilingReservationRelease(func(string) { releases++ })

	_, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true)
	if !errors.Is(err, persistErr) {
		t.Fatalf("ResumeSession error = %v, want %v", err, persistErr)
	}
	assertResumeSuccessorUnchanged(t, repo, successorSnapshot, starts, releases, stopped)
}

func TestRestoreResumeCredentialSnapshotDoesNotChangeSuccessorAttempt(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	successorSnapshot := models.GitCredentialSnapshot{Version: 1, Source: "successor", Transport: "successor"}
	repo.sessions["sess-1"].State = models.TaskSessionStateStarting
	repo.sessions["sess-1"].Metadata = map[string]interface{}{
		models.SessionMetaKeyAgentStartAttemptID:   "attempt-current",
		models.SessionMetaKeyGitCredentialSnapshot: models.GitCredentialSnapshot{Version: 1, Source: "current"},
	}
	repo.getTaskSessionFunc = func(_ context.Context, sessionID string) (*models.TaskSession, error) {
		staleSnapshot := cloneMockTaskSession(repo.sessions[sessionID])
		current := repo.sessions[sessionID]
		current.Metadata[models.SessionMetaKeyAgentStartAttemptID] = "successor-attempt"
		current.Metadata[models.SessionMetaKeyGitCredentialSnapshot] = successorSnapshot
		return staleSnapshot, nil
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.restoreResumeCredentialSnapshotIfStarting(context.Background(), "sess-1", "attempt-current", &resumeCredentialSnapshotBackup{
		value: models.GitCredentialSnapshot{Version: 1, Source: "previous"}, present: true,
	})

	current := repo.sessions["sess-1"]
	if current.State != models.TaskSessionStateStarting ||
		models.StringFromAny(current.Metadata[models.SessionMetaKeyAgentStartAttemptID]) != "successor-attempt" {
		t.Fatalf("successor attempt changed during credential restore: %+v", current)
	}
	if got := current.Metadata[models.SessionMetaKeyGitCredentialSnapshot]; got != successorSnapshot {
		t.Fatalf("successor credential snapshot = %#v, want %#v", got, successorSnapshot)
	}
}

func setupResumeWorkspaceBindingSuccessor(
	t *testing.T,
) (*mockRepository, *resumeCredentialStateIssuer, models.GitCredentialSnapshot) {
	t.Helper()
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	attachManagedGitHubRepositoryForResume(t, repo)
	repo.sessions["sess-1"].State = models.TaskSessionStateFailed
	oldSnapshot := models.GitCredentialSnapshot{Version: 1, Source: "executor", Transport: "executor_selected"}
	successorSnapshot := models.GitCredentialSnapshot{Version: 1, Source: "successor", Transport: "successor_transport"}
	repo.sessions["sess-1"].Metadata = map[string]interface{}{
		models.SessionMetaKeyGitCredentialSnapshot: oldSnapshot,
		"unrelated": "successor metadata",
	}
	repo.sessions["sess-1"].TaskEnvironmentID = "environment-successor"
	repo.sessions["sess-1"].WorkspacePath = "/tasks/successor-workspace"
	repo.executorsRunning["sess-1"].AgentExecutionID = "exec-successor"
	return repo, &resumeCredentialStateIssuer{repo: repo}, successorSnapshot
}

func installSuccessorResumeAttempt(repo *mockRepository, snapshot models.GitCredentialSnapshot) {
	current := repo.sessions["sess-1"]
	current.State = models.TaskSessionStateStarting
	current.ErrorMessage = "successor error marker"
	current.Metadata = map[string]interface{}{
		models.SessionMetaKeyAgentStartAttemptID:   "successor-attempt",
		models.SessionMetaKeyGitCredentialSnapshot: snapshot,
		"unrelated": "successor metadata",
	}
	current.TaskEnvironmentID = "environment-successor"
	current.WorkspacePath = "/tasks/successor-workspace"
	current.AgentExecutionID = "exec-successor"
}

func assertResumeSuccessorUnchanged(
	t *testing.T,
	repo *mockRepository,
	snapshot models.GitCredentialSnapshot,
	processStarts, reservationReleases int,
	stopped []string,
) {
	t.Helper()
	current := repo.sessions["sess-1"]
	if current.State != models.TaskSessionStateStarting || current.ErrorMessage != "successor error marker" {
		t.Fatalf("successor state/error changed: state=%s error=%q", current.State, current.ErrorMessage)
	}
	if got := models.StringFromAny(current.Metadata[models.SessionMetaKeyAgentStartAttemptID]); got != "successor-attempt" {
		t.Fatalf("successor attempt = %q, want successor-attempt", got)
	}
	if got := current.Metadata[models.SessionMetaKeyGitCredentialSnapshot]; got != snapshot {
		t.Fatalf("successor credential snapshot = %#v, want %#v", got, snapshot)
	}
	if got := current.Metadata["unrelated"]; got != "successor metadata" {
		t.Fatalf("successor unrelated metadata = %v, want preserved", got)
	}
	if current.TaskEnvironmentID != "environment-successor" || current.WorkspacePath != "/tasks/successor-workspace" {
		t.Fatalf("successor binding changed: environment=%q workspace=%q", current.TaskEnvironmentID, current.WorkspacePath)
	}
	if current.AgentExecutionID != "exec-successor" || repo.executorsRunning["sess-1"].AgentExecutionID != "exec-successor" {
		t.Fatalf("successor execution changed: session=%q running=%q", current.AgentExecutionID, repo.executorsRunning["sess-1"].AgentExecutionID)
	}
	if processStarts != 0 {
		t.Fatalf("stale agent process starts = %d, want 0", processStarts)
	}
	if reservationReleases != 0 {
		t.Fatalf("successor reservation releases = %d, want 0", reservationReleases)
	}
	if len(stopped) != 1 || stopped[0] != "exec-stale-resume" {
		t.Fatalf("stopped executions = %v, want only admitted stale execution", stopped)
	}
}

func TestResumeSessionCancelledBeforeWorkspaceBindingDoesNotRestoreState(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	repo.updateTaskSessionWorkspaceBindingFunc = func(
		_ context.Context,
		session *models.TaskSession,
		_ models.TaskSessionState,
		_ string,
	) (bool, time.Time, error) {
		repo.sessions[session.ID].State = models.TaskSessionStateCancelled
		return false, time.Time{}, nil
	}
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-resumed",
				WorkspacePath:    "/tasks/task-1/materialized",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)

	if _, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true); err == nil {
		t.Fatal("ResumeSession succeeded after cancellation")
	}
	if got := repo.sessions["sess-1"].State; got != models.TaskSessionStateCancelled {
		t.Fatalf("session state = %s, want cancellation to remain authoritative", got)
	}
}

func TestResumeSessionChangedEnvironmentPreventsWorkspaceBindingWrite(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	const workspacePath = "/tasks/task-1/materialized"
	initialEnvironment := &models.TaskEnvironment{
		ID:            "env-initial",
		TaskID:        "task-1",
		ExecutorType:  string(models.ExecutorTypeLocal),
		Status:        models.TaskEnvironmentStatusReady,
		WorkspacePath: workspacePath,
	}
	replacedEnvironment := &models.TaskEnvironment{
		ID:            "env-replaced",
		TaskID:        "task-1",
		ExecutorType:  string(models.ExecutorTypeLocal),
		Status:        models.TaskEnvironmentStatusReady,
		WorkspacePath: "/tasks/task-1/replaced",
	}
	repo.taskEnvironments[initialEnvironment.ID] = initialEnvironment
	repo.taskEnvironments[replacedEnvironment.ID] = replacedEnvironment
	repo.sessions["sess-1"].TaskEnvironmentID = initialEnvironment.ID
	var environmentReads int
	repo.getTaskEnvironmentByTaskIDFunc = func(context.Context, string) (*models.TaskEnvironment, error) {
		environmentReads++
		if environmentReads == 1 {
			return initialEnvironment, nil
		}
		return replacedEnvironment, nil
	}
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-resumed",
				WorkspacePath:    workspacePath,
				Status:           v1.AgentStatusStarting,
			}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)

	if _, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true); !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("ResumeSession error = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if len(repo.workspaceBindingWrites) != 0 {
		t.Fatalf("workspace binding writes = %d, want none after environment replacement", len(repo.workspaceBindingWrites))
	}
	if got := repo.sessions["sess-1"].TaskEnvironmentID; got != initialEnvironment.ID {
		t.Fatalf("persisted environment ID = %q, want unchanged %q", got, initialEnvironment.ID)
	}
}
