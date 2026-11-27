package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-003.7
func TestResumeInspectionContentionPreservesTaskState(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	seedSelectedWorktreeRecoveryEnvironment(
		repo, "task-1", "sess-1", models.TaskSessionStateWaitingForInput,
	)
	previousSnapshot := models.GitCredentialSnapshot{Version: 1, Source: "executor", Transport: "selected"}
	session := repo.sessions["sess-1"]
	session.RepositoryID = "repo-recovery"
	session.ErrorMessage = "previous session error"
	session.Metadata = map[string]interface{}{
		models.SessionMetaKeyGitCredentialSnapshot: previousSnapshot,
	}
	repo.repositories["repo-recovery"].LocalPath = t.TempDir()
	repo.repositories["repo-recovery"].Provider = "github"
	repo.repositories["repo-recovery"].SourceType = sourceTypeLocal
	repo.repositories["repo-recovery"].ProviderOwner = "acme"
	repo.repositories["repo-recovery"].ProviderName = "recovery"
	repo.taskRepositories["task-repo-recovery"] = &models.TaskRepository{
		ID: "task-repo-recovery", TaskID: "task-1", RepositoryID: "repo-recovery",
	}

	issuer := &resumeCredentialStateIssuer{repo: repo}
	agentManager := &mockAgentManager{}
	exec := newTestExecutor(t, agentManager, repo)
	exec.SetGitHubCredentialBroker(issuer, "http://localhost:8080/api/github/credentials/resolve")
	var admissionCalls int
	var requestedWaits []time.Duration
	var inspectionDeadlines []time.Time
	contention := &worktree.RecoveryInspectionContentionError{}
	exec.SetSelectedWorktreeRecoveryAdmission(func(
		_ context.Context,
		request worktree.RecoveryAdmissionRequest,
	) (*worktree.RecoveryAdmission, error) {
		admissionCalls++
		requestedWaits = append(requestedWaits, request.InspectionWait)
		inspectionDeadlines = append(inspectionDeadlines, request.InspectionDeadline)
		if admissionCalls == 2 {
			return nil, contention
		}
		return nil, nil
	})

	_, err := exec.ResumeSession(context.Background(), session, true)
	if !errors.Is(err, contention) {
		t.Fatalf("ResumeSession error = %v, want typed inspection contention", err)
	}
	if admissionCalls != 2 {
		t.Fatalf("selected recovery admission calls = %d, want 2", admissionCalls)
	}
	if agentManager.launchAgentCallCount != 0 {
		t.Fatalf("LaunchAgent calls = %d, want 0 before admission succeeds", agentManager.launchAgentCallCount)
	}
	current := repo.sessions["sess-1"]
	if current.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after safe contention = %s, want WAITING_FOR_INPUT", current.State)
	}
	if current.ErrorMessage != "previous session error" {
		t.Fatalf("session error after safe contention = %q, want preserved error", current.ErrorMessage)
	}
	if got := current.Metadata[models.SessionMetaKeyGitCredentialSnapshot]; got != previousSnapshot {
		t.Fatalf("credential snapshot after safe contention = %#v, want %#v", got, previousSnapshot)
	}
	if len(requestedWaits) != 2 || requestedWaits[0] <= 0 || requestedWaits[1] <= 0 ||
		requestedWaits[0] > worktree.RecoveryInspectionWaitBudget ||
		requestedWaits[1] > worktree.RecoveryInspectionWaitBudget {
		t.Fatalf("resume inspection waits = %v, want one bounded wait at both outer admissions", requestedWaits)
	}
	if len(inspectionDeadlines) != 2 || inspectionDeadlines[0].IsZero() ||
		!inspectionDeadlines[0].Equal(inspectionDeadlines[1]) {
		t.Fatalf("resume inspection deadlines = %v, want one shared deadline", inspectionDeadlines)
	}
}

func TestResumeLifecycleInspectionContentionRestoresAttemptSnapshot(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	seedSelectedWorktreeRecoveryEnvironment(
		repo, "task-1", "sess-1", models.TaskSessionStateWaitingForInput,
	)
	previousSnapshot := models.GitCredentialSnapshot{Version: 1, Source: "executor", Transport: "selected"}
	session := repo.sessions["sess-1"]
	session.ErrorMessage = "previous launch error"
	session.Metadata = map[string]interface{}{
		models.SessionMetaKeyGitCredentialSnapshot: previousSnapshot,
	}
	repo.repositories["repo-recovery"].LocalPath = t.TempDir()
	repo.repositories["repo-recovery"].Provider = "github"
	repo.repositories["repo-recovery"].SourceType = sourceTypeLocal
	repo.repositories["repo-recovery"].ProviderOwner = "acme"
	repo.repositories["repo-recovery"].ProviderName = "recovery"
	repo.taskRepositories["task-repo-recovery"] = &models.TaskRepository{
		ID: "task-repo-recovery", TaskID: "task-1", RepositoryID: "repo-recovery",
	}

	contention := &worktree.RecoveryInspectionContentionError{}
	agentManager := &mockAgentManager{
		launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return nil, contention
		},
	}
	issuer := &resumeCredentialStateIssuer{repo: repo}
	exec := newTestExecutor(t, agentManager, repo)
	exec.SetGitHubCredentialBroker(issuer, "http://localhost:8080/api/github/credentials/resolve")
	var admissionCalls int
	exec.SetSelectedWorktreeRecoveryAdmission(func(
		context.Context,
		worktree.RecoveryAdmissionRequest,
	) (*worktree.RecoveryAdmission, error) {
		admissionCalls++
		return nil, nil
	})

	_, err := exec.ResumeSession(context.Background(), session, true)
	if !errors.Is(err, contention) {
		t.Fatalf("ResumeSession error = %v, want lifecycle inspection contention", err)
	}
	if !IsSafeResumeInspectionDeferral(err) {
		t.Fatalf("ResumeSession error = %v, want verified safe deferral", err)
	}
	if admissionCalls != 2 {
		t.Fatalf("outer admissions = %d, want 2 successful admissions before LaunchAgent", admissionCalls)
	}
	if agentManager.launchAgentCallCount != 1 {
		t.Fatalf("LaunchAgent calls = %d, want 1", agentManager.launchAgentCallCount)
	}
	current := repo.sessions["sess-1"]
	if current.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after safe lifecycle contention = %s, want WAITING_FOR_INPUT", current.State)
	}
	if current.ErrorMessage != "previous launch error" {
		t.Fatalf("session error after safe lifecycle contention = %q, want prior error", current.ErrorMessage)
	}
	if got := current.Metadata[models.SessionMetaKeyGitCredentialSnapshot]; got != previousSnapshot {
		t.Fatalf("credential snapshot after safe lifecycle contention = %#v, want %#v", got, previousSnapshot)
	}
}

func TestResumeLifecycleInspectionContentionDoesNotHideUnsafeFailures(t *testing.T) {
	contention := &worktree.RecoveryInspectionContentionError{}
	startupErr := errors.New("lifecycle startup cleanup failed")
	rollbackErr := errors.New("resume rollback persistence failed")
	tests := []struct {
		name             string
		launchErr        error
		rollbackErr      error
		installSuccessor bool
		wantState        models.TaskSessionState
		wantErr          error
	}{
		{
			name:      "joined startup failure uses normal failure rollback",
			launchErr: errors.Join(contention, startupErr),
			wantState: models.TaskSessionStateWaitingForInput,
			wantErr:   startupErr,
		},
		{
			name:        "rollback failure remains an ordinary failure",
			launchErr:   contention,
			rollbackErr: rollbackErr,
			wantState:   models.TaskSessionStateStarting,
			wantErr:     rollbackErr,
		},
		{
			name:             "successor attempt remains untouched",
			launchErr:        contention,
			installSuccessor: true,
			wantState:        models.TaskSessionStateRunning,
			wantErr:          ErrSessionStateSuperseded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepository()
			setupLiveResumeTestFixture(repo)
			seedSelectedWorktreeRecoveryEnvironment(
				repo, "task-1", "sess-1", models.TaskSessionStateWaitingForInput,
			)
			agentManager := &mockAgentManager{
				launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
					return nil, tt.launchErr
				},
			}
			exec := newTestExecutor(t, agentManager, repo)
			exec.SetSelectedWorktreeRecoveryAdmission(func(
				context.Context,
				worktree.RecoveryAdmissionRequest,
			) (*worktree.RecoveryAdmission, error) {
				return nil, nil
			})
			if tt.rollbackErr != nil {
				exec.SetOnResumeFailureRollback(func(
					context.Context,
					ResumeFailureRollbackRequest,
				) (bool, error) {
					return false, tt.rollbackErr
				})
			}
			if tt.installSuccessor {
				exec.SetOnResumeFailureRollback(func(
					_ context.Context,
					request ResumeFailureRollbackRequest,
				) (bool, error) {
					current := repo.sessions[request.SessionID]
					current.State = models.TaskSessionStateRunning
					current.Metadata[models.SessionMetaKeyAgentStartAttemptID] = "successor-attempt"
					return false, nil
				})
			}

			_, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true)
			if !errors.Is(err, contention) {
				t.Fatalf("ResumeSession error = %v, want contention cause", err)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ResumeSession error = %v, want cause %v", err, tt.wantErr)
			}
			if IsSafeResumeInspectionDeferral(err) {
				t.Fatalf("ResumeSession error = %v, unsafe failure was classified as safe deferral", err)
			}
			current := repo.sessions["sess-1"]
			if current.State != tt.wantState {
				t.Fatalf("session state after %s = %s, want %s", tt.name, current.State, tt.wantState)
			}
			if tt.installSuccessor && models.StringFromAny(current.Metadata[models.SessionMetaKeyAgentStartAttemptID]) != "successor-attempt" {
				t.Fatalf("successor attempt metadata = %#v, want successor-attempt", current.Metadata)
			}
			if tt.rollbackErr != nil && !errors.Is(err, tt.rollbackErr) {
				t.Fatalf("ResumeSession error = %v, want rollback failure %v", err, tt.rollbackErr)
			}
		})
	}
}
