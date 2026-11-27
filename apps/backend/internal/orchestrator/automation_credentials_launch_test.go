package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type credentialAutomationLaunchService struct {
	*stubAutomationService
	validationErr       error
	validatedIDs        []string
	validatedAutomation *automation.Automation
	validationCalls     int
}

type credentialRunLaunchAutomationService struct {
	*stubAutomationService
	dispatchAutomation  *automation.Automation
	validationErr       error
	validatedAutomation *automation.Automation
}

func (s *credentialRunLaunchAutomationService) GetAutomationForDispatch(
	_ context.Context,
	id string,
) (*automation.Automation, error) {
	if id != s.dispatchAutomation.ID {
		return nil, nil
	}
	return s.dispatchAutomation, nil
}

func (s *credentialRunLaunchAutomationService) ValidateAutomationCredentials(
	_ context.Context,
	a *automation.Automation,
	_ []string,
) error {
	s.validatedAutomation = a
	return s.validationErr
}

func (s *credentialAutomationLaunchService) ValidateAutomationCredentials(
	_ context.Context,
	a *automation.Automation,
	repositoryIDs []string,
) error {
	s.validationCalls++
	s.validatedAutomation = a
	s.validatedIDs = append([]string(nil), repositoryIDs...)
	return s.validationErr
}

func TestAutomationCredentialLaunch(t *testing.T) {
	ctx := context.Background()
	a := &automation.Automation{
		ID: "automation-1", WorkspaceID: "workspace-1", Name: "Review", Enabled: true,
		TaskMode: automation.TaskModeAutomationRun,
	}
	credentialErr := &automation.CredentialValidationError{Field: "SERVICE_KEY", Source: "agent profile"}

	t.Run("new manual run fails before task creation", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		autoSvc := &credentialAutomationLaunchService{
			stubAutomationService: &stubAutomationService{automation: a}, validationErr: credentialErr,
		}
		svc.SetAutomationService(autoSvc)
		creator := &stubReviewTaskCreator{}
		svc.reviewTaskCreator = creator
		evt := &automation.AutomationTriggeredEvent{AutomationID: a.ID, TriggerType: automation.TriggerTypeManual}

		_, _, _, _, err := svc.prepareAutomationTask(ctx, a, evt, "Review", "Review task", map[string]interface{}{})

		require.ErrorIs(t, err, automation.ErrCredentialsUnavailable)
		require.Equal(t, 1, autoSvc.validationCalls)
		require.Empty(t, creator.got)
		svc.recordFailedRunError(ctx, evt, err)
		require.Len(t, autoSvc.runs, 1)
		require.Equal(t, automation.RunStatusFailed, autoSvc.runs[0].Status)
		require.Equal(t, credentialErr.Error(), autoSvc.runs[0].ErrorMessage)
		require.NotContains(t, autoSvc.runs[0].ErrorMessage, "secret-id")
	})

	t.Run("queued start rechecks credentials before runtime start", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		autoSvc := &credentialAutomationLaunchService{
			stubAutomationService: &stubAutomationService{automation: a}, validationErr: credentialErr,
		}
		svc.SetAutomationService(autoSvc)

		_, err := svc.startAutomationTask(ctx, a, &models.Task{ID: "task-queued"}, "", nil)

		require.ErrorIs(t, err, automation.ErrCredentialsUnavailable)
		require.Equal(t, 1, autoSvc.validationCalls)
		require.Empty(t, autoSvc.runs)
	})

	t.Run("temporary credential storage error remains distinct", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		autoSvc := &credentialAutomationLaunchService{
			stubAutomationService: &stubAutomationService{automation: a},
			validationErr:         errors.Join(automation.ErrCredentialStorageUnavailable, errors.New("database offline")),
		}
		svc.SetAutomationService(autoSvc)
		creator := &stubReviewTaskCreator{}
		svc.reviewTaskCreator = creator

		_, _, _, _, err := svc.prepareAutomationTask(ctx, a,
			&automation.AutomationTriggeredEvent{TriggerType: automation.TriggerTypeScheduled},
			"Review", "Review task", map[string]interface{}{},
		)

		require.ErrorIs(t, err, automation.ErrCredentialStorageUnavailable)
		require.Empty(t, creator.got)
	})
}

func TestAutomationCredentialRunLaunchKeepsPrivateChoices(t *testing.T) {
	ctx := context.Background()
	public := &automation.Automation{ID: "automation-1"}
	dispatch := &automation.Automation{
		ID: "automation-1",
		CredentialChoices: []automation.CredentialChoiceProjection{{
			Kind: automation.CredentialChoicePersonal, ProfileID: "executor-profile", Key: "EXECUTOR_KEY",
			State: automation.CredentialChoiceSelected, SecretID: "personal-secret", SourceSecretID: "profile-secret",
		}},
	}
	autoSvc := &credentialRunLaunchAutomationService{
		stubAutomationService: &stubAutomationService{automation: public},
		dispatchAutomation:    dispatch,
	}
	svc := &Service{automationService: autoSvc}

	got, err := svc.getAutomationForRunLaunch(ctx, dispatch.ID)

	require.NoError(t, err)
	require.Equal(t, dispatch.CredentialChoices, got.CredentialChoices)
	require.Equal(t, "personal-secret", got.CredentialChoices[0].SecretID)
	require.Equal(t, "profile-secret", got.CredentialChoices[0].SourceSecretID)
}

func TestAutomationCredentialRetryStartUsesCurrentChoicesWithSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	validationErr := errors.New("stop after credential validation")
	source := &automation.Automation{
		ID: "automation-1", WorkspaceID: "workspace-1", CreatedByUserID: "creator",
		CredentialChoices: []automation.CredentialChoiceProjection{{
			Kind: automation.CredentialChoicePersonal, ProfileID: "executor-profile", Key: "EXECUTOR_KEY",
			State: automation.CredentialChoiceSelected, SecretID: "personal-secret", SourceSecretID: "profile-secret",
		}},
	}
	autoSvc := &credentialRunLaunchAutomationService{
		stubAutomationService: &stubAutomationService{automation: source},
		dispatchAutomation:    source, validationErr: validationErr,
	}
	svc.SetAutomationService(autoSvc)
	snapshot := &automation.Automation{
		ID: source.ID, WorkspaceID: source.WorkspaceID, Name: "snapshot name", Prompt: "immutable prompt",
		ExecutorProfileID: "executor-profile", TaskMode: automation.TaskModeAutomationRun,
		ContinuationPolicy: automation.ContinuationPolicyNewTask, Enabled: true,
	}

	_, err := svc.startAutomationTask(ctx, snapshot, &models.Task{ID: "retry-task"}, "step-1", &automationRunLaunch{RunID: "retry-run"})

	require.ErrorIs(t, err, validationErr)
	require.Equal(t, "snapshot name", autoSvc.validatedAutomation.Name)
	require.Equal(t, "immutable prompt", autoSvc.validatedAutomation.Prompt)
	require.Equal(t, source.CreatedByUserID, autoSvc.validatedAutomation.CreatedByUserID)
	require.Equal(t, source.CredentialChoices, autoSvc.validatedAutomation.CredentialChoices)
}

func TestAutomationCredentialRetryRecoveryUsesCurrentChoices(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedAutomationTask(t, repo, "recovered-task", models.TaskOriginAutomationRun, false)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "recovered-session", TaskID: "recovered-task", State: models.TaskSessionStateRunning,
	}))
	current := &automation.Automation{
		ID: "automation-1", WorkspaceID: "ws-recovered-task", CreatedByUserID: "creator",
		CredentialChoices: []automation.CredentialChoiceProjection{{
			Kind: automation.CredentialChoiceWorkspace, RepositoryID: "repo-1", Key: "SERVICE_KEY",
			State: automation.CredentialChoiceRevoked,
		}},
	}
	validationErr := automation.ErrCredentialsUnavailable
	autoSvc := &credentialRunLaunchAutomationService{
		stubAutomationService: &stubAutomationService{automation: current},
		dispatchAutomation:    current, validationErr: validationErr,
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(autoSvc)
	snapshotAutomation := &automation.Automation{
		ID: current.ID, WorkspaceID: current.WorkspaceID, Name: "snapshot name", Prompt: "immutable prompt",
		ExecutorProfileID: "executor-profile", TaskMode: automation.TaskModeAutomationRun,
		ContinuationPolicy: automation.ContinuationPolicyNewTask, Enabled: true,
	}
	run := &automation.AutomationRun{
		ID: "recovered-run", AutomationID: current.ID, TriggerType: automation.TriggerTypeManual,
		ThreadAction: automation.ThreadActionResumed,
	}
	run.RetryLaunchConfigSnapshot = retrySnapshotForTest(run, snapshotAutomation)
	run.RetryLaunchConfigVersion = automation.RetryLaunchConfigVersion
	launch := &automation.RetryLaunch{RunID: run.ID, TaskID: "recovered-task", SessionID: "recovered-session"}

	handled := svc.dispatchReservedRetryLaunch(ctx, run, launch)

	require.True(t, handled)
	require.ErrorIs(t, autoSvc.validationErr, validationErr)
	require.NotNil(t, autoSvc.validatedAutomation)
	require.Equal(t, "snapshot name", autoSvc.validatedAutomation.Name)
	require.Equal(t, current.CreatedByUserID, autoSvc.validatedAutomation.CreatedByUserID)
	require.Equal(t, current.CredentialChoices, autoSvc.validatedAutomation.CredentialChoices)
}

func TestAutomationCredentialScheduledLaunchRecordsUnavailableCreatorOnce(t *testing.T) {
	ctx := context.Background()
	a := &automation.Automation{
		ID: "automation-1", WorkspaceID: "workspace-1", CreatedByUserID: "inactive-creator",
		Enabled: true, TaskMode: automation.TaskModeAutomationRun,
		CredentialChoices: []automation.CredentialChoiceProjection{{
			Kind: automation.CredentialChoicePersonal, ProfileID: "executor-profile", Key: "SERVICE_KEY",
			State: automation.CredentialChoiceSelected, SecretID: "personal-secret",
		}},
	}
	autoSvc := &credentialRunLaunchAutomationService{
		stubAutomationService: &stubAutomationService{automation: a}, dispatchAutomation: a,
	}
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(autoSvc)
	svc.autonomousExecutorContext = func(ctx context.Context, _, _ string) (context.Context, error) {
		return ctx, errors.New("creator account is inactive")
	}

	svc.createAutomationTaskLocked(ctx, &automation.AutomationTriggeredEvent{
		AutomationID: a.ID, TriggerType: automation.TriggerTypeScheduled,
	})

	require.Len(t, autoSvc.runs, 1)
	require.Equal(t, automation.RunStatusFailed, autoSvc.runs[0].Status)
	require.Equal(t, automation.ErrCredentialCreatorUnavailable.Error(), autoSvc.runs[0].ErrorMessage)
}
