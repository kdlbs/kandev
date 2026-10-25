package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type exactAttemptRecordingRepo struct {
	*sqliterepo.Repository
	attempts []models.ExactProfileLaunchReceipt
}

func (r *exactAttemptRecordingRepo) RecordExactProfileLaunchReceipt(ctx context.Context, receipt *models.ExactProfileLaunchReceipt) (bool, error) {
	r.attempts = append(r.attempts, *receipt)
	return r.Repository.RecordExactProfileLaunchReceipt(ctx, receipt)
}

func TestStartCreatedSessionSuccessfulRetryKeepsFirstDenialReceipt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCreated)
	task, err := repo.GetTask(ctx, "task1")
	if err != nil {
		t.Fatal(err)
	}
	task.WorkspaceID = "ws1"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	const revision int64 = 42
	seedExactProfileBindingSnapshot(t, repo, revision)
	if _, err := repo.AssignExactProfileAssignment(ctx, &models.ExactProfileAssignment{
		TaskID: "task1", WorkspaceID: "ws1", AgentProfileID: "profile-exact", ProfileRevision: revision, Generation: 1,
	}); err != nil {
		t.Fatal(err)
	}
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatal(err)
	}
	session.AgentProfileID = "profile-exact"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	profile := &executor.AgentProfileInfo{ProfileID: "profile-exact", WorkspaceID: "ws1", Revision: revision, Model: "exact-model"}
	agentMgr := &mockAgentManager{resolveProfileInfo: profile,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-retry"}, nil
		}}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", WorkspaceID: "ws1", Title: "Test Task", State: v1.TaskStateInProgress}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	recording := &exactAttemptRecordingRepo{Repository: repo}
	svc.repo = recording
	if _, err := svc.StartCreatedSession(ctx, "task1", "session1", "profile-exact", "start", false, false, true, nil, nil); !errors.Is(err, ErrExactProfileAssignmentInvalid) {
		t.Fatalf("first launch error = %v, want failed-closed denial", err)
	}
	profile.Enabled = true
	execution, err := svc.StartCreatedSession(ctx, "task1", "session1", "profile-exact", "retry", false, false, true, nil, nil)
	if err != nil || execution == nil || execution.AgentExecutionID != "exec-retry" {
		t.Fatalf("retry = (%+v, %v), want launched execution", execution, err)
	}
	if len(recording.attempts) != 2 || recording.attempts[0].Outcome != models.ExactProfileLaunchOutcomeFailedClosed ||
		recording.attempts[1].Outcome != models.ExactProfileLaunchOutcomeApplied || !recording.attempts[1].InferenceStarted {
		t.Fatalf("current-attempt writes = %+v", recording.attempts)
	}
	stored, err := repo.GetExactProfileLaunchReceipt(ctx, "task1", "session1")
	if err != nil || stored == nil || stored.Outcome != models.ExactProfileLaunchOutcomeFailedClosed || stored.InferenceStarted {
		t.Fatalf("first historical receipt = (%+v, %v)", stored, err)
	}
	response := svc.withExactProfileLaunchReceipt(ctx, executionToLaunchResponse("task1", execution))
	if response == nil || !response.Success || response.ExactProfileLaunchReceipt == nil ||
		response.ExactProfileLaunchReceipt.Outcome != models.ExactProfileLaunchOutcomeFailedClosed {
		t.Fatalf("successful retry with first historical audit = %+v", response)
	}
}

func TestResumeTaskSessionFailedRetryKeepsFirstSuccessReceipt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	task, err := repo.GetTask(ctx, "task1")
	if err != nil {
		t.Fatal(err)
	}
	task.WorkspaceID = "ws1"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	const revision int64 = 42
	seedExactProfileBindingSnapshot(t, repo, revision)
	if _, err := repo.AssignExactProfileAssignment(ctx, &models.ExactProfileAssignment{
		TaskID: "task1", WorkspaceID: "ws1", AgentProfileID: "profile-exact", ProfileRevision: revision, Generation: 1,
	}); err != nil {
		t.Fatal(err)
	}
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatal(err)
	}
	session.AgentExecutionID = "exec-first"
	session.AgentProfileID = "profile-exact"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	session.ExactProfileGeneration = 1
	session.ExactProfileRevision = revision
	seedExactProfileSessionBinding(t, repo, session)
	seedExecutorRunning(t, repo, session.ID, session.TaskID, session.AgentExecutionID)
	if _, err := repo.RecordExactProfileLaunchReceipt(ctx, &models.ExactProfileLaunchReceipt{
		TaskID: "task1", SessionID: "session1", AgentProfileID: "profile-exact", Generation: 1,
		ProfileRevision: revision, Model: "exact-model", Outcome: models.ExactProfileLaunchOutcomeApplied,
		InferenceStarted: true,
	}); err != nil {
		t.Fatal(err)
	}
	agentMgr := &mockAgentManager{isAgentRunning: false, repoForExecutionLookup: repo,
		resolveProfileInfo: &executor.AgentProfileInfo{ProfileID: "profile-exact", WorkspaceID: "ws1", Enabled: false,
			Revision: revision, Model: "exact-model"}}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", WorkspaceID: "ws1", Title: "Test Task", State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	recording := &exactAttemptRecordingRepo{Repository: repo}
	svc.repo = recording
	if _, err := svc.ResumeTaskSession(ctx, "task1", "session1"); !errors.Is(err, ErrExactProfileAssignmentInvalid) {
		t.Fatalf("retry error = %v, want failed-closed denial", err)
	}
	if len(recording.attempts) != 1 || recording.attempts[0].Outcome != models.ExactProfileLaunchOutcomeFailedClosed ||
		recording.attempts[0].InferenceStarted || recording.attempts[0].Generation != 1 ||
		recording.attempts[0].ProfileRevision != revision {
		t.Fatalf("current failed retry = %+v", recording.attempts)
	}
	stored, err := repo.GetExactProfileLaunchReceipt(ctx, "task1", "session1")
	if err != nil || stored == nil || stored.Outcome != models.ExactProfileLaunchOutcomeApplied || !stored.InferenceStarted {
		t.Fatalf("first historical receipt = (%+v, %v)", stored, err)
	}
}

func TestExactProfileAttemptResultKeepsCurrentRetrySeparateFromFirstReceipt(t *testing.T) {
	for _, tc := range []struct {
		name          string
		firstFailure  bool
		firstOutcome  string
		secondOutcome string
	}{
		{"failed first, successful retry", true, models.ExactProfileLaunchOutcomeFailedClosed, models.ExactProfileLaunchOutcomeApplied},
		{"successful first, failed retry", false, models.ExactProfileLaunchOutcomeApplied, models.ExactProfileLaunchOutcomeFailedClosed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCreated)
			service := &Service{repo: repo}
			first := &ExactProfileLaunchDecision{AgentProfileID: "profile-first", Generation: 1, Revision: 11, Model: "model-first"}
			second := &ExactProfileLaunchDecision{AgentProfileID: "profile-second", Generation: 2, Revision: 22, Model: "model-second"}
			var firstExecution, secondExecution *executor.TaskExecution
			if tc.firstFailure {
				secondExecution = &executor.TaskExecution{AgentExecutionID: "execution-retry"}
			} else {
				firstExecution = &executor.TaskExecution{AgentExecutionID: "execution-first"}
			}
			firstResult := exactProfileAttemptResultFor(first, firstExecution)
			if err := service.recordExactProfileLaunchReceipt(ctx, "task1", "session1", firstResult); err != nil {
				t.Fatal(err)
			}
			secondResult := exactProfileAttemptResultFor(second, secondExecution)
			if err := service.recordExactProfileLaunchReceipt(ctx, "task1", "session1", secondResult); err != nil {
				t.Fatal(err)
			}
			stored, err := repo.GetExactProfileLaunchReceipt(ctx, "task1", "session1")
			if err != nil {
				t.Fatal(err)
			}
			if stored == nil || stored.Outcome != tc.firstOutcome || stored.AgentProfileID != first.AgentProfileID ||
				stored.Generation != first.Generation || stored.ProfileRevision != first.Revision {
				t.Fatalf("historical first receipt = %+v", stored)
			}
			if secondResult == nil || secondResult.Outcome != tc.secondOutcome ||
				secondResult.AgentProfileID != second.AgentProfileID || secondResult.Generation != second.Generation ||
				secondResult.ProfileRevision != second.Revision || secondResult.InferenceStarted != (secondExecution != nil) {
				t.Fatalf("current retry result = %+v", secondResult)
			}
			if secondExecution != nil && secondResult.ExecutionID != secondExecution.AgentExecutionID {
				t.Fatalf("current retry execution = %q, want %q", secondResult.ExecutionID, secondExecution.AgentExecutionID)
			}
		})
	}
}
