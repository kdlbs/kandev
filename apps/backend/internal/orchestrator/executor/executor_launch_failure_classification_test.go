package executor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestClassifyLaunchFailureUsesTypedBaseBranchCategory(t *testing.T) {
	classification := classifyLaunchFailure(errors.Join(
		errors.New("environment preparation failed"), worktree.ErrInvalidBaseBranch,
	))
	if classification.code != models.LaunchErrorCategoryBaseBranchMissing {
		t.Fatalf("classification code = %q, want %q", classification.code, models.LaunchErrorCategoryBaseBranchMissing)
	}
	if classification.message == "" || classification.message == "environment preparation failed" {
		t.Fatalf("classification message = %q, want safe user message", classification.message)
	}
}

func TestClassifyLaunchFailureUsesWorkspaceCheckoutCategory(t *testing.T) {
	classification := classifyLaunchFailure(errors.Join(
		errors.New("PR ref could not be materialized"), worktree.ErrWorkspaceCheckoutFailed,
	))
	if classification.code != models.LaunchErrorCategoryWorkspaceCheckoutFailed {
		t.Fatalf("classification code = %q, want %q", classification.code, models.LaunchErrorCategoryWorkspaceCheckoutFailed)
	}
}

func TestTransitionLaunchFailurePersistsTypedErrorAndExactTaskRepository(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCreated,
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	_, changed := exec.transitionLaunchFailure(
		context.Background(), "task-1", "session-1", "repo-1", "task-repo-1",
		errors.Join(errors.New("prepare failed"), worktree.ErrInvalidBaseBranch),
	)
	if !changed {
		t.Fatal("transitionLaunchFailure did not transition the session")
	}
	session, err := repo.GetTaskSession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	errorValue, found := models.LoadLastAgentError(session.Metadata)
	if !found {
		t.Fatal("typed launch error was not persisted")
	}
	if errorValue.Code != models.LaunchErrorCategoryBaseBranchMissing {
		t.Fatalf("error code = %q, want %q", errorValue.Code, models.LaunchErrorCategoryBaseBranchMissing)
	}
	if errorValue.TaskRepositoryID != "task-repo-1" {
		t.Fatalf("task repository id = %q, want task-repo-1", errorValue.TaskRepositoryID)
	}
	if len(errorValue.RecoveryActions) != 2 ||
		errorValue.RecoveryActions[0] != models.RecoveryActionRetryDefault ||
		errorValue.RecoveryActions[1] != models.RecoveryActionPickBaseBranch {
		t.Fatalf("recovery actions = %#v, want default retry and branch picker", errorValue.RecoveryActions)
	}
}

func TestGenericLaunchFailureAlwaysOffersRetryLaunch(t *testing.T) {
	exec := &Executor{}
	exec.launchFailureReviewEligibility = func(context.Context, string) (bool, error) {
		return true, nil
	}
	errorValue := exec.buildLastAgentError(context.Background(), "task-1", "", errors.New("start failed"))
	if len(errorValue.RecoveryActions) != 1 || errorValue.RecoveryActions[0] != models.RecoveryActionRetryLaunch {
		t.Fatalf("eligible recovery actions = %#v, want retry_launch", errorValue.RecoveryActions)
	}

	exec.launchFailureReviewEligibility = func(context.Context, string) (bool, error) {
		return false, errors.New("lookup failed")
	}
	errorValue = exec.buildLastAgentError(context.Background(), "task-1", "", errors.New("start failed"))
	if len(errorValue.RecoveryActions) != 1 || errorValue.RecoveryActions[0] != models.RecoveryActionRetryLaunch {
		t.Fatalf("failed eligibility lookup exposed recovery actions = %#v", errorValue.RecoveryActions)
	}
}

func TestBuildLastAgentErrorSanitizesRepositoryPreparationDetails(t *testing.T) {
	exec := &Executor{}
	launchErr := &lifecycle.RepositoryPreparationError{
		RepositoryID:   "repo-back",
		RepositoryName: "backend",
		Cause:          errors.New("fatal: https://user:ghp_abcdefghijklmnopqrstuvwxyz1234567890AB@example.com/repo.git"),
	}

	errorValue := exec.buildLastAgentError(context.Background(), "task-1", "task-repo-2", launchErr)
	if !strings.Contains(errorValue.Details, "repo-back") || !strings.Contains(errorValue.Details, "backend") {
		t.Fatalf("launch details = %q, want repository identity", errorValue.Details)
	}
	if strings.Contains(errorValue.Details, "ghp_abcdefghijklmnopqrstuvwxyz1234567890AB") ||
		strings.Contains(errorValue.Details, "user:") {
		t.Fatalf("launch details exposed credential-bearing URL: %q", errorValue.Details)
	}
}

func TestBuildBootstrapLastAgentErrorUsesSafeCorrelatedProjection(t *testing.T) {
	exec := &Executor{}
	launchErr := &lifecycle.BootstrapFailure{
		Operation: "resume",
		Code:      models.AgentErrorCauseCodePermissionDenied,
		Detail:    "The required contribution access was denied.",
		Cause:     errors.New("permission denied for /private/worktree token=ghp_secret"),
	}

	errorValue := exec.buildBootstrapLastAgentError(
		context.Background(), "task-1", "session-1", "execution-1", launchErr, true,
	)
	if errorValue.Message != "The agent could not start." {
		t.Fatalf("bootstrap message = %q, want safe generic copy", errorValue.Message)
	}
	if errorValue.Code != models.LaunchErrorCategoryGenericLaunchFailure {
		t.Fatalf("bootstrap code = %q, want generic launch failure", errorValue.Code)
	}
	if errorValue.Phase != models.LaunchErrorPhaseBootstrap ||
		errorValue.ExecutionID != "execution-1" || errorValue.AttemptID != "execution-1" {
		t.Fatalf("bootstrap identity = %+v", errorValue)
	}
	if strings.Contains(errorValue.Details, "private/worktree") || strings.Contains(errorValue.Details, "ghp_secret") {
		t.Fatalf("bootstrap details exposed raw failure: %q", errorValue.Details)
	}
	if len(errorValue.Causes) != 1 || errorValue.Causes[0].Operation != models.AgentErrorCauseOperationResume ||
		errorValue.Causes[0].Code != models.AgentErrorCauseCodePermissionDenied {
		t.Fatalf("bootstrap causes = %#v", errorValue.Causes)
	}
}

func TestBootstrapFailureProjection(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID:               "session-1",
		TaskID:           "task-1",
		State:            models.TaskSessionStateStarting,
		AgentExecutionID: "exec-1",
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1", State: v1.TaskStateReview}
	stopCh := make(chan struct{})
	manager := &mockAgentManager{
		startAgentProcessFunc: func(context.Context, string) error {
			return &lifecycle.BootstrapFailure{
				Operation: models.AgentErrorCauseOperationResume,
				Code:      models.AgentErrorCauseCodePermissionDenied,
				Detail:    "The required contribution access was denied.",
				Cause:     errors.New("permission denied for /private/worktree token=ghp_secret"),
			}
		},
		stopAgentFunc: func(context.Context, string, bool) error {
			close(stopCh)
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)
	exec.SetOnSessionStateChange(func(ctx context.Context, _, sessionID string, state models.TaskSessionState, message string) error {
		return repo.UpdateTaskSessionState(ctx, sessionID, state, message)
	})

	exec.runAgentProcessAsync(
		context.Background(), "task-1", "session-1", "exec-1",
		func(context.Context) { t.Fatal("onSuccess ran after bootstrap failure") },
		false, true,
	)
	select {
	case <-stopCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for failed execution cleanup")
	}

	session, err := repo.GetTaskSession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateFailed {
		t.Fatalf("session state = %q, want FAILED", session.State)
	}
	if session.ErrorMessage != "The agent could not start." {
		t.Fatalf("session error message = %q, want safe generic copy", session.ErrorMessage)
	}
	errorValue, found := models.LoadLastAgentError(session.Metadata)
	if !found {
		t.Fatal("bootstrap failure was not persisted")
	}
	if errorValue.Phase != models.LaunchErrorPhaseBootstrap ||
		errorValue.ExecutionID != "exec-1" || errorValue.AttemptID != "exec-1" {
		t.Fatalf("bootstrap correlation = %+v", errorValue)
	}
	if errorValue.Stamp() == "" || len(errorValue.Causes) != 1 {
		t.Fatalf("bootstrap identity/cause = %+v", errorValue)
	}
	if strings.Contains(errorValue.Details, "private/worktree") || strings.Contains(errorValue.Details, "ghp_secret") {
		t.Fatalf("bootstrap details exposed raw startup error: %q", errorValue.Details)
	}
}

func TestBootstrapFailureSuccessorFence(t *testing.T) {
	repo := newMockRepository()
	successor := models.LastAgentError{
		Message:          "The agent could not start.",
		OccurredAt:       time.Now().UTC(),
		AgentExecutionID: "exec-new",
		ExecutionID:      "exec-new",
		Phase:            models.LaunchErrorPhaseBootstrap,
		AttemptID:        "exec-new",
		Code:             models.LaunchErrorCategoryGenericLaunchFailure,
		Details:          "operation=agent_bootstrap; cause=timeout",
		StampValue:       "successor-stamp",
	}
	repo.sessions["session-1"] = &models.TaskSession{
		ID:               "session-1",
		TaskID:           "task-1",
		State:            models.TaskSessionStateRunning,
		AgentExecutionID: "exec-new",
		Metadata: map[string]interface{}{
			models.SessionMetaKeyLastAgentError: successor,
		},
	}
	stopCh := make(chan struct{})
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "exec-new", nil
		},
		stopAgentFunc: func(context.Context, string, bool) error {
			close(stopCh)
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)
	exec.handleAgentProcessStartFailure(
		context.Background(), "task-1", "session-1", "exec-old",
		&lifecycle.BootstrapFailure{
			Operation: models.AgentErrorCauseOperationResume,
			Code:      models.AgentErrorCauseCodePermissionDenied,
			Detail:    "The required contribution access was denied.",
			Cause:     errors.New("old execution failed"),
		},
		true, true,
	)
	select {
	case <-stopCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for stale execution cleanup")
	}

	session, err := repo.GetTaskSession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("successor session state = %q, want RUNNING", session.State)
	}
	current, found := models.LoadLastAgentError(session.Metadata)
	if !found || current.Stamp() != "successor-stamp" {
		t.Fatalf("successor error = %+v, found=%t; stale failure replaced it", current, found)
	}
}

type bootstrapFailureSuccessorFenceRepository struct {
	*mockRepository
	successor      models.LastAgentError
	expectedStamp  string
	commitCallSeen bool
}

func (r *bootstrapFailureSuccessorFenceRepository) CommitBootstrapFailureIfCurrentExecution(
	_ context.Context,
	_, sessionID, _ string,
	expectedState models.TaskSessionState,
	expectedStamp string,
	_ models.LastAgentError,
) (bool, time.Time, error) {
	r.commitCallSeen = true
	if expectedState != models.TaskSessionStateStarting && expectedState != models.TaskSessionStateRunning {
		return false, time.Time{}, errors.New("unexpected bootstrap state")
	}
	if expectedStamp != r.expectedStamp {
		return false, time.Time{}, errors.New("bootstrap stamp was not captured from the final ownership read")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.sessions[sessionID]
	session.AgentExecutionID = "exec-new"
	session.State = models.TaskSessionStateRunning
	session.Metadata = map[string]interface{}{
		models.SessionMetaKeyLastAgentError: r.successor,
	}
	return false, time.Time{}, nil
}

func TestBootstrapFailureSuccessorFenceAfterFinalOwnershipRead(t *testing.T) {
	oldError := models.LastAgentError{
		Message:    "previous bootstrap failure",
		OccurredAt: time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC),
		StampValue: "old-stamp",
	}
	successor := models.LastAgentError{
		Message:          "The agent could not start.",
		OccurredAt:       time.Now().UTC(),
		AgentExecutionID: "exec-new",
		ExecutionID:      "exec-new",
		Phase:            models.LaunchErrorPhaseBootstrap,
		AttemptID:        "exec-new",
		Code:             models.LaunchErrorCategoryGenericLaunchFailure,
		Details:          "operation=agent_bootstrap; cause=successor",
		StampValue:       "successor-stamp",
	}
	for _, test := range []struct {
		name          string
		metadata      map[string]interface{}
		expectedStamp string
	}{
		{name: "absent error"},
		{
			name: "unchanged stamp",
			metadata: map[string]interface{}{
				models.SessionMetaKeyLastAgentError: oldError,
			},
			expectedStamp: "old-stamp",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &bootstrapFailureSuccessorFenceRepository{
				mockRepository: newMockRepository(),
				successor:      successor,
				expectedStamp:  test.expectedStamp,
			}
			repo.sessions["session-1"] = &models.TaskSession{
				ID:               "session-1",
				TaskID:           "task-1",
				State:            models.TaskSessionStateStarting,
				AgentExecutionID: "exec-old",
				Metadata:         test.metadata,
			}
			stopCh := make(chan struct{})
			manager := &mockAgentManager{
				getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
					return "exec-old", nil
				},
				stopAgentFunc: func(context.Context, string, bool) error {
					close(stopCh)
					return nil
				},
			}
			exec := newTestExecutor(t, manager, repo)
			exec.handleAgentProcessStartFailure(
				context.Background(), "task-1", "session-1", "exec-old",
				errors.New("old execution failed"), true, true,
			)
			select {
			case <-stopCh:
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for stale execution cleanup")
			}
			if !repo.commitCallSeen {
				t.Fatal("atomic bootstrap failure committer was not used")
			}
			session, err := repo.GetTaskSession(context.Background(), "session-1")
			if err != nil {
				t.Fatalf("GetTaskSession: %v", err)
			}
			if session.State != models.TaskSessionStateRunning || session.AgentExecutionID != "exec-new" {
				t.Fatalf("successor session = %+v, want RUNNING/exec-new", session)
			}
			current, found := models.LoadLastAgentError(session.Metadata)
			if !found || current.Stamp() != "successor-stamp" {
				t.Fatalf("successor error = %+v, found=%t; stale failure replaced it", current, found)
			}
		})
	}
}
