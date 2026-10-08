package orchestrator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func providerRestoredAuggieProfile() *executor.AgentProfileInfo {
	return &executor.AgentProfileInfo{
		ProfileID: "profile-uuid-6f5192e6-634b-4d35-96cf-34f57d6b55cf",
		AgentID:   "agent-uuid-cb5a9b96-75bb-4c75-8812-56663e699e6f",
		AgentName: "auggie", NativeSessionResume: true,
	}
}

func TestValidateSessionRecoverySettingsPolicyAction(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		policy  executor.ResumeSettingsPolicy
		wantErr bool
	}{
		{name: "explicit resume", action: recoveryActionResume, policy: executor.ResumeSettingsPolicyProviderRestored},
		{name: "runtime retry alias", action: recoveryActionRuntimeRetry, policy: executor.ResumeSettingsPolicyProviderRestored, wantErr: true},
		{name: "new branch resume", action: recoveryActionResumeNewBranch, policy: executor.ResumeSettingsPolicyProviderRestored, wantErr: true},
		{name: "fresh start", action: recoveryActionFreshStart, policy: executor.ResumeSettingsPolicyProviderRestored, wantErr: true},
		{name: "relocate and resume", action: models.RecoveryActionRelocateAndResume, policy: executor.ResumeSettingsPolicyProviderRestored, wantErr: true},
		{name: "legacy strict runtime retry", action: recoveryActionRuntimeRetry, policy: executor.ResumeSettingsPolicyStrict},
		{name: "unknown policy", action: recoveryActionResume, policy: executor.ResumeSettingsPolicy("other"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSessionRecoverySettingsPolicyAction(tt.action, tt.policy)
			if tt.wantErr && err == nil {
				t.Fatal("expected settings policy action validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateSessionRecoverySettingsPolicyAction: %v", err)
			}
		})
	}
}

func TestRecoverSessionRejectsProviderRestoredRuntimeRetryBeforeNormalization(t *testing.T) {
	_, err := (&Service{}).RecoverSessionWithSettingsPolicy(
		context.Background(), "task-policy", "session-policy", recoveryActionRuntimeRetry,
		executor.ResumeSettingsPolicyProviderRestored,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires the explicit resume action")
}

func TestValidateProviderRestoredRecoveryEligibility(t *testing.T) {
	tests := []struct {
		name         string
		state        models.TaskSessionState
		profile      *executor.AgentProfileInfo
		missingToken bool
		office       bool
		wantErr      bool
	}{
		{
			name:    "eligible Auggie task",
			state:   models.TaskSessionStateFailed,
			profile: providerRestoredAuggieProfile(),
		},
		{
			name:  "other provider",
			state: models.TaskSessionStateFailed,
			profile: &executor.AgentProfileInfo{
				AgentID:   "agent-uuid-cb5a9b96-75bb-4c75-8812-56663e699e6f",
				AgentName: "claude-acp", NativeSessionResume: true,
			},
			wantErr: true,
		},
		{
			name:  "Auggie passthrough",
			state: models.TaskSessionStateFailed,
			profile: &executor.AgentProfileInfo{
				AgentID:   "agent-uuid-cb5a9b96-75bb-4c75-8812-56663e699e6f",
				AgentName: "auggie", NativeSessionResume: true, CLIPassthrough: true,
			},
			wantErr: true,
		},
		{
			name:    "Office task",
			state:   models.TaskSessionStateFailed,
			profile: providerRestoredAuggieProfile(),
			office:  true,
			wantErr: true,
		},
		{
			name:    "non-failed session",
			state:   models.TaskSessionStateWaitingForInput,
			profile: providerRestoredAuggieProfile(),
			wantErr: true,
		},
		{
			name:         "missing provider token",
			state:        models.TaskSessionStateFailed,
			profile:      providerRestoredAuggieProfile(),
			missingToken: true,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task-policy", "session-policy", tt.state)
			session, err := repo.GetTaskSession(ctx, "session-policy")
			require.NoError(t, err)
			session.AgentProfileID = "profile-policy"
			require.NoError(t, repo.UpdateTaskSession(ctx, session))
			if !tt.missingToken {
				now := time.Now().UTC()
				require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
					ID:               "running-policy",
					SessionID:        session.ID,
					TaskID:           session.TaskID,
					AgentExecutionID: "exec-policy",
					ResumeToken:      "provider-conversation",
					CreatedAt:        now,
					UpdatedAt:        now,
				}))
			}

			agentMgr := &mockAgentManager{resolveProfileInfo: tt.profile}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
			if tt.office {
				task, getTaskErr := repo.GetTask(ctx, session.TaskID)
				require.NoError(t, getTaskErr)
				task.IsFromOffice = true
				svc.repo = ownershipOverrideRepo{
					sessionExecutorStore: repo,
					tasks:                map[string]*models.Task{task.ID: task},
				}
			}

			err = svc.validateProviderRestoredRecoveryEligibility(ctx, session.TaskID, session)
			if tt.wantErr && err == nil {
				t.Fatal("expected provider-restored recovery eligibility error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateProviderRestoredRecoveryEligibility: %v", err)
			}
		})
	}
}

type sessionStateSequenceRepo struct {
	sessionExecutorStore
	states []models.TaskSessionState
	reads  int
}

func (r *sessionStateSequenceRepo) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	session, err := r.sessionExecutorStore.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return session, err
	}
	if r.reads < len(r.states) {
		session.State = r.states[r.reads]
	}
	r.reads++
	return session, nil
}

func TestResumeTaskSessionWithOptionsRechecksProviderRestoredEligibilityUnderAttempt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-policy-boundary", "session-policy-boundary", models.TaskSessionStateFailed)
	session, err := repo.GetTaskSession(ctx, "session-policy-boundary")
	require.NoError(t, err)
	session.AgentProfileID = "profile-policy"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	now := time.Now().UTC()
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "running-policy-boundary",
		SessionID:        session.ID,
		TaskID:           session.TaskID,
		AgentExecutionID: "exec-policy-boundary",
		ResumeToken:      "provider-conversation",
		CreatedAt:        now,
		UpdatedAt:        now,
	}))

	var launchCalls int
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls++
			return &executor.LaunchAgentResponse{AgentExecutionID: "unexpected-launch"}, nil
		},
		resolveProfileInfo: &executor.AgentProfileInfo{
			AgentID:   "agent-uuid-cb5a9b96-75bb-4c75-8812-56663e699e6f",
			AgentName: "auggie", NativeSessionResume: true,
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.repo = &sessionStateSequenceRepo{
		sessionExecutorStore: repo,
		states: []models.TaskSessionState{
			models.TaskSessionStateFailed,
			models.TaskSessionStateFailed,
			models.TaskSessionStateWaitingForInput,
		},
	}

	_, err = svc.ResumeTaskSessionWithOptions(ctx, session.TaskID, session.ID, executor.ResumeOptions{
		SettingsPolicy: executor.ResumeSettingsPolicyProviderRestored,
		Origin:         string(launchOriginManual),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires a failed session")
	require.Zero(t, launchCalls, "a session that became ineligible at attempt admission must not launch")
}

func TestRecoverSessionProviderRestoredRejectsIneligibleSessionsWithoutLaunch(t *testing.T) {
	tests := []struct {
		name         string
		state        models.TaskSessionState
		profile      *executor.AgentProfileInfo
		missingToken bool
		office       bool
	}{
		{name: "other provider", state: models.TaskSessionStateFailed, profile: &executor.AgentProfileInfo{AgentID: "agent-uuid-cb5a9b96-75bb-4c75-8812-56663e699e6f", AgentName: "claude-acp", NativeSessionResume: true}},
		{name: "passthrough profile", state: models.TaskSessionStateFailed, profile: &executor.AgentProfileInfo{AgentID: "agent-uuid-cb5a9b96-75bb-4c75-8812-56663e699e6f", AgentName: "auggie", NativeSessionResume: true, CLIPassthrough: true}},
		{name: "office task", state: models.TaskSessionStateFailed, profile: providerRestoredAuggieProfile(), office: true},
		{name: "nonfailed session", state: models.TaskSessionStateWaitingForInput, profile: providerRestoredAuggieProfile()},
		{name: "missing provider token", state: models.TaskSessionStateFailed, profile: providerRestoredAuggieProfile(), missingToken: true},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			taskID := fmt.Sprintf("task-provider-restored-reject-%d", i)
			sessionID := fmt.Sprintf("session-provider-restored-reject-%d", i)
			seedTaskAndSession(t, repo, taskID, sessionID, tt.state)
			session, err := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			session.AgentProfileID = "profile-provider-restored-reject"
			require.NoError(t, repo.UpdateTaskSession(ctx, session))
			if !tt.missingToken {
				now := time.Now().UTC()
				require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
					ID: fmt.Sprintf("running-provider-restored-reject-%d", i), SessionID: sessionID, TaskID: taskID,
					AgentExecutionID: "exec-provider-restored-reject", ResumeToken: "native-conversation-token",
					CreatedAt: now, UpdatedAt: now,
				}))
			}

			var launchCalls int
			agentMgr := &mockAgentManager{
				resolveProfileInfo: tt.profile,
				launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
					launchCalls++
					return &executor.LaunchAgentResponse{AgentExecutionID: "unexpected-launch"}, nil
				},
			}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
			if tt.office {
				task, getTaskErr := repo.GetTask(ctx, taskID)
				require.NoError(t, getTaskErr)
				task.IsFromOffice = true
				svc.repo = ownershipOverrideRepo{
					sessionExecutorStore: repo,
					tasks:                map[string]*models.Task{taskID: task},
				}
			}
			_, err = svc.RecoverSessionWithSettingsPolicy(
				ctx, taskID, sessionID, recoveryActionResume, executor.ResumeSettingsPolicyProviderRestored,
			)
			require.Error(t, err)
			require.Zero(t, launchCalls)
			sessions, listErr := repo.ListTaskSessions(ctx, taskID)
			require.NoError(t, listErr)
			require.Len(t, sessions, 1, "an ineligible recovery must not silently create a replacement session")
		})
	}
}

func TestRecoverSessionProviderRestoredPolicyReachesLifecycleLaunchRequest(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskID, sessionID := "task-provider-restored-launch", "session-provider-restored-launch"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateExecutor(ctx, &models.Executor{
		ID: "executor-provider-restored-launch", Name: "Worktree", Type: models.ExecutorTypeWorktree,
		Status: models.ExecutorStatusActive, Resumable: true,
	}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repository-provider-restored-launch", WorkspaceID: "ws1", Name: "backend",
		SourceType: "local", LocalPath: t.TempDir(), DefaultBranch: "main", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repository-provider-restored-launch", TaskID: taskID,
		RepositoryID: "repository-provider-restored-launch", BaseBranch: "main", Position: 0,
		CreatedAt: now, UpdatedAt: now,
	}))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "profile-provider-restored-launch"
	session.ExecutorID = "executor-provider-restored-launch"
	session.RepositoryID = "repository-provider-restored-launch"
	session.BaseBranch = "main"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-provider-restored-launch", SessionID: sessionID, TaskID: taskID,
		AgentExecutionID: "exec-before-provider-restored-launch", ResumeToken: "native-conversation-token",
		Resumable: true, CreatedAt: now, UpdatedAt: now,
	}))

	var launched *executor.LaunchAgentRequest
	started := false
	agentMgr := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{
			resolveProfileInfo: providerRestoredAuggieProfile(),
			launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launched = req
				return &executor.LaunchAgentResponse{AgentExecutionID: "exec-provider-restored-launch", Status: v1.AgentStatusStarting}, nil
			},
		},
		repo: repo, sessionID: sessionID, taskID: taskID, onStartCalled: &started,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	response, err := svc.RecoverSessionWithSettingsPolicy(
		ctx, taskID, sessionID, recoveryActionResume, executor.ResumeSettingsPolicyProviderRestored,
	)
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, taskID, response.TaskID)
	require.Equal(t, sessionID, response.SessionID)
	require.True(t, started)
	require.NotNil(t, launched)
	require.Equal(t, executor.ResumeSettingsPolicyProviderRestored, launched.SessionSettingsPolicy)
	require.Equal(t, "native-conversation-token", launched.ACPSessionID)
	require.Equal(t, lifecycle.TaskLaunchScopeTask, launched.TaskScope)
}
