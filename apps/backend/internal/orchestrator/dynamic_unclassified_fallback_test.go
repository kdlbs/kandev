package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	_ "github.com/mattn/go-sqlite3"
)

func TestUnclassifiedFallbackEvidenceAdmission(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-unclassified-prompt"
		sessionID   = "session-unclassified-prompt"
		executionID = "execution-unclassified-prompt"
	)
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, "step-1")
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-1"] = &wfmodels.WorkflowStep{ID: "step-1", WorkflowID: "wf1", Name: "Work"}
	svc := createTestServiceWithScheduler(repo, stepGetter, newMockTaskRepo(), &mockAgentManager{})
	profile := unclassifiedAdmissionProfile(2)
	engine, decision := seedUnclassifiedAdmissionRoute(t, repo, ctx, profile, taskID, sessionID, executionID)
	svc.beginPromptAttempt(sessionID, executionID, 7, true)
	providerError := &streams.ProviderError{
		Source: streams.ProviderErrorSourceACPPrompt, ProviderID: "provider-x",
		Message: "provider returned an unsupported terminal response", DiagnosticIdentityComplete: true,
		OccurredAt: time.Now().UTC(),
	}
	data := watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, OwnerKind: "task", AgentExecutionID: executionID,
		PromptGeneration: 7, DynamicRouteAttempt: true, EvidenceKnown: true, ProviderError: providerError,
	}
	evidence := svc.unclassifiedPromptEvidence(ctx, data, mustTaskSession(t, repo, ctx, sessionID))
	if !evidence.TaskScope || !evidence.StepKnown || evidence.StepVeto || !evidence.CurrentAttempt || !evidence.DiagnosticComplete {
		t.Fatalf("trusted prompt evidence = %+v, want complete current task evidence", evidence)
	}
	failure := classifyKanbanFailure(data)
	if failure.Code != routingerr.CodeUnknownProvider || failure.Class != routingerr.ClassUnclassified {
		t.Fatalf("terminal provider classification = %+v, want unclassified unknown provider", failure)
	}
	if failure.FallbackAllowed || failure.AutoRetryable {
		t.Fatalf("terminal unknown classification widened global routing flags: %+v", failure)
	}
	decision, err := engine.ApplyUnclassifiedFailureContext(ctx, sessionID, profile, decision.Generation, "candidate-a", failure, evidence)
	if !errors.Is(err, dynamicruntime.ErrRecoveryPending) {
		t.Fatalf("trusted prompt failure error = %v, want manual recovery before threshold", err)
	}
	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	var policyState dynamicruntime.PolicyState
	if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policyState); err != nil {
		t.Fatalf("decode route policy state: %v", err)
	}
	if policyState.Unclassified == nil || policyState.Unclassified.Count != 1 {
		t.Fatalf("durable prompt streak = %+v, want one", policyState.Unclassified)
	}
	stepGetter.steps["step-1"].DisableUnclassifiedFallback = true
	svc.beginPromptAttempt(sessionID, executionID, 8, true)
	data.PromptGeneration = 8
	vetoEvidence := svc.unclassifiedPromptEvidence(ctx, data, mustTaskSession(t, repo, ctx, sessionID))
	if !vetoEvidence.StepKnown || !vetoEvidence.StepVeto {
		t.Fatalf("workflow veto evidence = %+v, want a known veto", vetoEvidence)
	}
	if vetoEvidence.AttemptID == policyState.Unclassified.LastAttemptID {
		t.Fatalf("workflow veto reused attempt identity %q", vetoEvidence.AttemptID)
	}
	_, vetoErr := engine.ApplyUnclassifiedFailureContext(ctx, sessionID, profile, decision.Generation, "candidate-a", failure, vetoEvidence)
	if !errors.Is(vetoErr, dynamicruntime.ErrRecoveryPending) {
		t.Fatalf("vetoed failure error = %v, want manual recovery", vetoErr)
	}
	state, err = repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState after veto: %v", err)
	}
	policyState = dynamicruntime.PolicyState{}
	if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policyState); err != nil {
		t.Fatalf("decode route policy state after veto: %v", err)
	}
	if policyState.Unclassified != nil {
		t.Fatalf("workflow veto retained streak: %+v", policyState.Unclassified)
	}
	if decision.ExecutionProfileID != "candidate-a" {
		t.Fatalf("pre-threshold candidate = %q, want candidate-a", decision.ExecutionProfileID)
	}
}

func TestPersistPendingDynamicRecoveryLeavesTurnForTerminalFailureCleanup(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-pending-dynamic-recovery", "session-pending-dynamic-recovery", models.TaskSessionStateRunning)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.turnService = &repoTurnService{repo: repo}
	eventBus := &recordingEventBus{}
	svc.eventBus = eventBus

	turn, err := svc.turnService.StartTurn(ctx, "session-pending-dynamic-recovery")
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	session := mustTaskSession(t, repo, ctx, "session-pending-dynamic-recovery")
	decision := dynamicruntime.RouteDecision{
		SessionID:  session.ID,
		Generation: 1,
		Status:     "action_required",
	}
	if !svc.persistPendingDynamicRecovery(ctx, session, decision, dynamicruntime.ErrRecoveryPending) {
		t.Fatal("persistPendingDynamicRecovery returned false")
	}

	activeTurn, err := svc.turnService.GetActiveTurn(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetActiveTurn: %v", err)
	}
	if activeTurn == nil || activeTurn.ID != turn.ID {
		t.Fatalf("active turn = %#v, want the failed turn to remain for terminal recovery cleanup", activeTurn)
	}
	settled, err := svc.turnService.GetTurn(ctx, turn.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if settled.CompletedAt != nil {
		t.Fatal("pending route persistence completed the turn before the terminal failure owner could settle it")
	}
	if terminated, _ := settled.Metadata[models.TurnMetaKeyErrorTerminated].(bool); terminated {
		t.Fatalf("turn metadata = %#v, want terminal failure cleanup to own error_terminated", settled.Metadata)
	}
	if len(eventBus.events) != 1 || eventBus.events[0].subject != events.TaskSessionStateChanged {
		t.Fatalf("published events = %#v, want one session state event for manual recovery", eventBus.events)
	}
	eventData, ok := eventBus.events[0].event.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data = %T, want map[string]interface{}", eventBus.events[0].event.Data)
	}
	if eventData["route_state"] != "action_required" {
		t.Fatalf("event route_state = %#v, want action_required", eventData["route_state"])
	}
}

func TestHandleAgentFailedManualDynamicRecoveryRunsTerminalCleanup(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		enabled   bool
		threshold int64
	}{
		{name: "disabled policy"},
		{name: "enabled policy below threshold", enabled: true, threshold: 3},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			const (
				taskID      = "task-manual-unclassified-terminal"
				sessionID   = "session-manual-unclassified-terminal"
				executionID = "execution-manual-unclassified-terminal"
				profileID   = "profile-manual-unclassified-terminal"
			)
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
			task, err := repo.GetTask(ctx, taskID)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			task.Origin = models.TaskOriginAutomationRun
			if err := repo.UpdateTask(ctx, task); err != nil {
				t.Fatalf("UpdateTask: %v", err)
			}

			document := routingpolicy.DefaultDocument()
			if testCase.enabled {
				document.Unclassified = &routingpolicy.UnclassifiedPolicy{
					Enabled: true, ConsecutiveFailureThreshold: testCase.threshold,
				}
			}
			rawPolicy, err := json.Marshal(document)
			if err != nil {
				t.Fatalf("marshal policy: %v", err)
			}
			resolver := newWorkflowDynamicProfileResolverWithCandidates(t, profileID, []workflowDynamicCandidate{
				{executionProfileID: "candidate-a", enabled: true, rulesJSON: string(rawPolicy)},
				{executionProfileID: "candidate-b", enabled: true},
			}, dynamicruntime.WithPersistence(repo), dynamicruntime.WithStateLoader(repo))
			profile := unclassifiedAdmissionProfile(2)
			profile.ID = profileID
			_, route := seedUnclassifiedAdmissionRoute(t, repo, ctx, profile, taskID, sessionID, executionID)

			taskRepo := newMockTaskRepo()
			seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
			agentManager := &mockAgentManager{repoForExecutionLookup: repo}
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
			svc.SetProfileExecutionResolver(resolver)
			autoSvc := &stubAutomationService{}
			svc.SetAutomationService(autoSvc)
			svc.turnService = &repoTurnService{repo: repo}

			queue := newAuthoritativeMemoryQueue(repo, testLogger())
			storage := queue.ManagedInputStorage()
			svc.messageQueue = queue
			svc.SetManagedInputStorage(storage)
			identity, err := queue.ResolveSessionIdentity(ctx, taskID, sessionID)
			if err != nil {
				t.Fatalf("ResolveSessionIdentity: %v", err)
			}
			turn, err := svc.turnService.StartTurn(ctx, sessionID)
			if err != nil {
				t.Fatalf("StartTurn: %v", err)
			}
			_, _, err = storage.AdmitManagedInput(ctx, identity, messagequeue.ManagedInputRequest{
				ID: "managed-input-manual-unclassified", OccurrenceKey: "manual-unclassified",
				PayloadDigest: "sha256:manual-unclassified", Payload: "do the task",
				Origin: messagequeue.ManagedInputOriginHuman, ConversationRevision: 1,
			}, 10)
			if err != nil {
				t.Fatalf("AdmitManagedInput: %v", err)
			}
			if _, _, err := storage.MarkManagedInputRunning(ctx, identity, "managed-input-manual-unclassified", turn.ID, executionID); err != nil {
				t.Fatalf("MarkManagedInputRunning: %v", err)
			}

			activity := svc.lockTurnActivity(sessionID, true)
			activity.executionClaims[executionID] = struct{}{}
			activity.mu.Unlock()

			const promptGeneration = 7
			svc.beginPromptAttempt(sessionID, executionID, promptGeneration, true)
			data := watcher.AgentEventData{
				TaskID: taskID, SessionID: sessionID, OwnerKind: "task", AgentExecutionID: executionID,
				PromptGeneration: promptGeneration, DynamicRouteAttempt: true, EvidenceKnown: true,
				ProviderError: &streams.ProviderError{
					Source: streams.ProviderErrorSourceACPPrompt, ProviderID: "provider-x",
					Message: "provider returned an unsupported terminal response", DiagnosticIdentityComplete: true,
					OccurredAt: time.Now().UTC(),
				},
				ErrorMessage: "provider returned an unsupported terminal response",
			}
			failure := classifyKanbanFailure(data)
			evidence := svc.unclassifiedPromptEvidence(ctx, data, mustTaskSession(t, repo, ctx, sessionID))
			if _, routeErr := resolver.RouteAfterUnclassifiedFailure(
				ctx, sessionID, profileID, "candidate-a", route.Generation, failure, evidence,
			); !errors.Is(routeErr, dynamicruntime.ErrRecoveryPending) {
				t.Fatalf("seed manual route state: %v, want ErrRecoveryPending", routeErr)
			}
			lock, release := svc.acquireCancelInFlightGuard(sessionID)
			lock.Lock()
			dispatch := svc.handleAgentFailedLocked(ctx, data)
			lock.Unlock()
			release()
			if dispatch != nil {
				dispatch(ctx)
			}

			if got := autoSvc.failed[taskID]; got != data.ErrorMessage {
				t.Fatalf("automation failure = %q, want %q", got, data.ErrorMessage)
			}
			currentSession := mustTaskSession(t, repo, ctx, sessionID)
			if currentSession.State != models.TaskSessionStateWaitingForInput || currentSession.RouteState != "action_required" {
				t.Fatalf("session after terminal recovery = state %q route %q, want waiting_for_input/action_required", currentSession.State, currentSession.RouteState)
			}
			activeTurn, err := svc.turnService.GetActiveTurn(ctx, sessionID)
			if err != nil {
				t.Fatalf("GetActiveTurn: %v", err)
			}
			if activeTurn != nil {
				t.Fatalf("active turn = %q, want terminal recovery to settle it", activeTurn.ID)
			}
			receipt, err := storage.GetManagedInput(ctx, identity, "managed-input-manual-unclassified")
			if err != nil {
				t.Fatalf("GetManagedInput: %v", err)
			}
			if receipt.State != messagequeue.ManagedInputStateFailed {
				t.Fatalf("managed input state = %q, want failed after proven no-effect failure", receipt.State)
			}
			svc.foregroundActivityMu.Lock()
			_, activityStillPresent := svc.foregroundActivity[sessionID]
			svc.foregroundActivityMu.Unlock()
			if activityStillPresent {
				t.Fatal("failed execution activity was not retired")
			}
			if len(agentManager.startAgentProcessCalls) != 0 {
				t.Fatalf("automatic successor starts = %d, want none below threshold", len(agentManager.startAgentProcessCalls))
			}
			routeState, err := repo.LoadRouteState(ctx, sessionID)
			if err != nil {
				t.Fatalf("LoadRouteState: %v", err)
			}
			if routeState.Generation != 1 || routeState.ExecutionProfileID != "candidate-a" || routeState.Status != "action_required" {
				t.Fatalf("route state = %+v, want candidate-a generation 1 action_required", routeState)
			}
			var policyState dynamicruntime.PolicyState
			if err := json.Unmarshal([]byte(routeState.PolicyStateJSON), &policyState); err != nil {
				t.Fatalf("decode policy state: %v", err)
			}
			if testCase.enabled && (policyState.Unclassified == nil || policyState.Unclassified.Count != 1) {
				t.Fatalf("below-threshold streak = %+v, want preserved count 1", policyState.Unclassified)
			}
			if !testCase.enabled && policyState.Unclassified != nil {
				t.Fatalf("disabled policy streak = %+v, want none", policyState.Unclassified)
			}
		})
	}
}

func TestDynamicUnclassifiedStreakResetsAfterClassifiedNonFallbackFailure(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-unclassified-mixed-streak"
		sessionID = "session-unclassified-mixed-streak"
		profileID = "profile-unclassified-mixed-streak"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	profile := unclassifiedAdmissionProfile(2)
	profile.ID = profileID
	policyJSON, err := json.Marshal(profile.Candidates[0].Policies)
	if err != nil {
		t.Fatalf("marshal candidate policy: %v", err)
	}
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, profileID, []workflowDynamicCandidate{
		{executionProfileID: "candidate-a", enabled: true, rulesJSON: string(policyJSON)},
		{executionProfileID: "candidate-b", enabled: true},
	}, dynamicruntime.WithPersistence(repo), dynamicruntime.WithStateLoader(repo))
	_, initial := seedUnclassifiedAdmissionRoute(t, repo, ctx, profile, taskID, sessionID, "execution-one")
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	svc.SetProfileExecutionResolver(resolver)
	defer svc.stopDynamicSuccessorWorkers()

	unknownFailure := func(executionID string, promptGeneration uint64) watcher.AgentEventData {
		svc.beginPromptAttempt(sessionID, executionID, promptGeneration, true)
		return watcher.AgentEventData{
			TaskID: taskID, SessionID: sessionID, OwnerKind: "task", AgentExecutionID: executionID,
			PromptGeneration: promptGeneration, DynamicRouteAttempt: true, EvidenceKnown: true,
			ProviderError: &streams.ProviderError{
				Source: streams.ProviderErrorSourceACPPrompt, ProviderID: "provider-x",
				Message: "provider returned an unsupported terminal response", DiagnosticIdentityComplete: true,
				OccurredAt: time.Now().UTC(),
			},
		}
	}
	first := unknownFailure("execution-one", 1)
	if handled := svc.routeDynamicAgentFailure(ctx, first, classifyKanbanFailure(first)); handled {
		t.Fatal("first matching unknown failure unexpectedly selected a successor below threshold")
	}

	resumeCandidate := func(executionID string) {
		t.Helper()
		resumed, err := resolver.ResolveRouteAction(
			ctx, sessionID, profileID, "candidate-a", initial.Generation, "retry",
		)
		if err != nil {
			t.Fatalf("retry current candidate: %v", err)
		}
		session := mustTaskSession(t, repo, ctx, sessionID)
		session.AgentExecutionID = executionID
		session.RouteGeneration = resumed.Generation
		session.RouteState = resumed.Decision.Status
		session.State = models.TaskSessionStateRunning
		if err := repo.UpdateTaskSession(ctx, session); err != nil {
			t.Fatalf("UpdateTaskSession retry: %v", err)
		}
	}

	resumeCandidate("execution-two")
	svc.beginPromptAttempt(sessionID, "execution-two", 2, true)
	classified := &routingerr.Error{
		Code: routingerr.CodeAgentTransportLost, Class: routingerr.ClassTransient,
		FallbackAllowed: false, AutoRetryable: true,
	}
	classifiedData := watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, OwnerKind: "task", AgentExecutionID: "execution-two",
		PromptGeneration: 2, DynamicRouteAttempt: true, EvidenceKnown: true,
	}
	if handled := svc.routeDynamicAgentFailure(ctx, classifiedData, classified); handled {
		t.Fatal("classified non-fallback failure unexpectedly routed to a successor")
	}

	resumeCandidate("execution-three")
	third := unknownFailure("execution-three", 3)
	if handled := svc.routeDynamicAgentFailure(ctx, third, classifyKanbanFailure(third)); handled {
		t.Fatal("same unknown failure reached the fallback threshold across an intervening classified failure")
	}
	routeState, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if routeState.Generation != initial.Generation || routeState.ExecutionProfileID != "candidate-a" || routeState.Status != "action_required" {
		t.Fatalf("route after mixed failure sequence = %+v, want candidate-a manual recovery without advancement", routeState)
	}
	var policyState dynamicruntime.PolicyState
	if routeState.PolicyStateJSON != "" {
		if err := json.Unmarshal([]byte(routeState.PolicyStateJSON), &policyState); err != nil {
			t.Fatalf("decode policy state: %v", err)
		}
	}
	if policyState.Unclassified == nil || policyState.Unclassified.Count != 1 {
		t.Fatalf("streak after mixed failure sequence = %+v, want reset then count 1", policyState.Unclassified)
	}
}

func unclassifiedAdmissionProfile(threshold int64) dynamicruntime.Profile {
	document := routingpolicy.DefaultDocument()
	document.Unclassified = &routingpolicy.UnclassifiedPolicy{Enabled: true, ConsecutiveFailureThreshold: threshold}
	return dynamicruntime.Profile{ID: "dynamic-unclassified", Version: 1, Candidates: []dynamicruntime.Candidate{
		{ID: "candidate-a", Enabled: true, Policies: document},
		{ID: "candidate-b", Enabled: true},
	}}
}

func seedUnclassifiedAdmissionRoute(
	t *testing.T,
	repo *sqliterepo.Repository,
	ctx context.Context,
	profile dynamicruntime.Profile,
	taskID, sessionID, executionID string,
) (*dynamicruntime.Engine, dynamicruntime.RouteDecision) {
	t.Helper()
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	decision, err := engine.Select(sessionID, profile, 0, "")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if err := engine.MarkActive(ctx, sessionID, decision.Generation); err != nil {
		t.Fatalf("MarkActive: %v", err)
	}
	session := mustTaskSession(t, repo, ctx, sessionID)
	session.AgentProfileID = profile.ID
	session.ExecutionProfileID = decision.ExecutionProfileID
	session.RouteGeneration = decision.Generation
	session.RouteState = "active"
	session.AgentExecutionID = executionID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}
	return engine, decision
}

func mustTaskSession(t *testing.T, repo *sqliterepo.Repository, ctx context.Context, sessionID string) *models.TaskSession {
	t.Helper()
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session == nil {
		t.Fatalf("GetTaskSession(%q) returned nil", sessionID)
	}
	return session
}
