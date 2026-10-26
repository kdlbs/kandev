package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func seedExactProfileSessionBinding(t *testing.T, repo *sqliterepo.Repository, session *models.TaskSession) {
	t.Helper()
	_, err := repo.DB().ExecContext(context.Background(), `UPDATE task_sessions
		SET exact_profile_generation = ?, exact_profile_revision = ? WHERE id = ?`,
		session.ExactProfileGeneration, session.ExactProfileRevision, session.ID)
	if err != nil {
		t.Fatalf("seed exact profile session binding: %v", err)
	}
}

func TestPromptTask_RejectsModelOutsideExactProfileAssignment(t *testing.T) {
	for _, supportsInPlaceSwitch := range []bool{true, false} {
		name := "replacement launch"
		if supportsInPlaceSwitch {
			name = "in-place selection"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "task-exact-model", "session-exact-model", "step1")
			profileRevision := time.Now().UTC().Round(0)
			if _, err := repo.AssignExactProfileAssignment(ctx, &models.ExactProfileAssignment{
				TaskID: "task-exact-model", WorkspaceID: "ws1", AgentProfileID: "profile-exact",
				ProfileRevision: profileRevision.UnixNano(), Generation: 1,
			}); err != nil {
				t.Fatalf("assign exact profile: %v", err)
			}

			session, err := repo.GetTaskSession(ctx, "session-exact-model")
			if err != nil {
				t.Fatalf("get session: %v", err)
			}
			session.State = models.TaskSessionStateWaitingForInput
			session.AgentProfileID = "profile-exact"
			session.ExactProfileGeneration = 1
			session.ExactProfileRevision = profileRevision.UnixNano()
			session.AgentProfileSnapshot = map[string]interface{}{"model": "exact-model"}
			if err := repo.UpdateTaskSession(ctx, session); err != nil {
				t.Fatalf("update session: %v", err)
			}
			seedExactProfileSessionBinding(t, repo, session)
			seedExecutorRunning(t, repo, session.ID, session.TaskID, "execution-exact-model")

			var launchCalls int
			agentMgr := &mockAgentManager{
				isAgentRunning:           true,
				setSessionModelSupported: supportsInPlaceSwitch,
				resolveProfileInfo: &executor.AgentProfileInfo{
					ProfileID: "profile-exact", WorkspaceID: "ws1", Enabled: true,
					Revision: profileRevision.UnixNano(), Model: "exact-model",
				},
				launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
					launchCalls++
					return &executor.LaunchAgentResponse{AgentExecutionID: "replacement-execution"}, nil
				},
			}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
			svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
			svc.registerBackgroundWork(session.ID, "tool-call", "execution-exact-model", "background-task")
			if !svc.markForegroundIdle(session.ID) {
				t.Fatal("failed to seed yielded foreground activity")
			}

			_, err = svc.PromptTask(ctx, session.TaskID, session.ID, "continue", "client-selected-model", false, nil, false)
			if !errors.Is(err, ErrExactProfileModelMismatch) {
				t.Fatalf("PromptTask error = %v, want exact profile assignment rejection", err)
			}
			if len(agentMgr.setSessionModelCalls) != 0 {
				t.Fatalf("model selection calls = %#v, want none", agentMgr.setSessionModelCalls)
			}
			if len(agentMgr.startAgentProcessCalls) != 0 || launchCalls != 0 {
				t.Fatalf("replacement startup = (%#v, %d), want none", agentMgr.startAgentProcessCalls, launchCalls)
			}
			if len(agentMgr.capturedPrompts) != 0 {
				t.Fatalf("prompt calls = %#v, want none", agentMgr.capturedPrompts)
			}
			if got := svc.foregroundActivityValue(session.ID); got != v1.ForegroundActivityBackground {
				t.Fatalf("foreground activity = %q, want background after rejection", got)
			}
		})
	}
}

func TestPromptTask_RejectsStaleExactBindingWithoutRequestedModel(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-exact-stale-prompt", "session-exact-stale-prompt", "step1")
	revision := time.Now().UTC().UnixNano()
	for generation := int64(1); generation <= 2; generation++ {
		if _, err := repo.AssignExactProfileAssignment(ctx, &models.ExactProfileAssignment{
			TaskID: "task-exact-stale-prompt", WorkspaceID: "ws1", AgentProfileID: "profile-exact",
			ProfileRevision: revision, Generation: generation,
		}); err != nil {
			t.Fatalf("assign generation %d: %v", generation, err)
		}
	}
	session, err := repo.GetTaskSession(ctx, "session-exact-stale-prompt")
	if err != nil {
		t.Fatal(err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentProfileID = "profile-exact"
	session.ExactProfileGeneration = 1
	session.ExactProfileRevision = revision
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	seedExactProfileSessionBinding(t, repo, session)
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "execution-exact-stale-prompt")
	agentMgr := &mockAgentManager{isAgentRunning: true, resolveProfileInfo: &executor.AgentProfileInfo{
		ProfileID: "profile-exact", WorkspaceID: "ws1", Enabled: true, Revision: revision, Model: "exact-model",
	}}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	_, err = svc.PromptTask(ctx, session.TaskID, session.ID, "continue", "", false, nil, false)
	if !errors.Is(err, ErrExactProfileAssignmentInvalid) {
		t.Fatalf("PromptTask error = %v, want stale exact assignment rejection", err)
	}
	if len(agentMgr.capturedPrompts) != 0 {
		t.Fatalf("dispatched prompts = %#v, want none", agentMgr.capturedPrompts)
	}
}

func TestPromptTask_RejectsExactProfileModelBeforeColdResume(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-exact-model-cold", "session-exact-model-cold", "step1")
	profileRevision := time.Now().UTC().Round(0)
	if _, err := repo.AssignExactProfileAssignment(ctx, &models.ExactProfileAssignment{
		TaskID: "task-exact-model-cold", WorkspaceID: "ws1", AgentProfileID: "profile-exact",
		ProfileRevision: profileRevision.UnixNano(), Generation: 1,
	}); err != nil {
		t.Fatalf("assign exact profile: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-exact-model-cold")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentProfileID = "profile-exact"
	session.ExactProfileGeneration = 1
	session.ExactProfileRevision = profileRevision.UnixNano()
	session.AgentProfileSnapshot = map[string]interface{}{"model": "exact-model"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	seedExactProfileSessionBinding(t, repo, session)
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "execution-exact-model-cold")

	var launchCalls int
	agentMgr := &mockAgentManager{
		resolveProfileInfo: &executor.AgentProfileInfo{
			ProfileID: "profile-exact", WorkspaceID: "ws1", Enabled: true,
			Revision: profileRevision.UnixNano(), Model: "exact-model",
		},
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls++
			return nil, errors.New("resume launch should not run")
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	_, err = svc.PromptTask(ctx, session.TaskID, session.ID, "continue", "client-selected-model", false, nil, false)
	if !errors.Is(err, ErrExactProfileModelMismatch) {
		t.Fatalf("PromptTask error = %v, want exact profile assignment rejection", err)
	}
	if launchCalls != 0 {
		t.Fatalf("cold resume launch calls = %d, want none", launchCalls)
	}
}

func TestAttemptModelSwitchForPrompt_RollsBackForegroundOnLateExactProfileRejection(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-exact-model-late", "session-exact-model-late", "step1")
	profileRevision := time.Now().UTC().Round(0)
	if _, err := repo.AssignExactProfileAssignment(ctx, &models.ExactProfileAssignment{
		TaskID: "task-exact-model-late", WorkspaceID: "ws1", AgentProfileID: "profile-exact",
		ProfileRevision: profileRevision.UnixNano(), Generation: 1,
	}); err != nil {
		t.Fatalf("assign exact profile: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-exact-model-late")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.AgentProfileID = "profile-exact"
	session.ExactProfileGeneration = 1
	session.ExactProfileRevision = profileRevision.UnixNano()
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	seedExactProfileSessionBinding(t, repo, session)
	agentMgr := &mockAgentManager{resolveProfileInfo: &executor.AgentProfileInfo{
		ProfileID: "profile-exact", WorkspaceID: "ws1", Enabled: true,
		Revision: profileRevision.UnixNano(), Model: "exact-model",
	}}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.registerBackgroundWork(session.ID, "tool-call", "execution-exact-model-late", "background-task")
	if !svc.markForegroundIdle(session.ID) {
		t.Fatal("failed to seed yielded foreground activity")
	}
	dispatch := svc.beginForegroundDispatch(session.ID, nil, "execution-exact-model-late")
	if dispatch == nil {
		t.Fatal("failed to begin foreground dispatch")
	}

	_, handled, err := svc.attemptModelSwitchForPrompt(
		ctx, session.TaskID, session.ID, "client-selected-model", "continue", session,
		dispatch, func() error { return nil }, func() error { return nil },
	)
	if !handled || !errors.Is(err, ErrExactProfileModelMismatch) {
		t.Fatalf("attemptModelSwitchForPrompt = (handled %t, err %v), want exact profile rejection", handled, err)
	}
	if got := svc.foregroundActivityValue(session.ID); got != v1.ForegroundActivityBackground {
		t.Fatalf("foreground activity = %q, want background after late rejection", got)
	}
}
