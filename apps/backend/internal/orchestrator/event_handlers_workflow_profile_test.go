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
