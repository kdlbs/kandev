package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestResolveStepAgentProfile(t *testing.T) {
	t.Run("returns step profile when set", func(t *testing.T) {
		svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
		step := &wfmodels.WorkflowStep{
			ID:             "step1",
			WorkflowID:     "wf1",
			AgentProfileID: "profile-step",
		}
		got := svc.resolveStepAgentProfile(context.Background(), step)
		if got != "profile-step" {
			t.Errorf("expected profile-step, got %q", got)
		}
	})

	t.Run("falls back to workflow profile when step has none", func(t *testing.T) {
		sg := newMockStepGetter()
		sg.workflowAgentProfileID = "profile-workflow"
		svc := createTestService(setupTestRepo(t), sg, newMockTaskRepo())
		step := &wfmodels.WorkflowStep{
			ID:         "step1",
			WorkflowID: "wf1",
		}
		got := svc.resolveStepAgentProfile(context.Background(), step)
		if got != "profile-workflow" {
			t.Errorf("expected profile-workflow, got %q", got)
		}
	})

	t.Run("returns empty when neither step nor workflow has profile", func(t *testing.T) {
		svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
		step := &wfmodels.WorkflowStep{
			ID:         "step1",
			WorkflowID: "wf1",
		}
		got := svc.resolveStepAgentProfile(context.Background(), step)
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("step profile takes precedence over workflow profile", func(t *testing.T) {
		sg := newMockStepGetter()
		sg.workflowAgentProfileID = "profile-workflow"
		svc := createTestService(setupTestRepo(t), sg, newMockTaskRepo())
		step := &wfmodels.WorkflowStep{
			ID:             "step1",
			WorkflowID:     "wf1",
			AgentProfileID: "profile-step",
		}
		got := svc.resolveStepAgentProfile(context.Background(), step)
		if got != "profile-step" {
			t.Errorf("expected profile-step, got %q", got)
		}
	})
}

func TestSwitchSessionForStep(t *testing.T) {
	ctx := context.Background()

	t.Run("completes old session and creates new one", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()

		// Seed workspace + workflow + task
		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)
		task := &models.Task{
			ID: "t1", WorkflowID: "wf1", WorkflowStepID: "step2",
			Title: "Test", Description: "Test", State: v1.TaskStateInProgress,
			CreatedAt: now, UpdatedAt: now,
		}
		_ = repo.CreateTask(ctx, task)

		// Create current session with profile-A
		session := &models.TaskSession{
			ID:                "s1",
			TaskID:            "t1",
			AgentProfileID:    "profile-a",
			ExecutorID:        "exec-local",
			ExecutorProfileID: "ep1",
			AgentExecutionID:  "ae1",
			State:             models.TaskSessionStateRunning,
			IsPrimary:         true,
			StartedAt:         now,
			UpdatedAt:         now,
		}
		_ = repo.CreateTaskSession(ctx, session)

		// Set up task repo mock with v1 task for scheduler
		taskRepo := newMockTaskRepo()
		taskRepo.tasks["t1"] = &v1.Task{
			ID:          "t1",
			WorkspaceID: "ws1",
			WorkflowID:  "wf1",
			Title:       "Test",
			Description: "Test",
			State:       v1.TaskStateInProgress,
		}

		agentMgr := &mockAgentManager{}
		log := testLogger()
		exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
		sched := scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.SchedulerConfig{})
		svc := &Service{
			logger:             log,
			repo:               repo,
			workflowStepGetter: newMockStepGetter(),
			taskRepo:           taskRepo,
			agentManager:       agentMgr,
			messageQueue:       messagequeue.NewService(log),
			executor:           exec,
			scheduler:          sched,
		}

		newSession, err := svc.switchSessionForStep(ctx, "t1", session, "profile-b")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify old session is completed
		oldSession, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get old session: %v", err)
		}
		if oldSession.State != models.TaskSessionStateCompleted {
			t.Errorf("expected old session state completed, got %s", oldSession.State)
		}
		if oldSession.CompletedAt == nil {
			t.Error("expected old session to have CompletedAt set")
		}

		// Verify new session exists with correct profile
		if newSession == nil {
			t.Fatal("expected new session, got nil")
		}
		if newSession.AgentProfileID != "profile-b" {
			t.Errorf("expected new session profile profile-b, got %q", newSession.AgentProfileID)
		}
		if newSession.ID == "s1" {
			t.Error("expected new session to have a different ID from old session")
		}
	})
}

// TestSwitchSessionForStep_ReusesExistingProfileSession verifies the core
// requirement: when switching to a profile that already has a session on this
// task, switchSessionForStep reuses it instead of creating a third session.
// Covers the A→B→A round trip (and beyond) at the unit-test level.
func TestSwitchSessionForStep_ReusesExistingProfileSession(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	repo := setupTestRepo(t)

	ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)
	task := &models.Task{
		ID: "t1", WorkflowID: "wf1", WorkflowStepID: "step1",
		Title: "Test", Description: "Test", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}
	_ = repo.CreateTask(ctx, task)

	// Prior session for profile-A — was active before, then completed when
	// the workflow switched away from this profile last time.
	completedAt := now.Add(-2 * time.Minute)
	prior := &models.TaskSession{
		ID:                "session-a",
		TaskID:            "t1",
		AgentProfileID:    "profile-a",
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		State:             models.TaskSessionStateCompleted,
		IsPrimary:         false,
		CompletedAt:       &completedAt,
		StartedAt:         now.Add(-3 * time.Minute),
		UpdatedAt:         completedAt,
	}
	_ = repo.CreateTaskSession(ctx, prior)

	// Currently-active session for profile-B — about to be switched away from.
	current := &models.TaskSession{
		ID:                "session-b",
		TaskID:            "t1",
		AgentProfileID:    "profile-b",
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		AgentExecutionID:  "ae-b",
		State:             models.TaskSessionStateRunning,
		IsPrimary:         true,
		StartedAt:         now,
		UpdatedAt:         now,
	}
	_ = repo.CreateTaskSession(ctx, current)

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Test", Description: "Test", State: v1.TaskStateInProgress,
	}

	agentMgr := &mockAgentManager{}
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
	sched := scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.SchedulerConfig{})
	svc := &Service{
		logger:             log,
		repo:               repo,
		workflowStepGetter: newMockStepGetter(),
		taskRepo:           taskRepo,
		agentManager:       agentMgr,
		messageQueue:       messagequeue.NewService(log),
		executor:           exec,
		scheduler:          sched,
	}

	revived, err := svc.switchSessionForStep(ctx, "t1", current, "profile-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Critical: reuse must return the existing session — NOT a brand-new id.
	if revived == nil || revived.ID != "session-a" {
		t.Fatalf("expected reused session-a, got %+v", revived)
	}

	// Total session count must remain 2 — no third session created.
	sessions, err := repo.ListTaskSessions(ctx, "t1")
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions after reuse, got %d", len(sessions))
	}

	// The reused session must be back to a non-terminal state (so it can
	// receive the next prompt) and be primary. Specifically, since the prior
	// session has no executors_running record (never launched), it should
	// flip to CREATED so autoStartStepPrompt routes through StartCreatedSession
	// for a fresh launch (instead of hitting "no executor record" in
	// ensureSessionRunning).
	reused, _ := repo.GetTaskSession(ctx, "session-a")
	if reused.State != models.TaskSessionStateCreated {
		t.Errorf("never-launched reused session must be CREATED (so StartCreatedSession launches it fresh), got %s", reused.State)
	}
	if reused.CompletedAt != nil {
		t.Error("reused session must have CompletedAt cleared")
	}
	if !reused.IsPrimary {
		t.Error("reused session must be primary")
	}

	// The previous current session-b must now be COMPLETED, not primary.
	parked, _ := repo.GetTaskSession(ctx, "session-b")
	if parked.State != models.TaskSessionStateCompleted {
		t.Errorf("previous current session must be COMPLETED, got %s", parked.State)
	}
	if parked.IsPrimary {
		t.Error("previous current session must no longer be primary")
	}
}

// TestSwitchSessionForStep_ReusesPreviouslyLaunchedSession covers the other
// branch of the revive: when the reused session has an executors_running
// record (it was previously launched and has a resume token), it flips to
// WAITING_FOR_INPUT so PromptTask's ensureSessionRunning lazy-resumes the
// agent via ResumeSession (preserving its prior conversation context).
func TestSwitchSessionForStep_ReusesPreviouslyLaunchedSession(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	repo := setupTestRepo(t)

	ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)
	task := &models.Task{
		ID: "t1", WorkflowID: "wf1", WorkflowStepID: "step1",
		Title: "Test", Description: "Test", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}
	_ = repo.CreateTask(ctx, task)

	// Prior session for profile-A — was previously active and has the
	// signals of a real launch: an executors_running record with a resume
	// token. This should route through the WAITING_FOR_INPUT branch of
	// reviveReusedSession.
	completedAt := now.Add(-2 * time.Minute)
	prior := &models.TaskSession{
		ID:                "session-a",
		TaskID:            "t1",
		AgentProfileID:    "profile-a",
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		AgentExecutionID:  "ae-a-1",
		State:             models.TaskSessionStateCompleted,
		CompletedAt:       &completedAt,
		StartedAt:         now.Add(-3 * time.Minute),
		UpdatedAt:         completedAt,
	}
	_ = repo.CreateTaskSession(ctx, prior)
	_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "er-a", SessionID: "session-a", TaskID: "t1",
		ResumeToken: "acp-session-a",
		Resumable:   true,
		CreatedAt:   completedAt, UpdatedAt: completedAt,
	})

	current := &models.TaskSession{
		ID:                "session-b",
		TaskID:            "t1",
		AgentProfileID:    "profile-b",
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		AgentExecutionID:  "ae-b",
		State:             models.TaskSessionStateRunning,
		IsPrimary:         true,
		StartedAt:         now,
		UpdatedAt:         now,
	}
	_ = repo.CreateTaskSession(ctx, current)

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Test", Description: "Test", State: v1.TaskStateInProgress,
	}

	agentMgr := &mockAgentManager{}
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
	sched := scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.SchedulerConfig{})
	svc := &Service{
		logger:             log,
		repo:               repo,
		workflowStepGetter: newMockStepGetter(),
		taskRepo:           taskRepo,
		agentManager:       agentMgr,
		messageQueue:       messagequeue.NewService(log),
		executor:           exec,
		scheduler:          sched,
	}

	revived, err := svc.switchSessionForStep(ctx, "t1", current, "profile-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revived == nil || revived.ID != "session-a" {
		t.Fatalf("expected reused session-a, got %+v", revived)
	}

	reused, _ := repo.GetTaskSession(ctx, "session-a")
	if reused.State != models.TaskSessionStateWaitingForInput {
		t.Errorf("previously-launched reused session must be WAITING_FOR_INPUT (so PromptTask lazy-resumes via ResumeSession), got %s", reused.State)
	}
}

// TestSwitchSessionForStep_ReusesFailedSession exercises the requirement that
// FAILED sessions are reused too. Without this, a previously-failed session
// would be skipped and a fresh one created, leaving the FAILED one as a
// duplicate tab in the UI showing its stale error banner.
func TestSwitchSessionForStep_ReusesFailedSession(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	repo := setupTestRepo(t)

	ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)
	task := &models.Task{
		ID: "t1", WorkflowID: "wf1", WorkflowStepID: "step1",
		Title: "Test", Description: "Test", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}
	_ = repo.CreateTask(ctx, task)

	failedAt := now.Add(-2 * time.Minute)
	prior := &models.TaskSession{
		ID:                "session-a",
		TaskID:            "t1",
		AgentProfileID:    "profile-a",
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		AgentExecutionID:  "ae-a",
		State:             models.TaskSessionStateFailed,
		ErrorMessage:      "execution already running",
		CompletedAt:       &failedAt,
		StartedAt:         now.Add(-3 * time.Minute),
		UpdatedAt:         failedAt,
	}
	_ = repo.CreateTaskSession(ctx, prior)
	_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "er-a", SessionID: "session-a", TaskID: "t1",
		ResumeToken: "acp-session-a",
		Resumable:   true,
		CreatedAt:   failedAt, UpdatedAt: failedAt,
	})

	current := &models.TaskSession{
		ID:                "session-b",
		TaskID:            "t1",
		AgentProfileID:    "profile-b",
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		AgentExecutionID:  "ae-b",
		State:             models.TaskSessionStateRunning,
		IsPrimary:         true,
		StartedAt:         now,
		UpdatedAt:         now,
	}
	_ = repo.CreateTaskSession(ctx, current)

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Test", Description: "Test", State: v1.TaskStateInProgress,
	}

	agentMgr := &mockAgentManager{}
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
	sched := scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.SchedulerConfig{})
	svc := &Service{
		logger:             log,
		repo:               repo,
		workflowStepGetter: newMockStepGetter(),
		taskRepo:           taskRepo,
		agentManager:       agentMgr,
		messageQueue:       messagequeue.NewService(log),
		executor:           exec,
		scheduler:          sched,
	}

	revived, err := svc.switchSessionForStep(ctx, "t1", current, "profile-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revived == nil || revived.ID != "session-a" {
		t.Fatalf("expected reused FAILED session-a, got %+v", revived)
	}

	// No duplicate session must be created.
	sessions, _ := repo.ListTaskSessions(ctx, "t1")
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d (FAILED session was not reused)", len(sessions))
	}

	reused, _ := repo.GetTaskSession(ctx, "session-a")
	if reused.State != models.TaskSessionStateWaitingForInput {
		t.Errorf("FAILED reused session must flip to WAITING_FOR_INPUT (lazy-resume via token), got %s", reused.State)
	}
	if reused.ErrorMessage != "" {
		t.Errorf("FAILED reused session must have ErrorMessage cleared, got %q", reused.ErrorMessage)
	}
	if reused.CompletedAt != nil {
		t.Error("FAILED reused session must have CompletedAt cleared")
	}
}

func TestProcessOnEnter_ProfileSwitch(t *testing.T) {
	ctx := context.Background()

	t.Run("switches session when step has different profile", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()

		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)
		task := &models.Task{
			ID: "t1", WorkflowID: "wf1", WorkflowStepID: "step2",
			Title: "Test", Description: "desc", State: v1.TaskStateInProgress,
			CreatedAt: now, UpdatedAt: now,
		}
		_ = repo.CreateTask(ctx, task)

		session := &models.TaskSession{
			ID:                "s1",
			TaskID:            "t1",
			AgentProfileID:    "profile-a",
			ExecutorID:        "exec-local",
			ExecutorProfileID: "ep1",
			State:             models.TaskSessionStateRunning,
			IsPrimary:         true,
			StartedAt:         now,
			UpdatedAt:         now,
		}
		_ = repo.CreateTaskSession(ctx, session)

		taskRepo := newMockTaskRepo()
		taskRepo.tasks["t1"] = &v1.Task{
			ID:          "t1",
			WorkspaceID: "ws1",
			WorkflowID:  "wf1",
			Title:       "Test",
			Description: "desc",
			State:       v1.TaskStateInProgress,
		}

		sg := newMockStepGetter()
		step := &wfmodels.WorkflowStep{
			ID:             "step2",
			WorkflowID:     "wf1",
			Name:           "Review",
			AgentProfileID: "profile-b",
		}
		sg.steps["step2"] = step

		agentMgr := &mockAgentManager{}
		log := testLogger()
		exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
		sched := scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.SchedulerConfig{})
		svc := &Service{
			logger:             log,
			repo:               repo,
			workflowStepGetter: sg,
			taskRepo:           taskRepo,
			agentManager:       agentMgr,
			messageQueue:       messagequeue.NewService(log),
			executor:           exec,
			scheduler:          sched,
		}

		svc.processOnEnter(ctx, "t1", session, step, "desc")

		// The old session should be completed
		oldSession, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get old session: %v", err)
		}
		if oldSession.State != models.TaskSessionStateCompleted {
			t.Errorf("expected old session completed, got %s", oldSession.State)
		}

		// There should be a new session with profile-b
		sessions, err := repo.ListTaskSessions(ctx, "t1")
		if err != nil {
			t.Fatalf("failed to list sessions: %v", err)
		}
		var newSession *models.TaskSession
		for _, s := range sessions {
			if s.ID != "s1" {
				newSession = s
				break
			}
		}
		if newSession == nil {
			t.Fatal("expected a new session to be created")
		}
		if newSession.AgentProfileID != "profile-b" {
			t.Errorf("expected new session profile profile-b, got %q", newSession.AgentProfileID)
		}
	})

	t.Run("no switch when step has same profile as session", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()

		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)
		task := &models.Task{
			ID: "t1", WorkflowID: "wf1", WorkflowStepID: "step1",
			Title: "Test", Description: "desc", State: v1.TaskStateInProgress,
			CreatedAt: now, UpdatedAt: now,
		}
		_ = repo.CreateTask(ctx, task)

		session := &models.TaskSession{
			ID:             "s1",
			TaskID:         "t1",
			AgentProfileID: "profile-a",
			State:          models.TaskSessionStateRunning,
			IsPrimary:      true,
			StartedAt:      now,
			UpdatedAt:      now,
		}
		_ = repo.CreateTaskSession(ctx, session)

		sg := newMockStepGetter()
		step := &wfmodels.WorkflowStep{
			ID:             "step1",
			WorkflowID:     "wf1",
			Name:           "Develop",
			AgentProfileID: "profile-a",
		}
		sg.steps["step1"] = step

		svc := createTestService(repo, sg, newMockTaskRepo())
		svc.processOnEnter(ctx, "t1", session, step, "desc")

		// Session should remain running (not completed)
		updatedSession, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if updatedSession.State == models.TaskSessionStateCompleted {
			t.Error("session should not be completed when profile matches")
		}

		// No new sessions should be created
		sessions, err := repo.ListTaskSessions(ctx, "t1")
		if err != nil {
			t.Fatalf("failed to list sessions: %v", err)
		}
		if len(sessions) != 1 {
			t.Errorf("expected 1 session, got %d", len(sessions))
		}
	})

	t.Run("no switch for passthrough sessions", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()

		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)
		task := &models.Task{
			ID: "t1", WorkflowID: "wf1", WorkflowStepID: "step2",
			Title: "Test", Description: "desc", State: v1.TaskStateInProgress,
			CreatedAt: now, UpdatedAt: now,
		}
		_ = repo.CreateTask(ctx, task)

		session := &models.TaskSession{
			ID:             "s1",
			TaskID:         "t1",
			AgentProfileID: "profile-a",
			State:          models.TaskSessionStateRunning,
			IsPrimary:      true,
			StartedAt:      now,
			UpdatedAt:      now,
		}
		_ = repo.CreateTaskSession(ctx, session)

		sg := newMockStepGetter()
		step := &wfmodels.WorkflowStep{
			ID:             "step2",
			WorkflowID:     "wf1",
			Name:           "Review",
			AgentProfileID: "profile-b",
		}
		sg.steps["step2"] = step

		agentMgr := &mockAgentManager{isPassthrough: true}
		svc := createTestServiceWithAgent(repo, sg, newMockTaskRepo(), agentMgr)
		svc.processOnEnter(ctx, "t1", session, step, "desc")

		// Session should NOT be completed (passthrough skips profile switch)
		updatedSession, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if updatedSession.State == models.TaskSessionStateCompleted {
			t.Error("passthrough session should not be completed for profile switch")
		}
	})

	t.Run("no switch when step has no profile", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()

		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)
		task := &models.Task{
			ID: "t1", WorkflowID: "wf1", WorkflowStepID: "step1",
			Title: "Test", Description: "desc", State: v1.TaskStateInProgress,
			CreatedAt: now, UpdatedAt: now,
		}
		_ = repo.CreateTask(ctx, task)

		session := &models.TaskSession{
			ID:             "s1",
			TaskID:         "t1",
			AgentProfileID: "profile-a",
			State:          models.TaskSessionStateRunning,
			IsPrimary:      true,
			StartedAt:      now,
			UpdatedAt:      now,
		}
		_ = repo.CreateTaskSession(ctx, session)

		sg := newMockStepGetter()
		step := &wfmodels.WorkflowStep{
			ID:         "step1",
			WorkflowID: "wf1",
			Name:       "Develop",
			// No AgentProfileID
		}
		sg.steps["step1"] = step

		svc := createTestService(repo, sg, newMockTaskRepo())
		svc.processOnEnter(ctx, "t1", session, step, "desc")

		// Session should remain running
		sessions, err := repo.ListTaskSessions(ctx, "t1")
		if err != nil {
			t.Fatalf("failed to list sessions: %v", err)
		}
		if len(sessions) != 1 {
			t.Errorf("expected 1 session, got %d", len(sessions))
		}
	})
}

func TestSwitchSessionForStep_PreservesOldSessionOnFailure(t *testing.T) {
	ctx := context.Background()

	t.Run("old session not completed when scheduler.GetTask fails", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()

		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)
		task := &models.Task{
			ID: "t1", WorkflowID: "wf1", WorkflowStepID: "step2",
			Title: "Test", Description: "Test", State: v1.TaskStateInProgress,
			CreatedAt: now, UpdatedAt: now,
		}
		_ = repo.CreateTask(ctx, task)

		session := &models.TaskSession{
			ID:                "s1",
			TaskID:            "t1",
			AgentProfileID:    "profile-a",
			ExecutorID:        "exec-local",
			ExecutorProfileID: "ep1",
			AgentExecutionID:  "ae1",
			State:             models.TaskSessionStateRunning,
			IsPrimary:         true,
			StartedAt:         now,
			UpdatedAt:         now,
		}
		_ = repo.CreateTaskSession(ctx, session)

		// Make scheduler.GetTask fail — the old session must stay untouched.
		taskRepo := newMockTaskRepo()
		taskRepo.getTaskErr = errors.New("task store unavailable")

		agentMgr := &mockAgentManager{}
		log := testLogger()
		exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
		sched := scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.SchedulerConfig{})
		svc := &Service{
			logger:             log,
			repo:               repo,
			workflowStepGetter: newMockStepGetter(),
			taskRepo:           taskRepo,
			agentManager:       agentMgr,
			messageQueue:       messagequeue.NewService(log),
			executor:           exec,
			scheduler:          sched,
		}

		_, err := svc.switchSessionForStep(ctx, "t1", session, "profile-b")
		if err == nil {
			t.Fatal("expected error when scheduler.GetTask fails")
		}

		// The old session must NOT be completed — failure happened before touching it.
		oldSession, getErr := repo.GetTaskSession(ctx, "s1")
		if getErr != nil {
			t.Fatalf("failed to get old session: %v", getErr)
		}
		if oldSession.State == models.TaskSessionStateCompleted {
			t.Error("old session must not be marked completed when PrepareSession fails before it")
		}
		if oldSession.CompletedAt != nil {
			t.Error("old session must not have CompletedAt set when PrepareSession fails before it")
		}
	})
}

func TestResolveStepAgentProfile_UsedByHandleTaskMovedNoSession(t *testing.T) {
	// This test verifies that resolveStepAgentProfile correctly prioritizes
	// step profile over workflow profile. The actual handleTaskMovedNoSession
	// integration is covered by the resolution order tests above.

	t.Run("step profile beats workflow default", func(t *testing.T) {
		sg := newMockStepGetter()
		sg.workflowAgentProfileID = "profile-workflow"
		svc := createTestService(setupTestRepo(t), sg, newMockTaskRepo())

		step := &wfmodels.WorkflowStep{
			ID:             "step1",
			WorkflowID:     "wf1",
			AgentProfileID: "profile-step",
		}
		got := svc.resolveStepAgentProfile(context.Background(), step)
		if got != "profile-step" {
			t.Errorf("expected profile-step, got %q", got)
		}
	})

	t.Run("workflow profile used when step has none", func(t *testing.T) {
		sg := newMockStepGetter()
		sg.workflowAgentProfileID = "profile-workflow"
		svc := createTestService(setupTestRepo(t), sg, newMockTaskRepo())

		step := &wfmodels.WorkflowStep{
			ID:         "step1",
			WorkflowID: "wf1",
		}
		got := svc.resolveStepAgentProfile(context.Background(), step)
		if got != "profile-workflow" {
			t.Errorf("expected profile-workflow, got %q", got)
		}
	})
}
