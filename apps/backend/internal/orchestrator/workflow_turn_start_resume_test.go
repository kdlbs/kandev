package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// @covers AC-TASKS-RESUME-PROMPT-QUEUE-001.10
func TestOnTurnStartDuringResumePreservesStarting(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateStarting, ""); err != nil {
		t.Fatalf("mark session starting: %v", err)
	}

	steps := newMockStepGetter()
	steps.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Implement", Position: 0,
		Events: wfmodels.StepEvents{OnTurnStart: []wfmodels.OnTurnStartAction{{
			Type: wfmodels.OnTurnStartMoveToNext,
		}}},
	}
	steps.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Review", Position: 1,
	}
	svc := createTestService(repo, steps, newMockTaskRepo())

	if _, err := svc.ProcessOnTurnStart(ctx, "t1", "s1"); err != nil {
		t.Fatalf("ProcessOnTurnStart: %v", err)
	}

	updatedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("load transitioned task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step2" {
		t.Fatalf("workflow step = %q, want step2", updatedTask.WorkflowStepID)
	}

	updatedSession, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load resumed session: %v", err)
	}
	if updatedSession.State != models.TaskSessionStateStarting {
		t.Fatalf("session state = %s, want STARTING", updatedSession.State)
	}
}

// @covers AC-TASKS-RESUME-PROMPT-QUEUE-001.10
func TestResumeCredentialSnapshotSurvivesTurnStart(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(
		t,
		models.WorkflowProfileSessionStartPolicyReuse,
		models.WorkflowProfileSessionEndPolicyPark,
	)
	session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.RepositoryID = "resume-race-repository"
	if err := fixture.repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("prepare resumable session: %v", err)
	}
	if err := fixture.repo.CreateRepository(ctx, &models.Repository{
		ID: "resume-race-repository", WorkspaceID: "ws1", Name: "widgets",
		SourceType: "local", LocalPath: t.TempDir(), Provider: "github",
		ProviderOwner: "acme", ProviderName: "widgets", DefaultBranch: "main",
		RemoteURL: "https://github.com/acme/widgets.git",
	}); err != nil {
		t.Fatalf("create managed GitHub repository: %v", err)
	}
	if err := fixture.repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "resume-race-task-repository", TaskID: "t1", RepositoryID: "resume-race-repository",
	}); err != nil {
		t.Fatalf("attach repository to task: %v", err)
	}

	fixture.stepGetter.steps["step-a"] = &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", Name: "Implement", Position: 0,
		AgentProfileID: "profile-a",
		Events: wfmodels.StepEvents{OnTurnStart: []wfmodels.OnTurnStartAction{{
			Type: wfmodels.OnTurnStartMoveToNext,
		}}},
	}
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Name: "Review", Position: 1,
		AgentProfileID: "profile-a",
	}

	issuer := &turnStartBarrierCredentialIssuer{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	fixture.svc.executor.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/v1/github/credentials/resolve")
	var launchCalls atomic.Int32
	fixture.agentMgr.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launchCalls.Add(1)
		return &executor.LaunchAgentResponse{
			AgentExecutionID: "resume-race-execution",
			WorktreePath:     req.RepositoryPath,
		}, nil
	}
	processStarted := make(chan struct{}, 1)
	fixture.svc.executor.SetOnAgentProcessStarted(func(context.Context, string, string, string) {
		processStarted <- struct{}{}
	})
	fixture.svc.messageCreator = &mockMessageCreator{}
	dispatchComplete := make(chan struct{}, 1)
	fixture.svc.onQueuedMessageExecutionComplete = func() {
		dispatchComplete <- struct{}{}
	}
	var transitionsBefore int
	if err := fixture.repo.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ?`, "t1").Scan(&transitionsBefore); err != nil {
		t.Fatalf("count initial workflow transitions: %v", err)
	}

	resumeDone := make(chan error, 1)
	go func() {
		_, resumeErr := fixture.svc.executor.ResumeSession(ctx, session, true)
		resumeDone <- resumeErr
	}()
	select {
	case <-issuer.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("resume did not reach the credential boundary")
	}

	if _, err := fixture.svc.ProcessOnTurnStart(ctx, "t1", session.ID); err != nil {
		close(issuer.release)
		t.Fatalf("ProcessOnTurnStart at credential boundary: %v", err)
	}
	userTurnID := "resume-race-user-turn"
	if err := fixture.repo.CreateTurn(ctx, &models.Turn{
		ID: userTurnID, TaskID: "t1", TaskSessionID: session.ID, StartedAt: time.Now().UTC(),
	}); err != nil {
		close(issuer.release)
		t.Fatalf("persist accepted user turn: %v", err)
	}
	const userPrompt = "resume race credential-boundary prompt"
	if err := fixture.repo.CreateMessage(ctx, &models.Message{
		ID: "resume-race-user-message", TaskID: "t1", TaskSessionID: session.ID,
		TurnID: userTurnID, AuthorType: models.MessageAuthorUser, Content: userPrompt,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		close(issuer.release)
		t.Fatalf("persist accepted user message: %v", err)
	}
	if err := fixture.svc.QueueUserPrompt(ctx, "t1", session.ID, userPrompt, "", false, nil, map[string]interface{}{}, true); err != nil {
		close(issuer.release)
		t.Fatalf("queue accepted prompt during resume: %v", err)
	}
	if got := fixture.svc.messageQueue.GetStatus(ctx, session.ID).Count; got != 1 {
		close(issuer.release)
		t.Fatalf("queued prompt count during credential persistence = %d, want 1", got)
	}
	close(issuer.release)
	select {
	case err := <-resumeDone:
		if err != nil {
			t.Fatalf("ResumeSession after turn-start transition: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("resume did not finish after releasing the credential boundary")
	}
	select {
	case <-processStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("resumed process did not start")
	}

	updatedTask, err := fixture.repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("load transitioned task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step-b" {
		t.Fatalf("workflow step = %q, want step-b", updatedTask.WorkflowStepID)
	}
	var transitionCount int
	if err := fixture.repo.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ?`, "t1").Scan(&transitionCount); err != nil {
		t.Fatalf("count workflow transitions: %v", err)
	}
	if transitionCount-transitionsBefore != 1 {
		t.Fatalf("workflow transition count delta = %d, want 1 (before=%d after=%d)", transitionCount-transitionsBefore, transitionsBefore, transitionCount)
	}
	updatedSession, err := fixture.repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("load resumed session: %v", err)
	}
	if updatedSession.State != models.TaskSessionStateStarting {
		t.Fatalf("session state = %s, want STARTING", updatedSession.State)
	}
	if models.StringFromAny(updatedSession.Metadata[models.SessionMetaKeyAgentStartAttemptID]) == "" {
		t.Fatal("resume did not retain its startup-attempt identity")
	}
	if updatedSession.RepositoryID != "resume-race-repository" || updatedSession.AgentProfileID != "profile-a" {
		t.Fatalf("resume recipient identity changed: repository=%q profile=%q", updatedSession.RepositoryID, updatedSession.AgentProfileID)
	}
	running, err := fixture.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	if err != nil {
		t.Fatalf("load resumed runtime identity: %v", err)
	}
	if running.ResumeToken != "acp-session-a" {
		t.Fatalf("ACP conversation identity = %q, want original acp-session-a", running.ResumeToken)
	}
	if launchCalls.Load() != 1 {
		t.Fatalf("agent launch calls = %d, want 1", launchCalls.Load())
	}
	if err := fixture.repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("publish genuine prompt readiness: %v", err)
	}
	fixture.agentMgr.isAgentRunning = true
	identity, err := fixture.svc.messageQueue.ResolveSessionIdentity(ctx, "t1", session.ID)
	if err != nil {
		t.Fatalf("resolve prompt queue identity: %v", err)
	}
	fixture.svc.CheckQueueAdmissionReadiness(ctx, identity)
	select {
	case <-dispatchComplete:
	case <-time.After(5 * time.Second):
		t.Fatal("queued resumed prompt was not dispatched after readiness")
	}
	fixture.agentMgr.mu.Lock()
	dispatchedPrompts := append([]string(nil), fixture.agentMgr.capturedPrompts...)
	fixture.agentMgr.mu.Unlock()
	if len(dispatchedPrompts) != 1 || !strings.Contains(dispatchedPrompts[0], userPrompt) {
		t.Fatalf("dispatched prompts = %#v, want one prompt containing the accepted user input", dispatchedPrompts)
	}
	var userMessageCount int
	if err := fixture.repo.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM task_session_messages WHERE task_id = ? AND task_session_id = ? AND author_type = ? AND content = ?`,
		"t1", session.ID, models.MessageAuthorUser, userPrompt).Scan(&userMessageCount); err != nil {
		t.Fatalf("count persisted user messages: %v", err)
	}
	if userMessageCount != 1 {
		t.Fatalf("persisted user message count = %d, want 1", userMessageCount)
	}
}

// @covers AC-TASKS-RESUME-PROMPT-QUEUE-001.11
func TestResumeTurnStartGenuineFailureRetainsRecovery(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(
		t,
		models.WorkflowProfileSessionStartPolicyReuse,
		models.WorkflowProfileSessionEndPolicyPark,
	)
	session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.RepositoryID = "resume-failure-repository"
	if err := fixture.repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("prepare resumable session: %v", err)
	}
	if err := fixture.repo.CreateRepository(ctx, &models.Repository{
		ID: "resume-failure-repository", WorkspaceID: "ws1", Name: "widgets",
		SourceType: "local", LocalPath: t.TempDir(), Provider: "github",
		ProviderOwner: "acme", ProviderName: "widgets", DefaultBranch: "main",
		RemoteURL: "https://github.com/acme/widgets.git",
	}); err != nil {
		t.Fatalf("create managed GitHub repository: %v", err)
	}
	if err := fixture.repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "resume-failure-task-repository", TaskID: "t1", RepositoryID: "resume-failure-repository",
	}); err != nil {
		t.Fatalf("attach repository to task: %v", err)
	}
	fixture.stepGetter.steps["step-a"] = &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", Name: "Implement", Position: 0,
		AgentProfileID: "profile-a",
		Events: wfmodels.StepEvents{OnTurnStart: []wfmodels.OnTurnStartAction{{
			Type: wfmodels.OnTurnStartMoveToNext,
		}}},
	}
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Name: "Review", Position: 1,
		AgentProfileID: "profile-a",
	}

	issuer := &turnStartBarrierCredentialIssuer{entered: make(chan struct{}), release: make(chan struct{})}
	fixture.svc.executor.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/v1/github/credentials/resolve")
	launchErr := errors.New("resume launch rejected")
	var launchCalls atomic.Int32
	fixture.agentMgr.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launchCalls.Add(1)
		return nil, launchErr
	}

	resumeDone := make(chan error, 1)
	go func() {
		_, resumeErr := fixture.svc.executor.ResumeSession(ctx, session, true)
		resumeDone <- resumeErr
	}()
	select {
	case <-issuer.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("resume did not reach the credential boundary")
	}
	if _, err := fixture.svc.ProcessOnTurnStart(ctx, "t1", session.ID); err != nil {
		close(issuer.release)
		t.Fatalf("ProcessOnTurnStart at credential boundary: %v", err)
	}
	const userPrompt = "retain prompt after genuine resume failure"
	userTurnID := "resume-failure-user-turn"
	if err := fixture.repo.CreateTurn(ctx, &models.Turn{
		ID: userTurnID, TaskID: "t1", TaskSessionID: session.ID, StartedAt: time.Now().UTC(),
	}); err != nil {
		close(issuer.release)
		t.Fatalf("persist accepted user turn: %v", err)
	}
	if err := fixture.repo.CreateMessage(ctx, &models.Message{
		ID: "resume-failure-user-message", TaskID: "t1", TaskSessionID: session.ID,
		TurnID: userTurnID, AuthorType: models.MessageAuthorUser, Content: userPrompt,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		close(issuer.release)
		t.Fatalf("persist accepted user message: %v", err)
	}
	if _, err := fixture.svc.messageQueue.QueueMessage(
		ctx, session.ID, "t1", userPrompt, "", messagequeue.QueuedByUser, false, nil,
	); err != nil {
		close(issuer.release)
		t.Fatalf("retain prompt during resume: %v", err)
	}
	close(issuer.release)
	select {
	case err := <-resumeDone:
		if !errors.Is(err, launchErr) {
			t.Fatalf("ResumeSession error = %v, want launch error %v", err, launchErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("failed resume did not finish after releasing credential boundary")
	}

	updatedTask, err := fixture.repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("load transitioned task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step-b" {
		t.Fatalf("workflow step = %q, want step-b", updatedTask.WorkflowStepID)
	}
	updatedSession, err := fixture.repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("load session after genuine launch failure: %v", err)
	}
	if updatedSession.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %s, want original WAITING_FOR_INPUT state", updatedSession.State)
	}
	if !strings.Contains(updatedSession.ErrorMessage, launchErr.Error()) {
		t.Fatalf("session error = %q, want genuine launch failure", updatedSession.ErrorMessage)
	}
	if got := fixture.svc.messageQueue.GetStatus(ctx, session.ID).Count; got != 1 {
		t.Fatalf("queued user prompt count = %d, want 1 retained prompt", got)
	}
	fixture.agentMgr.mu.Lock()
	dispatchedPrompts := append([]string(nil), fixture.agentMgr.capturedPrompts...)
	fixture.agentMgr.mu.Unlock()
	if len(dispatchedPrompts) != 0 {
		t.Fatalf("prompts dispatched after genuine startup failure = %#v, want none", dispatchedPrompts)
	}
	if launchCalls.Load() != 1 {
		t.Fatalf("agent launch calls = %d, want one failed launch", launchCalls.Load())
	}
}

// @covers AC-TASKS-RESUME-PROMPT-QUEUE-001.10
func TestTurnStartPreparationPreservesConcurrentStartup(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateStarting,
		models.TaskSessionStateRunning,
		models.TaskSessionStateWaitingForInput,
	} {
		t.Run(string(state), func(t *testing.T) {
			ctx := context.Background()
			fixture := newProfileSwitchFixture(
				t,
				models.WorkflowProfileSessionStartPolicyReuse,
				models.WorkflowProfileSessionEndPolicyPark,
			)
			fixture.stepGetter.steps["step-a"] = &wfmodels.WorkflowStep{
				ID: "step-a", WorkflowID: "wf1", Name: "Implement", Position: 0,
				AgentProfileID: "profile-a",
			}
			fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
				ID: "step-b", WorkflowID: "wf1", Name: "Review", Position: 1,
				AgentProfileID: "profile-a",
			}
			if err := fixture.repo.UpdateTaskSessionState(ctx, fixture.current.ID, models.TaskSessionStateStarting, ""); err != nil {
				t.Fatalf("prepare startup snapshot: %v", err)
			}
			staleSnapshot, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
			if err != nil {
				t.Fatalf("load source session snapshot: %v", err)
			}
			if err := fixture.repo.UpdateTaskSessionState(ctx, staleSnapshot.ID, state, "concurrent outcome"); err != nil {
				t.Fatalf("install concurrent recipient state: %v", err)
			}
			fixture.svc.workflowStore = newWorkflowStore(
				fixture.repo, fixture.stepGetter, fixture.agentMgr, noopPublisher, testLogger(), &operationLedger{},
			)

			applied := fixture.svc.applyEngineTransitionWithCommit(
				ctx,
				"t1",
				staleSnapshot,
				engine.HandleResult{Transitioned: true, FromStepID: "step-a", ToStepID: "step-b"},
				engine.TriggerOnTurnStart,
				"",
				false,
				func(commitCtx context.Context) (bool, error) {
					task, err := fixture.repo.GetTask(commitCtx, "t1")
					if err != nil {
						return false, err
					}
					task.WorkflowStepID = "step-b"
					return true, fixture.repo.UpdateTask(commitCtx, task)
				},
			)
			if !applied {
				t.Fatal("engine transition was not applied")
			}

			current, err := fixture.repo.GetTaskSession(ctx, staleSnapshot.ID)
			if err != nil {
				t.Fatalf("load concurrent recipient state: %v", err)
			}
			if current.State != state {
				t.Fatalf("recipient state = %s, want concurrent %s state preserved", current.State, state)
			}
		})
	}
}

// @covers AC-TASKS-RESUME-PROMPT-QUEUE-001.10
func TestTurnStartPreparationPreservesWIPStartup(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(
		t,
		models.WorkflowProfileSessionStartPolicyReuse,
		models.WorkflowProfileSessionEndPolicyPark,
	)
	session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if err := fixture.repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateStarting, ""); err != nil {
		t.Fatalf("mark session starting: %v", err)
	}
	session, err = fixture.repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("reload startup session: %v", err)
	}
	fixture.stepGetter.steps["step-a"] = &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", Name: "Implement", Position: 0,
		AgentProfileID: "profile-a",
	}
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Name: "Review", Position: 1,
		AgentProfileID: "profile-a",
	}

	applied := fixture.svc.applyEngineTransitionWithCommit(
		ctx,
		"t1",
		session,
		engine.HandleResult{Transitioned: true, FromStepID: "step-a", ToStepID: "step-b"},
		engine.TriggerOnTurnStart,
		"",
		false,
		func(commitCtx context.Context) (bool, error) {
			task, err := fixture.repo.GetTask(commitCtx, "t1")
			if err != nil {
				return false, err
			}
			task.WorkflowStepID = "step-b"
			task.QueuedForStepID = "step-b"
			task.WIPAdmitted = false
			return true, fixture.repo.UpdateTask(commitCtx, task)
		},
	)
	if !applied {
		t.Fatal("queued engine transition was not applied")
	}

	current, err := fixture.repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("load queued startup session: %v", err)
	}
	if current.State != models.TaskSessionStateStarting {
		t.Fatalf("session state = %s, want STARTING", current.State)
	}
}

type turnStartBarrierCredentialIssuer struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (i *turnStartBarrierCredentialIssuer) Issue(
	ctx context.Context,
	_ gitcredentials.Scope,
) (gitcredentials.Lease, error) {
	i.once.Do(func() { close(i.entered) })
	select {
	case <-i.release:
		return gitcredentials.Lease{Token: "opaque-resume-lease"}, nil
	case <-ctx.Done():
		return gitcredentials.Lease{}, ctx.Err()
	}
}
