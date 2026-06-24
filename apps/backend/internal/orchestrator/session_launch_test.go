package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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

func TestNormalizeRecoverSessionError(t *testing.T) {
	t.Run("maps profile not found errors to actionable profile guidance", func(t *testing.T) {
		in := errors.New("failed to resolve agent profile: profile not found: sql: no rows in result set")
		err := normalizeRecoverSessionError(in)
		if err == nil {
			t.Fatal("expected mapped error")
		}
		want := "the agent profile used by this session was deleted; start a new session and choose an available agent profile: " + in.Error()
		if got := err.Error(); got != want {
			t.Fatalf("unexpected error: %q", got)
		}
	})

	t.Run("maps agent profile not found errors to actionable profile guidance", func(t *testing.T) {
		in := errors.New("agent profile not found")
		err := normalizeRecoverSessionError(in)
		if err == nil {
			t.Fatal("expected mapped error")
		}
		want := "the agent profile used by this session was deleted; start a new session and choose an available agent profile: " + in.Error()
		if got := err.Error(); got != want {
			t.Fatalf("unexpected error: %q", got)
		}
	})

	t.Run("does not map generic sql no rows errors", func(t *testing.T) {
		in := errors.New("sql: no rows in result set")
		err := normalizeRecoverSessionError(in)
		if err == nil {
			t.Fatal("expected passthrough error")
		}
		if err.Error() != in.Error() {
			t.Fatalf("expected passthrough error %q, got %q", in.Error(), err.Error())
		}
	})

	t.Run("does not map executor profile not found errors", func(t *testing.T) {
		in := errors.New("executor profile not found")
		err := normalizeRecoverSessionError(in)
		if err == nil {
			t.Fatal("expected passthrough error")
		}
		if err.Error() != in.Error() {
			t.Fatalf("expected passthrough error %q, got %q", in.Error(), err.Error())
		}
	})

	t.Run("passes through unrelated errors", func(t *testing.T) {
		in := errors.New("network timeout")
		err := normalizeRecoverSessionError(in)
		if err == nil {
			t.Fatal("expected passthrough error")
		}
		if err.Error() != in.Error() {
			t.Fatalf("expected passthrough error %q, got %q", in.Error(), err.Error())
		}
	})
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
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
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

	t.Run("resolver error returns false", func(t *testing.T) {
		errorMgr := &mockAgentManager{resolveProfileErr: errors.New("lookup failed")}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), errorMgr)
		if svc.isPassthroughProfile(context.Background(), "profile1") {
			t.Error("expected isPassthroughProfile=false when resolver errors")
		}
	})
}

// TestShouldUpgradePassthroughPrepare pins the launchPrepare upgrade decision.
//
// Regression guard for the prompt-delivery break introduced by the passthrough
// upgrade (PR #744): the two-phase create flow does a cheap prompt-less prepare
// followed by an async IntentStartCreated that carries the prompt. If a
// passthrough prepare is eagerly upgraded to a full launch there, the PTY spawns
// with an empty prompt and the prompt-bearing start is rejected against the
// now-running session — so the agent sits at an empty prompt. DeferredStart must
// suppress the upgrade so that follow-up start launches the passthrough WITH the
// prompt, exactly as ACP already does.
func TestShouldUpgradePassthroughPrepare(t *testing.T) {
	repo := setupTestRepo(t)
	passthrough := &mockAgentManager{isPassthrough: true}
	regular := &mockAgentManager{isPassthrough: false}

	tests := []struct {
		name string
		mgr  *mockAgentManager
		req  LaunchSessionRequest
		want bool
	}{
		{
			name: "prepare-only passthrough upgrades (quick-chat terminal needs a PTY)",
			mgr:  passthrough,
			req:  LaunchSessionRequest{AgentProfileID: "profile1"},
			want: true,
		},
		{
			name: "deferred-start passthrough does NOT upgrade (prompt-bearing start follows)",
			mgr:  passthrough,
			req:  LaunchSessionRequest{AgentProfileID: "profile1", DeferredStart: true},
			want: false,
		},
		{
			name: "auto-start passthrough does NOT upgrade (avoids launchStart bounce)",
			mgr:  passthrough,
			req:  LaunchSessionRequest{AgentProfileID: "profile1", AutoStart: true},
			want: false,
		},
		{
			name: "non-passthrough never upgrades",
			mgr:  regular,
			req:  LaunchSessionRequest{AgentProfileID: "profile1"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), tt.mgr)
			if got := svc.shouldUpgradePassthroughPrepare(context.Background(), &tt.req); got != tt.want {
				t.Errorf("shouldUpgradePassthroughPrepare()=%v, want %v", got, tt.want)
			}
		})
	}
}

// TestLaunchPrepare_PassthroughDoesNotRecurse guards against an infinite
// launchStart ↔ launchPrepare bounce when a passthrough profile is combined
// with AutoStart=true and a step that blocks auto-start.
//
// To actually exercise the guard, the task must be persisted with a
// WorkflowStepID that maps to a step lacking `auto_start_agent` — otherwise
// `shouldBlockAutoStart` short-circuits to false and the test bypasses the
// downgrade path entirely. Wiring the full scheduler+executor stack is too
// heavy, so we run in a goroutine with a panic recover: stack-overflow
// recursion would never return, while the legitimate downstream nil-deref
// panic from the stub scheduler still completes within the deadline.
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

	// Persist a task with the blocking step so shouldBlockAutoStart returns
	// true, forcing launchStart to downgrade into launchPrepare. Without the
	// `!req.AutoStart` guard, that re-enters launchStart and recurses.
	seedTaskAndSessionWithStep(t, repo, "task1", "sess-pass", "step-blocked")

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
	case <-time.After(150 * time.Millisecond):
		t.Fatal("LaunchSession recursed indefinitely (timed out)")
	}
}

// seedTaskAndSessionWithStep is a variant of seedTaskAndSession that wires
// the task to a workflow step, required by shouldBlockAutoStart.
func seedTaskAndSessionWithStep(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID, stepID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)

	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Test Workflow", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)

	task := &models.Task{
		ID:             taskID,
		WorkflowID:     "wf1",
		WorkflowStepID: stepID,
		Title:          "Test Task",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	session := &models.TaskSession{
		ID:        sessionID,
		TaskID:    taskID,
		State:     models.TaskSessionStateCreated,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
}

func TestLaunchRestoreWorkspace_IncludesWorktreeInfo(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
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

// --- launchStart reuse of prepared sessions ---

// TestFindReusableCreatedSession pins the lookup that lets an explicit Start
// adopt a session.ensure-prepared row instead of creating a duplicate.
func TestFindReusableCreatedSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	t.Run("returns nil when task has no sessions", func(t *testing.T) {
		if got := svc.findReusableCreatedSession(ctx, "task-missing"); got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})

	t.Run("skips non-CREATED sessions", func(t *testing.T) {
		seedTaskAndSession(t, repo, "task-running", "s-running", models.TaskSessionStateRunning)
		if got := svc.findReusableCreatedSession(ctx, "task-running"); got != nil {
			t.Fatalf("expected nil for RUNNING-only task, got %+v", got)
		}
	})

	t.Run("prefers primary CREATED over newer non-primary", func(t *testing.T) {
		seedTaskAndSession(t, repo, "task-primary", "s-primary", models.TaskSessionStateCreated)
		if err := repo.SetSessionPrimary(ctx, "s-primary"); err != nil {
			t.Fatalf("set primary: %v", err)
		}
		newer := &models.TaskSession{
			ID:        "s-newer",
			TaskID:    "task-primary",
			State:     models.TaskSessionStateCreated,
			StartedAt: time.Now().UTC().Add(time.Minute),
			UpdatedAt: time.Now().UTC().Add(time.Minute),
		}
		if err := repo.CreateTaskSession(ctx, newer); err != nil {
			t.Fatalf("create newer: %v", err)
		}
		got := svc.findReusableCreatedSession(ctx, "task-primary")
		if got == nil || got.ID != "s-primary" {
			t.Fatalf("expected primary session, got %+v", got)
		}
	})

	t.Run("falls back to newest CREATED when no primary", func(t *testing.T) {
		// Stress the UpdatedAt comparison by inverting it against the SQL
		// started_at DESC order: s-sql-newer is inserted with a LATER
		// StartedAt but an OLDER UpdatedAt than s-updated-newer. SQL puts
		// s-sql-newer first; only the UpdatedAt comparison in
		// findReusableCreatedSession can pick s-updated-newer. Without that
		// comparison, the test would erroneously accept whatever SQL returns
		// first. CreateTaskSession honors caller-supplied StartedAt/UpdatedAt
		// when non-zero, so this writes the desired skew directly.
		// seedTaskAndSession creates the parent task; the seeded session is
		// dropped immediately so only the two crafted rows remain.
		seedTaskAndSession(t, repo, "task-no-primary", "s-throwaway", models.TaskSessionStateCreated)
		if err := repo.DeleteTaskSession(ctx, "s-throwaway"); err != nil {
			t.Fatalf("delete throwaway: %v", err)
		}
		now := time.Now().UTC()
		sqlNewer := &models.TaskSession{
			ID:        "s-sql-newer",
			TaskID:    "task-no-primary",
			State:     models.TaskSessionStateCreated,
			StartedAt: now.Add(time.Minute),
			UpdatedAt: now.Add(-time.Hour),
		}
		if err := repo.CreateTaskSession(ctx, sqlNewer); err != nil {
			t.Fatalf("create sql-newer: %v", err)
		}
		updatedNewer := &models.TaskSession{
			ID:        "s-updated-newer",
			TaskID:    "task-no-primary",
			State:     models.TaskSessionStateCreated,
			StartedAt: now,
			UpdatedAt: now,
		}
		if err := repo.CreateTaskSession(ctx, updatedNewer); err != nil {
			t.Fatalf("create updated-newer: %v", err)
		}
		got := svc.findReusableCreatedSession(ctx, "task-no-primary")
		if got == nil || got.ID != "s-updated-newer" {
			t.Fatalf("expected newest-UpdatedAt session, got %+v", got)
		}
	})
}

// TestProfilesCompatible covers the agent-profile matching rule used by
// launchStart's reuse path: identical or either-side-empty is fine; differing
// non-empty profiles must NOT share a session row.
func TestProfilesCompatible(t *testing.T) {
	cases := []struct {
		name         string
		caller, sess string
		want         bool
	}{
		{"both empty", "", "", true},
		{"caller empty", "", "p", true},
		{"session empty", "p", "", true},
		{"same profile", "p", "p", true},
		{"different profiles", "p1", "p2", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := profilesCompatible(tc.caller, tc.sess); got != tc.want {
				t.Errorf("profilesCompatible(%q,%q)=%v, want %v", tc.caller, tc.sess, got, tc.want)
			}
		})
	}
}

// TestCanReusePreparedSession pins the gate that protects launchStart from
// adopting a prepared session when the caller has chosen its own executor,
// executor profile, or priority — start_created carries none of those, so
// reusing would silently drop them.
func TestCanReusePreparedSession(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	cases := []struct {
		name string
		req  LaunchSessionRequest
		want bool
	}{
		{"empty fields → reuse", LaunchSessionRequest{TaskID: "t"}, true},
		{"explicit session_id → no reuse", LaunchSessionRequest{TaskID: "t", SessionID: "s"}, false},
		{"executor_id set → no reuse", LaunchSessionRequest{TaskID: "t", ExecutorID: "e"}, false},
		{"executor_profile_id set → no reuse", LaunchSessionRequest{TaskID: "t", ExecutorProfileID: "ep"}, false},
		{"priority set → no reuse", LaunchSessionRequest{TaskID: "t", Priority: "high"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := svc.canReusePreparedSession(&tc.req); got != tc.want {
				t.Errorf("canReusePreparedSession()=%v, want %v", got, tc.want)
			}
		})
	}
}

// TestLaunchStart_ReusesPreparedSession regression-tests the duplicate-session
// bug: opening a task whose step lacks auto_start_agent calls session.ensure
// (creates a CREATED session via prepare); the subsequent edit-dialog "Start
// Agent" click must adopt that session instead of letting StartTask create a
// second row.
func TestLaunchStart_ReusesPreparedSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

	seedTaskAndSession(t, repo, "task1", "session-prepared", models.TaskSessionStateCreated)
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", WorkspaceID: "ws1", Title: "Test", Description: "desc"}
	if err := repo.SetSessionPrimary(ctx, "session-prepared"); err != nil {
		t.Fatalf("set primary: %v", err)
	}
	prepared, err := repo.GetTaskSession(ctx, "session-prepared")
	if err != nil {
		t.Fatalf("load seeded session: %v", err)
	}
	prepared.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, prepared); err != nil {
		t.Fatalf("update seeded profile: %v", err)
	}

	// Mirrors buildStartRequest from the edit dialog: explicit start, no
	// session_id, agent profile + prompt supplied. Downstream agent launch
	// panics on the stub scheduler — that's expected and not what this test
	// pins. Run the call in a recovered goroutine so we can still inspect
	// the post-state session count from the main goroutine.
	done := make(chan struct{})
	go func() {
		defer func() {
			_ = recover()
			close(done)
		}()
		_, _ = svc.LaunchSession(ctx, &LaunchSessionRequest{
			TaskID:         "task1",
			Intent:         IntentStart,
			AgentProfileID: "profile1",
			Prompt:         "hello",
		})
	}()
	select {
	case <-done:
	case <-time.After(150 * time.Millisecond):
		t.Fatal("LaunchSession hung")
	}

	sessions, err := repo.ListTaskSessions(ctx, "task1")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected single session after reuse, got %d", len(sessions))
	}
	if sessions[0].ID != "session-prepared" {
		t.Fatalf("expected reused session id, got %q", sessions[0].ID)
	}
}

// TestLaunchStart_NoReuseOnProfileMismatch confirms that when the caller picks
// an agent profile that differs from the prepared session's, launchStart falls
// through to StartTask instead of silently overriding the prepared session's
// profile.
func TestLaunchStart_NoReuseOnProfileMismatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

	seedTaskAndSession(t, repo, "task1", "session-prepared", models.TaskSessionStateCreated)
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", WorkspaceID: "ws1", Title: "Test", Description: "desc"}
	if err := repo.SetSessionPrimary(ctx, "session-prepared"); err != nil {
		t.Fatalf("set primary: %v", err)
	}
	prepared, err := repo.GetTaskSession(ctx, "session-prepared")
	if err != nil {
		t.Fatalf("load seeded session: %v", err)
	}
	prepared.AgentProfileID = "profile-prepared"
	if err := repo.UpdateTaskSession(ctx, prepared); err != nil {
		t.Fatalf("update seeded profile: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer func() {
			_ = recover()
			close(done)
		}()
		_, _ = svc.LaunchSession(ctx, &LaunchSessionRequest{
			TaskID:         "task1",
			Intent:         IntentStart,
			AgentProfileID: "profile-different",
			Prompt:         "hello",
		})
	}()
	select {
	case <-done:
	case <-time.After(150 * time.Millisecond):
		t.Fatal("LaunchSession hung")
	}

	sessions, err := repo.ListTaskSessions(ctx, "task1")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected fresh session for different profile, got %d sessions", len(sessions))
	}
}

// TestEnsureSession_AutoStartDoesNotDeadlock guards the reentrant-mutex bug:
// EnsureSession acquires ensureLocks[taskID] then calls LaunchSession; when
// the step has auto_start_agent, the dispatch lands in launchStart which used
// to re-acquire the same non-reentrant mutex and freeze the request. The fix
// threads an ensureLockHeld context marker so launchStart skips the inner
// acquire — this test fails (hangs) without that marker.
func TestEnsureSession_AutoStartDoesNotDeadlock(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-auto"] = &wfmodels.WorkflowStep{
		ID:             "step-auto",
		Name:           "auto-start",
		AgentProfileID: "profile1",
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, &mockAgentManager{})

	seedTaskAndSessionWithStep(t, repo, "task1", "session-throwaway", "step-auto")
	if err := repo.DeleteTaskSession(ctx, "session-throwaway"); err != nil {
		t.Fatalf("delete seed session: %v", err)
	}
	taskRepo.tasks["task1"] = &v1.Task{
		ID:          "task1",
		WorkspaceID: "ws1",
		Title:       "Test",
		Description: "desc",
	}

	done := make(chan struct{})
	go func() {
		defer func() {
			_ = recover()
			close(done)
		}()
		_, _ = svc.EnsureSession(ctx, "task1")
	}()
	select {
	case <-done:
	case <-time.After(150 * time.Millisecond):
		t.Fatal("EnsureSession deadlocked: launchStart re-acquired ensureLock that EnsureSession already holds")
	}
}
