package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestEnsureSession_RequiresTaskID(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if _, err := svc.EnsureSession(context.Background(), ""); err == nil {
		t.Fatal("expected error when task_id is empty")
	}
}

func TestEnsureSession_TaskNotFound(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if _, err := svc.EnsureSession(context.Background(), "missing"); err == nil {
		t.Fatal("expected error when task is missing")
	}
}

func TestEnsureSession_ReturnsExistingPrimary(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	seedTaskAndSession(t, repo, "task1", "session-old", models.TaskSessionStateCompleted)
	// Add a newer non-primary session, then mark the older one primary.
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-new", TaskID: "task1", State: models.TaskSessionStateRunning,
		StartedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("failed to create newer session: %v", err)
	}
	if err := repo.SetSessionPrimary(ctx, "session-old"); err != nil {
		t.Fatalf("failed to mark primary: %v", err)
	}

	resp, err := svc.EnsureSession(ctx, "task1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.SessionID != "session-old" {
		t.Errorf("expected primary session-old, got %q", resp.SessionID)
	}
	if resp.Source != "existing_primary" {
		t.Errorf("expected source=existing_primary, got %q", resp.Source)
	}
	if resp.NewlyCreated {
		t.Error("expected NewlyCreated=false")
	}
}

func TestEnsureSession_ReturnsExistingNewest_NoPrimary(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	seedTaskAndSession(t, repo, "task1", "session-old", models.TaskSessionStateCompleted)
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-new", TaskID: "task1", State: models.TaskSessionStateRunning,
		StartedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("failed to create newer session: %v", err)
	}

	resp, err := svc.EnsureSession(ctx, "task1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.SessionID != "session-new" {
		t.Errorf("expected newest session-new, got %q", resp.SessionID)
	}
	if resp.Source != "existing_newest" {
		t.Errorf("expected source=existing_newest, got %q", resp.Source)
	}
}

func TestEnsureSession_Concurrent_ReturnsSameExistingSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

	const N = 8
	var wg sync.WaitGroup
	results := make([]string, N)
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			resp, err := svc.EnsureSession(ctx, "task1")
			if err != nil {
				t.Errorf("concurrent ensure failed: %v", err)
				return
			}
			results[idx] = resp.SessionID
		}(i)
	}
	wg.Wait()

	for i, sid := range results {
		if sid != "session1" {
			t.Errorf("call %d: expected session1, got %q", i, sid)
		}
	}
}

// acquireEnsureLock must serialize callers per task id. The previous attempt
// to bound map growth (Delete on release) opened a window where a third
// caller could LoadOrStore a fresh mutex while a second caller still held
// the about-to-be-discarded one, putting two goroutines in the critical
// section at the same time. This test pins down that property by counting
// the maximum observed concurrency under the same task id.
func TestAcquireEnsureLock_SerializesPerTaskID(t *testing.T) {
	const N = 16
	var (
		active int
		mu     sync.Mutex
		maxCon int
		wg     sync.WaitGroup
	)
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			release := acquireEnsureLock("task-x")
			defer release()
			mu.Lock()
			active++
			if active > maxCon {
				maxCon = active
			}
			mu.Unlock()
			time.Sleep(2 * time.Millisecond)
			mu.Lock()
			active--
			mu.Unlock()
		}()
	}
	wg.Wait()
	if maxCon != 1 {
		t.Errorf("expected max concurrency 1 under same task id, got %d", maxCon)
	}
}

// Distinct task ids must NOT serialize against each other.
func TestAcquireEnsureLock_AllowsConcurrencyAcrossTaskIDs(t *testing.T) {
	const N = 8
	start := make(chan struct{})
	var holding int
	var mu sync.Mutex
	var peak int
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		taskID := fmt.Sprintf("task-%d", i)
		go func() {
			defer wg.Done()
			release := acquireEnsureLock(taskID)
			defer release()
			<-start
			mu.Lock()
			holding++
			if holding > peak {
				peak = holding
			}
			mu.Unlock()
			time.Sleep(2 * time.Millisecond)
			mu.Lock()
			holding--
			mu.Unlock()
		}()
	}
	// Let all goroutines acquire their distinct locks, then release.
	time.Sleep(20 * time.Millisecond)
	close(start)
	wg.Wait()
	if peak < 2 {
		t.Errorf("expected concurrency across distinct task ids, peak=%d", peak)
	}
}

func TestStepAllowsAutoStart(t *testing.T) {
	if !stepAllowsAutoStart(nil) {
		t.Error("expected nil step to allow auto-start (no workflow step constraint)")
	}
	stepWith := &wfmodels.WorkflowStep{
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}
	if !stepAllowsAutoStart(stepWith) {
		t.Error("expected step with auto_start_agent to allow auto-start")
	}
	stepWithout := &wfmodels.WorkflowStep{}
	if stepAllowsAutoStart(stepWithout) {
		t.Error("expected step without auto_start_agent to disallow auto-start")
	}
}

func TestResolveTaskAgentProfile_TaskMetadataWins(t *testing.T) {
	repo := setupTestRepo(t)
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", AgentProfileID: "step-profile"}
	stepGetter.workflowAgentProfileID = "wf-profile"
	svc := createTestService(repo, stepGetter, newMockTaskRepo())

	task := &models.Task{
		ID:             "t1",
		WorkspaceID:    "ws1",
		WorkflowStepID: "step1",
		Metadata:       map[string]interface{}{"agent_profile_id": "task-profile"},
	}
	if got, _ := svc.resolveTaskAgentProfile(context.Background(), task); got != "task-profile" {
		t.Errorf("expected task-profile, got %q", got)
	}
}

func TestResolveTaskAgentProfile_StepThenWorkflowThenWorkspace(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("step override", func(t *testing.T) {
		repo := setupTestRepo(t)
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", AgentProfileID: "step-profile"}
		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		task := &models.Task{ID: "t1", WorkflowStepID: "step1"}
		if got, _ := svc.resolveTaskAgentProfile(ctx, task); got != "step-profile" {
			t.Errorf("expected step-profile, got %q", got)
		}
	})

	t.Run("workflow default when step has none", func(t *testing.T) {
		repo := setupTestRepo(t)
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1"}
		stepGetter.workflowAgentProfileID = "wf-profile"
		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		task := &models.Task{ID: "t1", WorkflowStepID: "step1"}
		if got, _ := svc.resolveTaskAgentProfile(ctx, task); got != "wf-profile" {
			t.Errorf("expected wf-profile, got %q", got)
		}
	})

	t.Run("workspace default when step+workflow have none", func(t *testing.T) {
		repo := setupTestRepo(t)
		ws := &models.Workspace{
			ID: "ws-x", Name: "X", DefaultAgentProfileID: strPtr("ws-profile"),
			CreatedAt: now, UpdatedAt: now,
		}
		if err := repo.CreateWorkspace(ctx, ws); err != nil {
			t.Fatalf("create workspace: %v", err)
		}
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		task := &models.Task{ID: "t1", WorkspaceID: "ws-x"}
		if got, _ := svc.resolveTaskAgentProfile(ctx, task); got != "ws-profile" {
			t.Errorf("expected ws-profile, got %q", got)
		}
	})

	t.Run("returns empty when nothing resolves", func(t *testing.T) {
		repo := setupTestRepo(t)
		ws := &models.Workspace{ID: "ws-y", Name: "Y", CreatedAt: now, UpdatedAt: now}
		if err := repo.CreateWorkspace(ctx, ws); err != nil {
			t.Fatalf("create workspace: %v", err)
		}
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		task := &models.Task{ID: "t1", WorkspaceID: "ws-y"}
		if got, _ := svc.resolveTaskAgentProfile(ctx, task); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
}
