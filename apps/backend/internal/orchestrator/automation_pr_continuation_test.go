package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

const prTestSubject = "acme/widget#17"

// prContinuationAutomationService adds PR subject mappings to the dispatch-
// rejecting stub so tests can observe preparation without launching agents.
type prContinuationAutomationService struct {
	*rejectingAutomationService
	mappings map[string]string
	setErr   error
	// boundRuns and terminal model the exact run ledger for admitted runs.
	boundRuns map[string]*automation.AutomationRun
	terminal  map[string]string
}

func (s *prContinuationAutomationService) GetRun(_ context.Context, id string) (*automation.AutomationRun, error) {
	return s.boundRuns[id], nil
}

func (s *prContinuationAutomationService) BindRunTask(_ context.Context, runID, taskID, _ string) error {
	s.boundRuns[runID] = &automation.AutomationRun{ID: runID, TaskID: taskID, Status: automation.RunStatusTaskCreated}
	return nil
}

func (s *prContinuationAutomationService) BindRun(_ context.Context, runID, taskID, sessionID, turnID string, _ automation.ThreadAction, _ string) error {
	s.boundRuns[runID] = &automation.AutomationRun{ID: runID, TaskID: taskID, SessionID: sessionID, TurnID: turnID}
	return nil
}

func (s *prContinuationAutomationService) MarkRunTerminal(_ context.Context, runID, _, _ string, status automation.RunStatus, errMsg string) error {
	s.terminal[runID] = string(status) + ": " + errMsg
	return nil
}

func (s *prContinuationAutomationService) MarkRunTerminalByBinding(context.Context, string, string, string, automation.RunStatus, string) error {
	return nil
}

func (s *prContinuationAutomationService) GetPRContinuationTaskID(_ context.Context, _, subject string) (string, error) {
	return s.mappings[subject], nil
}

func (s *prContinuationAutomationService) SetPRContinuationTaskID(_ context.Context, _, subject, taskID string) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.mappings[subject] = taskID
	return nil
}

// recordingPRLifecycle records moves with their one-shot entry options.
type recordingPRLifecycle struct {
	repo          *sqliterepo.Repository
	moves         []string
	moveOptions   []taskservice.MoveTaskOptions
	moveErr       error
	moveResult    *taskservice.MoveTaskResult
	archived      []string
	deleted       []string
	deleteReasons []string
}

func (l *recordingPRLifecycle) MoveTaskWithOptions(
	_ context.Context, _, _, stepID string, _ int, opts taskservice.MoveTaskOptions,
) (*taskservice.MoveTaskResult, error) {
	l.moveOptions = append(l.moveOptions, opts)
	if l.moveErr != nil {
		return nil, l.moveErr
	}
	l.moves = append(l.moves, stepID)
	if l.moveResult != nil {
		return l.moveResult, nil
	}
	return &taskservice.MoveTaskResult{}, nil
}

func (l *recordingPRLifecycle) ArchiveTask(_ context.Context, id string) error {
	l.archived = append(l.archived, id)
	return nil
}

func (l *recordingPRLifecycle) DeleteTaskWithLifecycleAndReason(_ context.Context, id, reason string) error {
	l.deleted = append(l.deleted, id)
	l.deleteReasons = append(l.deleteReasons, reason)
	return nil
}

func perPRAutomation() *automation.Automation {
	return &automation.Automation{
		ID: "a-pr", WorkspaceID: "ws-1", Name: "PR review", Prompt: "Review {{pr.number}}",
		WorkflowID: "wf-1", WorkflowStepID: "step-start", Enabled: true,
		AgentProfileID: "agent-a", ExecutorProfileID: "executor-a",
		ContinuationPolicy: automation.ContinuationPolicyReusePerPR,
		TaskMode:           automation.TaskModeNormalTask, RepositoryMode: automation.RepositoryModeNone,
	}
}

func perPREvent(subject string) *automation.AutomationTriggeredEvent {
	data, _ := json.Marshal(map[string]any{
		"pr_subject": subject, "event": automation.PREventNewCommits, "number": 17,
		"head_sha": "bbb", "previous_head_sha": "aaa",
	})
	return &automation.AutomationTriggeredEvent{
		AutomationID: "a-pr", TriggerID: "trg-pr", TriggerType: automation.TriggerTypeGitHubPR, TriggerData: data,
	}
}

func seedPRTask(t *testing.T, repo *sqliterepo.Repository, taskID, stepID string, state models.TaskSessionState) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: stepID,
		Origin: models.TaskOriginAutomationTask, Title: "PR task",
		Metadata: map[string]interface{}{
			models.MetaKeyAgentProfileID:           "agent-a",
			models.MetaKeyExecutorProfileID:        "executor-a",
			models.MetaKeyAutomationTaskMode:       string(automation.TaskModeNormalTask),
			models.MetaKeyAutomationRepositoryMode: string(automation.RepositoryModeNone),
		},
		CreatedAt: now, UpdatedAt: now,
	}))
	if state == "" {
		return
	}
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: taskID + "-session", TaskID: taskID, State: state, IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}))
}

// TestPRContinuationUsesPrimarySession proves a newer secondary tab neither
// receives the follow-up nor hides a running primary session.
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.6
func TestPRContinuationUsesPrimarySession(t *testing.T) {
	a := perPRAutomation()
	f := newPRFixture(t, a)
	seedPRTask(t, f.repo, "pr-task", "step-done", models.TaskSessionStateRunning)
	later := time.Now().UTC().Add(time.Minute)
	require.NoError(t, f.repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: "secondary", TaskID: "pr-task", State: models.TaskSessionStateWaitingForInput,
		StartedAt: later, UpdatedAt: later,
	}))
	f.auto.mappings[prTestSubject] = "pr-task"

	_, _, _, _, err := f.prepare(t, a, perPREvent(prTestSubject))
	require.ErrorIs(t, err, errPRTaskBusy, "the running primary session decides, not the newer secondary tab")

	busy, err := f.svc.AutomationPRTaskBusy(context.Background(), "pr-task")
	require.NoError(t, err)
	require.True(t, busy)
}

type prFixture struct {
	repo      *sqliterepo.Repository
	svc       *Service
	auto      *prContinuationAutomationService
	creator   *stubReviewTaskCreator
	lifecycle *recordingPRLifecycle
}

func newPRFixture(t *testing.T, a *automation.Automation) *prFixture {
	t.Helper()
	repo := setupTestRepo(t)
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "ws-1", Name: "PR", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	auto := &prContinuationAutomationService{
		rejectingAutomationService: &rejectingAutomationService{
			stubAutomationService: &stubAutomationService{automation: a},
			dispatchErr:           errors.New("dispatch rejected by test"),
		},
		mappings:  map[string]string{},
		boundRuns: map[string]*automation.AutomationRun{},
		terminal:  map[string]string{},
	}
	svc.SetAutomationService(auto)
	creator := &stubReviewTaskCreator{task: &models.Task{ID: "created-task"}}
	svc.reviewTaskCreator = creator
	lifecycle := &recordingPRLifecycle{repo: repo}
	svc.SetAutomationPRTaskLifecycle(lifecycle)
	return &prFixture{repo: repo, svc: svc, auto: auto, creator: creator, lifecycle: lifecycle}
}

func (f *prFixture) prepare(t *testing.T, a *automation.Automation, evt *automation.AutomationTriggeredEvent) (*models.Task, *models.TaskSession, automation.ThreadAction, string, error) {
	t.Helper()
	task, session, action, reasons, err := f.svc.prepareAutomationTask(context.Background(), a, evt, "title", "prompt", map[string]interface{}{})
	return task, session, action, reasons.Thread, err
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.2
func TestPRContinuationCreatesAndMapsFirstTask(t *testing.T) {
	a := perPRAutomation()
	f := newPRFixture(t, a)

	task, session, action, reason, err := f.prepare(t, a, perPREvent(prTestSubject))
	require.NoError(t, err)
	require.Nil(t, session)
	require.Equal(t, "created-task", task.ID)
	require.Equal(t, automation.ThreadActionCreated, action, reason)
	require.Equal(t, "step-start", f.creator.got.WorkflowStepID, "a new PR task starts in the configured start step")
	require.Equal(t, "created-task", f.auto.mappings[prTestSubject])
}

func TestPRContinuationDeletesTaskWhenMappingFails(t *testing.T) {
	a := perPRAutomation()
	f := newPRFixture(t, a)
	f.auto.setErr = errors.New("store down")

	_, _, _, _, err := f.prepare(t, a, perPREvent(prTestSubject))
	require.ErrorContains(t, err, "store down")
	require.Empty(t, f.auto.mappings)
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.3
func TestPRContinuationResumesReadyTaskInAnyStep(t *testing.T) {
	a := perPRAutomation()
	f := newPRFixture(t, a)
	seedPRTask(t, f.repo, "pr-task", "step-done", models.TaskSessionStateWaitingForInput)
	f.auto.mappings[prTestSubject] = "pr-task"

	task, session, action, reason, err := f.prepare(t, a, perPREvent(prTestSubject))
	require.NoError(t, err)
	require.Equal(t, automation.ThreadActionResumed, action, reason)
	require.Equal(t, "pr-task", task.ID)
	require.Equal(t, "pr-task-session", session.ID)
	require.Nil(t, f.creator.got, "a resumed firing creates no task")
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.4
func TestPRContinuationReplacesUnusableTask(t *testing.T) {
	cases := map[string]func(t *testing.T, f *prFixture){
		"missing task": func(t *testing.T, f *prFixture) { f.auto.mappings[prTestSubject] = "gone" },
		"archived task": func(t *testing.T, f *prFixture) {
			seedPRTask(t, f.repo, "pr-task", "step-done", models.TaskSessionStateWaitingForInput)
			require.NoError(t, f.repo.ArchiveTask(context.Background(), "pr-task"))
			f.auto.mappings[prTestSubject] = "pr-task"
		},
		"ended session": func(t *testing.T, f *prFixture) {
			seedPRTask(t, f.repo, "pr-task", "step-done", models.TaskSessionStateCompleted)
			f.auto.mappings[prTestSubject] = "pr-task"
		},
		"no session": func(t *testing.T, f *prFixture) {
			seedPRTask(t, f.repo, "pr-task", "step-done", "")
			f.auto.mappings[prTestSubject] = "pr-task"
		},
		"changed profile": func(t *testing.T, f *prFixture) {
			seedPRTask(t, f.repo, "pr-task", "step-done", models.TaskSessionStateWaitingForInput)
			f.auto.mappings[prTestSubject] = "pr-task"
			f.auto.automation.AgentProfileID = "agent-b"
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			a := perPRAutomation()
			f := newPRFixture(t, a)
			setup(t, f)

			task, _, action, reason, err := f.prepare(t, f.auto.automation, perPREvent(prTestSubject))
			require.NoError(t, err)
			require.Equal(t, automation.ThreadActionReplaced, action, reason)
			require.Contains(t, reason, "replacement")
			require.Equal(t, "created-task", task.ID)
			require.Equal(t, "created-task", f.auto.mappings[prTestSubject])
		})
	}
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.6
func TestPRContinuationFailsRunWhenTaskBecameBusy(t *testing.T) {
	a := perPRAutomation()
	f := newPRFixture(t, a)
	seedPRTask(t, f.repo, "pr-task", "step-done", models.TaskSessionStateRunning)
	f.auto.mappings[prTestSubject] = "pr-task"

	f.svc.createAutomationTask(context.Background(), perPREvent(prTestSubject))

	require.Nil(t, f.creator.got, "a busy PR task is never duplicated")
	require.Equal(t, "pr-task", f.auto.mappings[prTestSubject])
	require.Len(t, f.auto.runs, 1)
	require.Equal(t, automation.RunStatusFailed, f.auto.runs[0].Status)
	require.Contains(t, f.auto.runs[0].ErrorMessage, "busy")
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.7
func TestPRContinuationNonPRFiringUsesNewTask(t *testing.T) {
	a := perPRAutomation()
	f := newPRFixture(t, a)

	task, _, action, _, err := f.prepare(t, a, &automation.AutomationTriggeredEvent{
		AutomationID: "a-pr", TriggerID: "manual", TriggerType: automation.TriggerTypeScheduled,
	})
	require.NoError(t, err)
	require.Equal(t, automation.ThreadActionCreated, action)
	require.Equal(t, "created-task", task.ID)
	require.Empty(t, f.auto.mappings, "a firing without a PR subject is not mapped")
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008.1
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008.2
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008.3
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.9
func TestPRContinuationPromptSelection(t *testing.T) {
	a := perPRAutomation()
	evt := perPREvent(prTestSubject)
	require.Equal(t, "main", prContinuationPrompt(a, evt, nil, false, "main"), "empty follow-up uses the main prompt")

	a.FollowUpPrompt = "Review {{pr.previous_head_sha}}..{{pr.head_sha}}"
	require.Equal(t, "Review aaa..bbb", prContinuationPrompt(a, evt, nil, false, "main"),
		"the follow-up prompt uses the main prompt's agent interpolation")

	snapshot := &automation.RetryLaunchConfigSnapshot{ResolvedFollowUpPrompt: "resolved follow-up"}
	require.Equal(t, "resolved follow-up", prContinuationPrompt(a, evt, snapshot, true, "main"),
		"a retry uses the snapshot's resolved follow-up prompt")

	a.ContinuationPolicy = automation.ContinuationPolicyReuseThread
	require.Equal(t, "main", prContinuationPrompt(a, evt, nil, false, "main"), "only reuse_per_pr has a follow-up prompt")

	restored := automationFromRetrySnapshot(automation.RetryLaunchConfigSnapshot{
		ContinuationPolicy: automation.ContinuationPolicyReusePerPR,
		FollowUpPrompt:     "f", PRContinuationStepID: "review",
	})
	require.Equal(t, "f", restored.FollowUpPrompt)
	require.Equal(t, "review", restored.PRContinuationStepID)
}

func TestAutomationPRTaskBusyClassifiesSessions(t *testing.T) {
	cases := map[models.TaskSessionState]bool{
		models.TaskSessionStateRunning:         true,
		models.TaskSessionStateStarting:        true,
		models.TaskSessionStateCreated:         true,
		models.TaskSessionStateWaitingForInput: false,
		models.TaskSessionStateIdle:            false,
		models.TaskSessionStateCompleted:       false,
	}
	for state, want := range cases {
		t.Run(string(state), func(t *testing.T) {
			f := newPRFixture(t, perPRAutomation())
			seedPRTask(t, f.repo, "pr-task", "step-done", state)
			busy, err := f.svc.AutomationPRTaskBusy(context.Background(), "pr-task")
			require.NoError(t, err)
			require.Equal(t, want, busy)
		})
	}
}

// A per-PR follow-up recovered after a restart sends the snapshot's resolved
// follow-up instructions, not the initial ones.
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008.2
func TestRecoveredContinuationPromptUsesFollowUp(t *testing.T) {
	snapshot := automation.RetryLaunchConfigSnapshot{
		ContinuationPolicy:     automation.ContinuationPolicyReusePerPR,
		FollowUpPrompt:         "Review {{pr.head_sha}}",
		ResolvedPrompt:         "initial instructions",
		ResolvedFollowUpPrompt: "Review bbb",
	}
	require.Equal(t, "Review bbb", recoveredContinuationPrompt(snapshot))

	snapshot.ContinuationPolicy = automation.ContinuationPolicyReuseThread
	require.Equal(t, "initial instructions", recoveredContinuationPrompt(snapshot),
		"a shared-task continuation keeps the main prompt")
}
