package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestResolveIntent(t *testing.T) {
	tests := []struct {
		name string
		req  LaunchSessionRequest
		want SessionIntent
	}{
		// Explicit intents take priority
		{
			name: "explicit start intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentStart},
			want: IntentStart,
		},
		{
			name: "explicit resume intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentResume, SessionID: "s1"},
			want: IntentResume,
		},
		{
			name: "explicit prepare intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentPrepare},
			want: IntentPrepare,
		},
		{
			name: "explicit start_created intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentStartCreated, SessionID: "s1"},
			want: IntentStartCreated,
		},
		{
			name: "explicit workflow_step intent",
			req:  LaunchSessionRequest{TaskID: "t1", Intent: IntentWorkflowStep, SessionID: "s1", WorkflowStepID: "ws1"},
			want: IntentWorkflowStep,
		},

		// Inferred intents (no explicit intent set)
		{
			name: "workflow_step inferred from session_id + workflow_step_id",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", WorkflowStepID: "ws1"},
			want: IntentWorkflowStep,
		},
		{
			name: "resume inferred from session_id only",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1"},
			want: IntentResume,
		},
		{
			name: "resume inferred from session_id with no prompt and no agent_profile",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1"},
			want: IntentResume,
		},
		{
			name: "start_created inferred from session_id + prompt",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", Prompt: "hello"},
			want: IntentStartCreated,
		},
		{
			name: "start_created inferred from session_id + agent_profile_id",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", AgentProfileID: "ap1"},
			want: IntentStartCreated,
		},
		{
			name: "start_created inferred from session_id + prompt + agent_profile_id",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", Prompt: "hello", AgentProfileID: "ap1"},
			want: IntentStartCreated,
		},
		{
			name: "prepare inferred from launch_workspace without prompt",
			req:  LaunchSessionRequest{TaskID: "t1", LaunchWorkspace: true},
			want: IntentPrepare,
		},
		{
			name: "start inferred from minimal request",
			req:  LaunchSessionRequest{TaskID: "t1"},
			want: IntentStart,
		},
		{
			name: "start inferred when prompt provided without session_id",
			req:  LaunchSessionRequest{TaskID: "t1", Prompt: "do something"},
			want: IntentStart,
		},
		{
			name: "start inferred when launch_workspace + prompt (not prepare)",
			req:  LaunchSessionRequest{TaskID: "t1", LaunchWorkspace: true, Prompt: "do something"},
			want: IntentStart,
		},

		// Edge cases
		{
			name: "resume wins over start_created when session_id set, no prompt, no agent_profile",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", ExecutorID: "e1"},
			want: IntentResume,
		},
		{
			name: "workflow_step wins over resume when both session_id and workflow_step_id set",
			req:  LaunchSessionRequest{TaskID: "t1", SessionID: "s1", WorkflowStepID: "ws1"},
			want: IntentWorkflowStep,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveIntent(&tt.req)
			if got != tt.want {
				t.Errorf("ResolveIntent() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- launchRestoreWorkspace ---

func TestLaunchRestoreWorkspace_MissingSessionID(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	_, err := svc.LaunchSession(context.Background(), &LaunchSessionRequest{
		TaskID: "task1",
		Intent: IntentRestoreWorkspace,
	})
	if err == nil {
		t.Fatal("expected error when session_id is empty")
	}
}

func TestLaunchRestoreWorkspace_SessionNotFound(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	_, err := svc.LaunchSession(context.Background(), &LaunchSessionRequest{
		TaskID:    "task1",
		Intent:    IntentRestoreWorkspace,
		SessionID: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error when session does not exist")
	}
}

func TestLaunchRestoreWorkspace_WrongTask(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	seedTaskAndSession(t, repo, "task-other", "session1", models.TaskSessionStateCompleted)

	_, err := svc.LaunchSession(context.Background(), &LaunchSessionRequest{
		TaskID:    "task-wrong",
		Intent:    IntentRestoreWorkspace,
		SessionID: "session1",
	})
	if err == nil {
		t.Fatal("expected error when session does not belong to task")
	}
}

func TestLaunchRestoreWorkspace_Success(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCompleted)

	resp, err := svc.LaunchSession(context.Background(), &LaunchSessionRequest{
		TaskID:    "task1",
		Intent:    IntentRestoreWorkspace,
		SessionID: "session1",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success=true")
	}
	if resp.SessionID != "session1" {
		t.Errorf("expected session_id 'session1', got %q", resp.SessionID)
	}
	if resp.State != string(models.TaskSessionStateCompleted) {
		t.Errorf("expected state %q, got %q", models.TaskSessionStateCompleted, resp.State)
	}
}

// --- launchPrepare passthrough upgrade ---

func TestIsPassthroughProfile(t *testing.T) {
	repo := setupTestRepo(t)
	passthroughMgr := &mockAgentManager{isPassthrough: true}
	regularMgr := &mockAgentManager{isPassthrough: false}

	t.Run("passthrough profile detected", func(t *testing.T) {
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), passthroughMgr)
		if !svc.isPassthroughProfile(context.Background(), "profile1") {
			t.Error("expected isPassthroughProfile=true for passthrough profile")
		}
	})

	t.Run("non-passthrough profile not detected", func(t *testing.T) {
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), regularMgr)
		if svc.isPassthroughProfile(context.Background(), "profile1") {
			t.Error("expected isPassthroughProfile=false for non-passthrough profile")
		}
	})

	t.Run("empty profile id returns false", func(t *testing.T) {
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), passthroughMgr)
		if svc.isPassthroughProfile(context.Background(), "") {
			t.Error("expected isPassthroughProfile=false for empty profile id")
		}
	})

	t.Run("nil agent manager returns false", func(t *testing.T) {
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.agentManager = nil
		if svc.isPassthroughProfile(context.Background(), "profile1") {
			t.Error("expected isPassthroughProfile=false when agent manager is nil")
		}
	})
}

// TestLaunchPrepare_PassthroughDoesNotRecurse guards against an infinite
// launchStart ↔ launchPrepare bounce when a passthrough profile is combined
// with AutoStart=true and a step that blocks auto-start. Wiring the full
// scheduler+executor stack is heavy, so we run the dispatch in a goroutine
// with a panic recover: stack-overflow recursion would never return, while
// the legitimate downstream nil-deref panic from the stub scheduler still
// completes within the deadline.
func TestLaunchPrepare_PassthroughDoesNotRecurse(t *testing.T) {
	repo := setupTestRepo(t)
	mgr := &mockAgentManager{isPassthrough: true}
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-blocked"] = &wfmodels.WorkflowStep{
		ID:   "step-blocked",
		Name: "blocked",
		// no on_enter actions => auto_start_agent missing => blocked
	}
	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, mgr)

	done := make(chan struct{})
	go func() {
		defer func() {
			// Stub scheduler nil-deref is expected. Recursion would never reach
			// this point.
			_ = recover()
			close(done)
		}()
		_, _ = svc.LaunchSession(context.Background(), &LaunchSessionRequest{
			TaskID:         "task1",
			Intent:         IntentStart,
			AgentProfileID: "profile-pass",
			AutoStart:      true,
			WorkflowStepID: "step-blocked",
		})
	}()

	select {
	case <-done:
		// returned without recursing
	case <-time.After(2 * time.Second):
		t.Fatal("LaunchSession recursed indefinitely (timed out)")
	}
}

func TestLaunchRestoreWorkspace_IncludesWorktreeInfo(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

	// Add worktree to the session
	if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
		ID:             "wt1",
		SessionID:      "session1",
		WorktreeID:     "wid1",
		RepositoryID:   "repo1",
		WorktreePath:   "/tmp/worktrees/session1",
		WorktreeBranch: "feature/test",
	}); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	resp, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:    "task1",
		Intent:    IntentRestoreWorkspace,
		SessionID: "session1",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.WorktreePath == nil || *resp.WorktreePath != "/tmp/worktrees/session1" {
		t.Errorf("expected worktree_path '/tmp/worktrees/session1', got %v", resp.WorktreePath)
	}
	if resp.WorktreeBranch == nil || *resp.WorktreeBranch != "feature/test" {
		t.Errorf("expected worktree_branch 'feature/test', got %v", resp.WorktreeBranch)
	}
}
