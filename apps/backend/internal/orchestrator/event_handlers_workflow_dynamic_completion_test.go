package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentsettingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type storeBackedWorkflowAgentManager struct {
	*mockAgentManager
	profiles *lifecycle.StoreProfileResolver
}

func (m *storeBackedWorkflowAgentManager) ResolveAgentProfile(
	ctx context.Context,
	profileID string,
) (*executor.AgentProfileInfo, error) {
	profile, err := m.profiles.ResolveProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	return &executor.AgentProfileInfo{
		ProfileID:         profile.ProfileID,
		ProfileName:       profile.ProfileName,
		AgentID:           profile.AgentID,
		AgentName:         profile.AgentName,
		Model:             profile.Model,
		RequireExactModel: profile.RequireExactModel,
	}, nil
}

func newStoreBackedDynamicProfileResolver(t *testing.T, profileID string) *lifecycle.StoreProfileResolver {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	repo, cleanup, err := agentsettingsstore.Provide(db, db, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })
	ctx := context.Background()
	require.NoError(t, repo.CreateAgent(ctx, &agentsettingsmodels.Agent{ID: "dynamic", Name: "dynamic"}))
	require.NoError(t, repo.CreateAgentProfile(ctx, &agentsettingsmodels.AgentProfile{
		ID: profileID, AgentID: "dynamic", Name: "Dynamic profile", Enabled: true,
	}))
	return lifecycle.NewStoreProfileResolver(repo, nil)
}

func useStoreBackedProfileResolver(fixture *profileSwitchFixture, resolver *lifecycle.StoreProfileResolver) {
	fixture.svc.agentManager = &storeBackedWorkflowAgentManager{
		mockAgentManager: fixture.agentMgr,
		profiles:         resolver,
	}
}

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1
// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.5
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.1
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.3
func TestWorkflowDynamicProfileReuse(t *testing.T) {
	const dynamicProfileID = "profile-dynamic"
	ctx := context.Background()

	for _, startPolicy := range []models.WorkflowProfileSessionStartPolicy{
		"", models.WorkflowProfileSessionStartPolicyReuse,
	} {
		name := string(startPolicy)
		if name == "" {
			name = "default"
		}
		t.Run("current session/"+name, func(t *testing.T) {
			fixture := newProfileSwitchFixture(t, startPolicy, models.WorkflowProfileSessionEndPolicyPark)
			fixture.current.AgentProfileID = dynamicProfileID
			fixture.current.ExecutionProfileID = "profile-concrete"
			fixture.current.RouteGeneration = 7
			require.NoError(t, fixture.repo.UpdateTaskSession(ctx, fixture.current))
			useStoreBackedProfileResolver(fixture, newStoreBackedDynamicProfileResolver(t, dynamicProfileID))

			err := fixture.svc.preflightWorkflowStepCredentials(ctx, "t1", fixture.current, &wfmodels.WorkflowStep{
				ID: "step-b", WorkflowID: "wf1", AgentProfileID: dynamicProfileID,
				ProfileSessionStartPolicy: startPolicy,
			})

			require.NoError(t, err)
			persisted, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
			require.NoError(t, err)
			require.Equal(t, dynamicProfileID, persisted.AgentProfileID)
			require.Equal(t, "profile-concrete", persisted.ExecutionProfileID)
			require.Equal(t, int64(7), persisted.RouteGeneration)
		})
	}

	t.Run("parked reusable session", func(t *testing.T) {
		fixture := newProfileSwitchFixture(
			t,
			models.WorkflowProfileSessionStartPolicyReuse,
			models.WorkflowProfileSessionEndPolicyPark,
		)
		parked := &models.TaskSession{
			ID: "session-dynamic", TaskID: "t1", AgentProfileID: dynamicProfileID,
			ExecutorID: "exec-local", ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1",
			State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		require.NoError(t, fixture.repo.CreateTaskSession(ctx, parked))
		useStoreBackedProfileResolver(fixture, newStoreBackedDynamicProfileResolver(t, dynamicProfileID))
		target := &wfmodels.WorkflowStep{ID: "step-b", WorkflowID: "wf1", AgentProfileID: dynamicProfileID}
		source := &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1"}

		policy, selected, err := fixture.svc.exactModelWorkflowStartPolicy(
			ctx,
			"t1",
			fixture.current.ID,
			target,
			source,
			dynamicProfileID,
			models.WorkflowProfileSessionStartPolicyReuse,
		)

		require.NoError(t, err)
		require.Equal(t, models.WorkflowProfileSessionStartPolicyReuse, policy)
		require.Equal(t, parked.ID, selected.ID)
	})
}

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.5
func TestWorkflowDynamicCompletionAdvances(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, "", models.WorkflowProfileSessionEndPolicyPark)
	const dynamicProfileID = "profile-dynamic"
	fixture.current.AgentProfileID = dynamicProfileID
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, fixture.current))
	useStoreBackedProfileResolver(fixture, newStoreBackedDynamicProfileResolver(t, dynamicProfileID))
	fixture.svc.SetWorkflowStepGetter(fixture.stepGetter)
	fixture.stepGetter.steps["step-a"] = &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", Name: "Work", Position: 0,
		Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToNext}}},
	}
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Name: "Review", Position: 1,
		AgentProfileID:            dynamicProfileID,
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	}
	now := time.Now().UTC()
	require.NoError(t, fixture.repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-a", TaskID: "t1", TaskSessionID: fixture.current.ID,
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}))
	fixture.svc.turnService = &repoTurnService{repo: fixture.repo}
	events := &mockEventBus{}
	fixture.svc.eventBus = events

	fixture.svc.handleAgentReady(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: fixture.current.ID, AgentExecutionID: "execution-a",
	})
	fixture.svc.handleCompleteStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "t1", SessionID: fixture.current.ID,
		Data: &lifecycle.AgentStreamEventData{Type: agentEventComplete},
	})

	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, "step-b", task.WorkflowStepID)
	sessions, err := fixture.repo.ListTaskSessions(ctx, "t1")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, fixture.current.ID, sessions[0].ID)
	require.Equal(t, dynamicProfileID, sessions[0].AgentProfileID)
	require.Equal(t, models.TaskSessionStateWaitingForInput, sessions[0].State)
	turn, err := fixture.repo.GetTurn(ctx, "turn-a")
	require.NoError(t, err)
	require.NotNil(t, turn.CompletedAt)
	require.Empty(t, fixture.agentMgr.capturedPrompts, "a destination without auto_start must not launch a prompt")
	require.NotEmpty(t, events.published())
}

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11
// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.14
func TestWorkflowProfileLookupFailureRecoversCompletedSession(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	fixture.svc.SetWorkflowStepGetter(fixture.stepGetter)
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Name: "Review", Position: 1,
	}
	const missingProfileID = "missing-profile"
	fixture.current.AgentProfileID = missingProfileID
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, fixture.current))
	useStoreBackedProfileResolver(fixture, newStoreBackedDynamicProfileResolver(t, "profile-dynamic"))
	fixture.stepGetter.steps["step-b"].AgentProfileID = missingProfileID
	fixture.stepGetter.steps["step-b"].ProfileSessionStartPolicy = models.WorkflowProfileSessionStartPolicyReuse
	fixture.svc.turnService = &repoTurnService{repo: fixture.repo}
	eventBus := &mockEventBus{}
	fixture.svc.eventBus = eventBus
	commitCalls := 0

	applied := fixture.svc.applyEngineTransitionWithCommitMode(
		withWorkflowProfileSwitchGuardHeld(ctx, fixture.current.ID, ""),
		"t1",
		fixture.current,
		engine.HandleResult{Transitioned: true, FromStepID: "step-a", ToStepID: "step-b"},
		engine.TriggerOnTurnComplete,
		"Test",
		transitionLifecycleWithOnEnter,
		func(context.Context) (bool, error) {
			commitCalls++
			return true, nil
		},
	)

	require.False(t, applied)
	require.Zero(t, commitCalls, "rejected completion must not commit the transition")
	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, "step-a", task.WorkflowStepID)
	session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	require.True(t, session.IsPrimary)
	require.Empty(t, fixture.agentMgr.capturedPrompts)
	stateEvents := eventBus.published()
	require.Len(t, stateEvents, 1)
	require.Equal(t, events.TaskSessionStateChanged, stateEvents[0].Subject)
	payload, ok := stateEvents[0].Event.Data.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "t1", payload["task_id"])
	require.Equal(t, fixture.current.ID, payload["session_id"])
	require.Equal(t, string(models.TaskSessionStateRunning), payload["old_state"])
	require.Equal(t, string(models.TaskSessionStateWaitingForInput), payload["new_state"])
}

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.14
func TestWorkflowCredentialPreflightFailureDispatchesFollowUp(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	configureDestinationOnlyCredentialPreflightFailure(t, fixture)
	fixture.svc.turnService = &repoTurnService{repo: fixture.repo}
	fixture.agentMgr.promptDone = make(chan struct{})
	eventBus := &mockEventBus{}
	fixture.svc.eventBus = eventBus
	startedAt := time.Now().UTC()
	require.NoError(t, fixture.repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-completed", TaskID: "t1", TaskSessionID: fixture.current.ID,
		StartedAt: startedAt, CreatedAt: startedAt, UpdatedAt: startedAt,
	}))

	fixture.svc.handleAgentReady(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: fixture.current.ID, AgentExecutionID: "execution-a",
	})
	fixture.svc.handleCompleteStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "t1", SessionID: fixture.current.ID, ExecutionID: "execution-a",
		Data: &lifecycle.AgentStreamEventData{Type: agentEventComplete, TurnID: "turn-completed"},
	})

	assertRejectedCompletionStayedOnSource(t, fixture, "turn-completed")
	assertSessionReadinessPublished(t, eventBus, fixture.current.ID)
	require.Empty(t, fixture.agentMgr.capturedPrompts, "preflight rejection must not launch the destination")
	fixture.agentMgr.isAgentRunning = true
	require.NoError(t, fixture.svc.QueueUserPrompt(ctx, "t1", fixture.current.ID, "follow up", "", false, nil, nil, false))
	select {
	case <-fixture.agentMgr.promptDone:
	case <-time.After(2 * time.Second):
		t.Fatal("follow-up prompt was admitted but not dispatched")
	}

	fixture.agentMgr.mu.Lock()
	capturedPrompts := append([]string(nil), fixture.agentMgr.capturedPrompts...)
	capturedCalls := append([]promptCall(nil), fixture.agentMgr.capturedPromptCalls...)
	fixture.agentMgr.mu.Unlock()
	require.Equal(t, []string{"follow up"}, capturedPrompts)
	require.Len(t, capturedCalls, 1)
	require.Equal(t, "execution-a", capturedCalls[0].ExecutionID)
	turns, err := fixture.repo.ListTurnsBySession(ctx, fixture.current.ID)
	require.NoError(t, err)
	require.Len(t, turns, 2, "the follow-up must start exactly one new turn")
	activeTurn, err := fixture.svc.turnService.GetActiveTurn(ctx, fixture.current.ID)
	require.NoError(t, err)
	require.NotNil(t, activeTurn)
	require.NotEqual(t, "turn-completed", activeTurn.ID)
	require.Zero(t, fixture.svc.messageQueue.GetStatus(ctx, fixture.current.ID).Count, "follow-up queue entry must be consumed")
}

func configureDestinationOnlyCredentialPreflightFailure(t *testing.T, fixture *profileSwitchFixture) {
	t.Helper()
	ctx := context.Background()
	fixture.svc.SetWorkflowStepGetter(fixture.stepGetter)
	fixture.stepGetter.steps["step-a"] = &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", Name: "Work", Position: 0,
		Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToNext}}},
	}
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Name: "Review", Position: 1,
		AgentProfileID: "profile-b", ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	}
	require.NoError(t, fixture.repo.CreateExecutor(ctx, &models.Executor{
		ID: "exec-ssh", Name: "SSH", Type: models.ExecutorTypeSSH, Status: models.ExecutorStatusActive,
	}))
	require.NoError(t, fixture.repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: "ep-source-token", ExecutorID: "exec-ssh", Name: "Source GitHub token",
		Config: map[string]string{"remote_auth_secrets": `{"gh_cli_env":"test-token"}`},
	}))
	require.NoError(t, fixture.repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: "ep-destination", ExecutorID: "exec-ssh", Name: "Destination without GitHub token",
	}))
	fixture.current.ExecutorID = "exec-ssh"
	fixture.current.ExecutorProfileID = "ep-source-token"
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, fixture.current))
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-destination", TaskID: "t1", AgentProfileID: "profile-b",
		ExecutorID: "exec-ssh", ExecutorProfileID: "ep-destination", TaskEnvironmentID: "env-1",
		State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(), IsPrimary: false,
	}))
	fixture.svc.executor.SetGitHubCredentialBroker(
		fakeSwitchSessionCredentialIssuer{}, "https://kandev.example/api/v1/github/credentials/resolve",
	)
	require.NoError(t, fixture.repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-invalid-github", WorkspaceID: "ws1", Name: "Invalid GitHub remote",
		SourceType: "local", Provider: "github", RemoteURL: "https://forge.example/acme/widgets.git",
	}))
	require.NoError(t, fixture.repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-invalid-github", TaskID: "t1", RepositoryID: "repo-invalid-github",
	}))
}

func assertRejectedCompletionStayedOnSource(t *testing.T, fixture *profileSwitchFixture, completedTurnID string) {
	t.Helper()
	task, err := fixture.repo.GetTask(context.Background(), "t1")
	require.NoError(t, err)
	require.Equal(t, "step-a", task.WorkflowStepID)
	source, err := fixture.repo.GetTaskSession(context.Background(), fixture.current.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, source.State)
	require.True(t, source.IsPrimary)
	require.Equal(t, "profile-a", source.AgentProfileID)
	destination, err := fixture.repo.GetTaskSession(context.Background(), "session-destination")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, destination.State)
	require.False(t, destination.IsPrimary)
	turn, err := fixture.repo.GetTurn(context.Background(), completedTurnID)
	require.NoError(t, err)
	require.NotNil(t, turn.CompletedAt, "READY must complete the real active turn")
}

func assertSessionReadinessPublished(t *testing.T, eventBus *mockEventBus, sessionID string) {
	t.Helper()
	for _, published := range eventBus.published() {
		if published.Subject != events.TaskSessionStateChanged {
			continue
		}
		payload, ok := published.Event.Data.(map[string]interface{})
		if ok && payload["session_id"] == sessionID &&
			payload["new_state"] == string(models.TaskSessionStateWaitingForInput) {
			return
		}
	}
	require.Fail(t, "READY recovery must publish WAITING_FOR_INPUT for the original session")
}

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.14
func TestWorkflowPreflightFailurePreservesLiveTurn(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		trigger engine.Trigger
		mode    transitionLifecycleMode
		target  bool
	}{
		{name: "manual move", trigger: engine.Trigger("manual"), mode: transitionLifecycleWithOnEnter, target: true},
		{name: "turn start", trigger: engine.TriggerOnTurnStart, mode: transitionLifecycleOnTurnStart, target: true},
		{name: "guarded decision", trigger: engine.TriggerOnTurnComplete, mode: transitionLifecycleGuardedDecision, target: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
			fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{ID: "step-b", WorkflowID: "wf1", AgentProfileID: "profile-b"}
			fixture.svc.executor.SetGitHubCredentialBroker(
				fakeSwitchSessionCredentialIssuer{}, "https://kandev.example/api/v1/github/credentials/resolve",
			)
			require.NoError(t, fixture.repo.CreateRepository(ctx, &models.Repository{
				ID: "repo1", WorkspaceID: "ws1", Name: "widgets", SourceType: "local",
				Provider: "acme-forge", RemoteURL: "https://forge.example/acme/widgets.git",
			}))
			require.NoError(t, fixture.repo.CreateTaskRepository(ctx, &models.TaskRepository{
				ID: "taskrepo1", TaskID: "t1", RepositoryID: "repo1",
			}))
			now := time.Now().UTC()
			require.NoError(t, fixture.repo.CreateTurn(ctx, &models.Turn{
				ID: "active-turn", TaskID: "t1", TaskSessionID: fixture.current.ID,
				StartedAt: now, CreatedAt: now, UpdatedAt: now,
			}))
			fixture.svc.turnService = &repoTurnService{repo: fixture.repo}
			result := engine.HandleResult{Transitioned: true, FromStepID: "step-a", ToStepID: "step-b"}
			if !testCase.target {
				result.ToStepID = "missing-step"
			}

			fixture.svc.applyEngineTransitionWithCommitMode(
				withWorkflowProfileSwitchGuardHeld(ctx, fixture.current.ID, ""),
				"t1", fixture.current, result, testCase.trigger, "Test", testCase.mode,
				func(context.Context) (bool, error) { return true, nil },
			)

			session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateRunning, session.State)
			task, err := fixture.repo.GetTask(ctx, "t1")
			require.NoError(t, err)
			require.Equal(t, "step-a", task.WorkflowStepID)
			activeTurn, err := (&repoTurnService{repo: fixture.repo}).GetActiveTurn(ctx, fixture.current.ID)
			require.NoError(t, err)
			require.Equal(t, "active-turn", activeTurn.ID)
		})
	}
}

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.14
func TestWorkflowCompletionRecoveryPreservesNewerTurn(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateWaitingForInput,
		models.TaskSessionStateCompleted,
	} {
		t.Run(string(state), func(t *testing.T) {
			ctx := context.Background()
			fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
			fixture.svc.SetWorkflowStepGetter(fixture.stepGetter)
			fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
				ID: "step-b", WorkflowID: "wf1", AgentProfileID: "profile-b",
			}
			fixture.svc.executor.SetGitHubCredentialBroker(
				fakeSwitchSessionCredentialIssuer{}, "https://kandev.example/api/v1/github/credentials/resolve",
			)
			require.NoError(t, fixture.repo.CreateRepository(ctx, &models.Repository{
				ID: "repo1", WorkspaceID: "ws1", Name: "widgets", SourceType: "local",
				Provider: "acme-forge", RemoteURL: "https://forge.example/acme/widgets.git",
			}))
			require.NoError(t, fixture.repo.CreateTaskRepository(ctx, &models.TaskRepository{
				ID: "taskrepo1", TaskID: "t1", RepositoryID: "repo1",
			}))
			fixture.svc.turnService = &repoTurnService{repo: fixture.repo}
			snapshot := *fixture.current
			require.NoError(t, fixture.repo.UpdateTaskSessionState(ctx, fixture.current.ID, state, ""))
			if state == models.TaskSessionStateWaitingForInput {
				now := time.Now().UTC()
				require.NoError(t, fixture.repo.CreateTurn(ctx, &models.Turn{
					ID: "newer-turn", TaskID: "t1", TaskSessionID: fixture.current.ID,
					StartedAt: now, CreatedAt: now, UpdatedAt: now,
				}))
				require.NoError(t, fixture.repo.UpdateTaskSessionState(ctx, fixture.current.ID, models.TaskSessionStateRunning, ""))
			}

			fixture.svc.applyEngineTransitionWithCommitMode(
				withWorkflowProfileSwitchGuardHeld(ctx, fixture.current.ID, ""),
				"t1", &snapshot,
				engine.HandleResult{Transitioned: true, FromStepID: "step-a", ToStepID: "step-b"},
				engine.TriggerOnTurnComplete, "Test", transitionLifecycleWithOnEnter,
				func(context.Context) (bool, error) { return true, nil },
			)

			persisted, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
			require.NoError(t, err)
			if state == models.TaskSessionStateWaitingForInput {
				require.Equal(t, models.TaskSessionStateRunning, persisted.State)
				activeTurn, err := (&repoTurnService{repo: fixture.repo}).GetActiveTurn(ctx, fixture.current.ID)
				require.NoError(t, err)
				require.Equal(t, "newer-turn", activeTurn.ID)
				return
			}
			require.Equal(t, models.TaskSessionStateCompleted, persisted.State)
		})
	}

	t.Run("duplicate completed turn publishes readiness once", func(t *testing.T) {
		ctx := context.Background()
		fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
		fixture.svc.SetWorkflowStepGetter(fixture.stepGetter)
		fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
			ID: "step-b", WorkflowID: "wf1", AgentProfileID: "profile-b",
		}
		fixture.svc.executor.SetGitHubCredentialBroker(
			fakeSwitchSessionCredentialIssuer{}, "https://kandev.example/api/v1/github/credentials/resolve",
		)
		require.NoError(t, fixture.repo.CreateRepository(ctx, &models.Repository{
			ID: "repo1", WorkspaceID: "ws1", Name: "widgets", SourceType: "local",
			Provider: "acme-forge", RemoteURL: "https://forge.example/acme/widgets.git",
		}))
		require.NoError(t, fixture.repo.CreateTaskRepository(ctx, &models.TaskRepository{
			ID: "taskrepo1", TaskID: "t1", RepositoryID: "repo1",
		}))
		fixture.svc.turnService = &repoTurnService{repo: fixture.repo}
		eventBus := &mockEventBus{}
		fixture.svc.eventBus = eventBus
		result := engine.HandleResult{Transitioned: true, FromStepID: "step-a", ToStepID: "step-b"}
		apply := func() bool {
			return fixture.svc.applyEngineTransitionWithCommitMode(
				withWorkflowProfileSwitchGuardHeld(ctx, fixture.current.ID, ""),
				"t1", fixture.current, result, engine.TriggerOnTurnComplete, "Test",
				transitionLifecycleWithOnEnter, func(context.Context) (bool, error) { return true, nil },
			)
		}

		require.False(t, apply())
		require.False(t, apply())
		persisted, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateWaitingForInput, persisted.State)
		task, err := fixture.repo.GetTask(ctx, "t1")
		require.NoError(t, err)
		require.Equal(t, "step-a", task.WorkflowStepID)
		require.Len(t, eventBus.published(), 1)
	})
}
