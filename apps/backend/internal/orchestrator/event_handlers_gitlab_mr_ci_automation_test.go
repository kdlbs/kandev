package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

// --- Pure decision functions (AC9, AC12, AC14, AC15) ---

func TestMRAutoFixCanRun(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{gitlabMRStateOpen, true},
		{gitlabMRStateMerged, false},
		{gitlabMRStateClosed, false},
		{gitlabMRStateLocked, false},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if got := mrAutoFixCanRun(tt.state); got != tt.want {
				t.Errorf("mrAutoFixCanRun(%q) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestMRAutoFixChecksSettled(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"no pipeline", "", true},
		{"success", "success", true},
		{"failed", "failed", true},
		{"canceled", "canceled", true},
		{"skipped", "skipped", true},
		{"running", "running", false},
		{"pending", "pending", false},
		{"manual", "manual", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := &gitlab.MRAutomationSnapshot{PipelineStatus: tt.status}
			if got := mrAutoFixChecksSettled(snapshot); got != tt.want {
				t.Errorf("mrAutoFixChecksSettled(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

// TestMRAutomationReadyToMerge covers AC14 (one case per gate) and AC15
// (the detailed_merge_status fallback).
func TestMRAutomationReadyToMerge(t *testing.T) {
	ready := func() *gitlab.MRAutomationSnapshot {
		return &gitlab.MRAutomationSnapshot{
			MR: &gitlab.MR{
				State: gitlabMRStateOpen, Draft: false,
				MergeStatus: "can_be_merged", DetailedMergeStatus: "mergeable",
			},
			PipelineStatus:        "success",
			UnresolvedDiscussions: 0,
		}
	}
	tests := []struct {
		name   string
		mutate func(*gitlab.MRAutomationSnapshot)
		want   bool
	}{
		{name: "ready", want: true},
		{name: "not open", mutate: func(s *gitlab.MRAutomationSnapshot) { s.MR.State = gitlabMRStateClosed }, want: false},
		{name: "draft", mutate: func(s *gitlab.MRAutomationSnapshot) { s.MR.Draft = true }, want: false},
		{name: "pipeline not success", mutate: func(s *gitlab.MRAutomationSnapshot) { s.PipelineStatus = "failed" }, want: false},
		{name: "unresolved discussions", mutate: func(s *gitlab.MRAutomationSnapshot) { s.UnresolvedDiscussions = 1 }, want: false},
		{name: "not_approved", mutate: func(s *gitlab.MRAutomationSnapshot) { s.MR.DetailedMergeStatus = "not_approved" }, want: false},
		{name: "discussions_not_resolved", mutate: func(s *gitlab.MRAutomationSnapshot) { s.MR.DetailedMergeStatus = "discussions_not_resolved" }, want: false},
		{
			name: "pre-15.6 fallback to merge_status, cannot merge",
			mutate: func(s *gitlab.MRAutomationSnapshot) {
				s.MR.DetailedMergeStatus = ""
				s.MR.MergeStatus = "cannot_be_merged"
			},
			want: false,
		},
		{
			name: "pre-15.6 fallback to merge_status, can merge",
			mutate: func(s *gitlab.MRAutomationSnapshot) {
				s.MR.DetailedMergeStatus = ""
				s.MR.MergeStatus = "can_be_merged"
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := ready()
			if tt.mutate != nil {
				tt.mutate(snapshot)
			}
			if got := mrAutomationReadyToMerge(snapshot); got != tt.want {
				t.Errorf("mrAutomationReadyToMerge() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMRAutomationReadyToMerge_NilSnapshot(t *testing.T) {
	if mrAutomationReadyToMerge(nil) {
		t.Error("mrAutomationReadyToMerge(nil) = true, want false")
	}
}

// --- Delta builder (AC6-AC8) ---

func TestMRAutoFixBuildDelta(t *testing.T) {
	snapshot := &gitlab.MRAutomationSnapshot{
		FailingJobs: []gitlab.PipelineJob{
			{Name: "unit", Status: "failed", WebURL: "https://ci/1"}, // unchanged
			{Name: "lint", Status: "failed", WebURL: "https://ci/2"}, // new
		},
		Discussions: []gitlab.MRDiscussion{
			{
				ID: "d1", Resolvable: true, Resolved: false, Path: "main.go", Line: 12,
				Notes: []gitlab.MRNote{{ID: 10, Body: "fix this"}}, // unchanged
			},
			{
				ID: "d2", Resolvable: true, Resolved: false, Path: "main.go", Line: 20,
				Notes: []gitlab.MRNote{{ID: 11, Body: "also this"}}, // new
			},
			{
				ID: "d3", Resolvable: true, Resolved: true, // resolved — excluded entirely
				Notes: []gitlab.MRNote{{ID: 12, Body: "already handled"}},
			},
		},
	}
	previous := mrAutoFixCheckpoint{
		FailedJobs: []mrAutoFixJobSnapshot{{Name: "unit", Status: "failed", WebURL: "https://ci/1"}},
		Notes:      []mrAutoFixNoteSnapshot{{ID: 10, Body: "fix this", Path: "main.go", Line: 12}},
	}

	delta := mrAutoFixBuildDelta(snapshot, previous)
	if len(delta.FailedJobs) != 1 || delta.FailedJobs[0].Name != "lint" {
		t.Fatalf("FailedJobs = %+v, want only lint", delta.FailedJobs)
	}
	if len(delta.Notes) != 1 || delta.Notes[0].ID != 11 {
		t.Fatalf("Notes = %+v, want only note 11", delta.Notes)
	}
}

func TestMRAutoFixBuildDelta_ChangedNoteBodyCountsAsNew(t *testing.T) {
	snapshot := &gitlab.MRAutomationSnapshot{
		Discussions: []gitlab.MRDiscussion{
			{ID: "d1", Resolvable: true, Resolved: false, Notes: []gitlab.MRNote{{ID: 10, Body: "updated body"}}},
		},
	}
	previous := mrAutoFixCheckpoint{Notes: []mrAutoFixNoteSnapshot{{ID: 10, Body: "original body"}}}

	delta := mrAutoFixBuildDelta(snapshot, previous)
	if len(delta.Notes) != 1 || delta.Notes[0].Body != "updated body" {
		t.Fatalf("Notes = %+v, want the changed note", delta.Notes)
	}
}

func TestMRAutoFixBuildDelta_NilSnapshot(t *testing.T) {
	delta := mrAutoFixBuildDelta(nil, mrAutoFixCheckpoint{})
	if len(delta.FailedJobs) != 0 || len(delta.Notes) != 0 {
		t.Fatalf("delta = %+v, want empty for a nil snapshot", delta)
	}
}

// --- Round cap / exhaustion / dedupe (AC7, AC10) ---

func TestMRAutoFixRoundsExhausted(t *testing.T) {
	exhaustedAt := time.Now()
	tests := []struct {
		name  string
		state *gitlab.TaskMRLifecycleState
		want  bool
	}{
		{"nil state", nil, false},
		{"under cap", &gitlab.TaskMRLifecycleState{AutoFixRoundCount: 5}, false},
		{"at cap", &gitlab.TaskMRLifecycleState{AutoFixRoundCount: gitlab.TaskMRAutoFixMaxRounds}, true},
		{"explicitly exhausted", &gitlab.TaskMRLifecycleState{AutoFixExhaustedAt: &exhaustedAt}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mrAutoFixRoundsExhausted(tt.state); got != tt.want {
				t.Errorf("mrAutoFixRoundsExhausted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMRAutoFixDuplicateAttemptBlocksMergeAt(t *testing.T) {
	now := time.Now()
	recentEnqueue := now.Add(-30 * time.Minute)
	staleEnqueue := now.Add(-2 * time.Hour)
	tests := []struct {
		name  string
		state *gitlab.TaskMRLifecycleState
		want  bool
	}{
		{"nil state", nil, false},
		{"no prior enqueue", &gitlab.TaskMRLifecycleState{}, false},
		{"within block window", &gitlab.TaskMRLifecycleState{LastFixEnqueuedAt: &recentEnqueue}, true},
		{"outside block window", &gitlab.TaskMRLifecycleState{LastFixEnqueuedAt: &staleEnqueue}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mrAutoFixDuplicateAttemptBlocksMergeAt(tt.state, now); got != tt.want {
				t.Errorf("mrAutoFixDuplicateAttemptBlocksMergeAt() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Prompt rendering ---

func TestMRAutoFixRenderPrompt(t *testing.T) {
	mr := &gitlab.TaskMR{ProjectPath: "group/widget", MRIID: 7}
	delta := mrAutoFixCheckpoint{
		FailedJobs: []mrAutoFixJobSnapshot{{Name: "unit", Status: "failed", WebURL: "https://ci/1"}},
		Notes:      []mrAutoFixNoteSnapshot{{Path: "main.go", Line: 12, Body: "fix this"}},
	}
	prompt := mrAutoFixRenderPrompt("Base instructions\n\n{{mr.feedback}}", mr, delta)
	if !strings.Contains(prompt, "Base instructions") || !strings.Contains(prompt, "group/widget!7") || !strings.Contains(prompt, "fix this") {
		t.Fatalf("rendered prompt missing expected content:\n%s", prompt)
	}
	if strings.Contains(prompt, mrAutoFixFeedbackToken) {
		t.Fatalf("rendered prompt should replace the feedback placeholder:\n%s", prompt)
	}
	visible := sysprompt.StripSystemContent(prompt)
	if strings.Contains(visible, "Base instructions") {
		t.Fatalf("shared MR auto-fix prompt should be hidden from visible chat content, got:\n%s", visible)
	}
}

func TestMRAutoFixRenderPrompt_EmptyBase(t *testing.T) {
	if got := mrAutoFixRenderPrompt("   ", &gitlab.TaskMR{}, mrAutoFixCheckpoint{}); got != "" {
		t.Errorf("mrAutoFixRenderPrompt(empty base) = %q, want empty", got)
	}
}

// --- Integration: handleTaskMRCIAutomation (AC6, AC7, AC9, AC12, AC13, AC16) ---

func TestHandleTaskMRCIAutomation_DispatchesFixPromptOnFailingJob(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	mr := &gitlab.TaskMR{TaskID: "task-1", ProjectPath: "group/widget", MRIID: 1, State: gitlabMRStateOpen}
	fake := &mockGitLabMRAutomationService{
		snapshot: &gitlab.MRAutomationSnapshot{
			MR:             &gitlab.MR{State: gitlabMRStateOpen, IID: 1, ProjectPath: "group/widget"},
			PipelineStatus: "failed",
			FailingJobs:    []gitlab.PipelineJob{{Name: "unit", Status: "failed"}},
		},
	}
	svc.SetGitLabMRAutomationService(fake)

	svc.handleTaskMRCIAutomation(ctx, mr, &gitlab.TaskMRAutomationResponse{
		TaskID: "task-1", AutoFixEnabled: true, EffectiveAutoFixPrompt: "Fix it\n\n{{mr.feedback}}",
	})

	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 || !strings.Contains(status.Entries[0].Content, mrAutoFixMention) {
		t.Fatalf("expected one queued auto-fix prompt, got %+v", status)
	}
	if len(fake.fixAttempts) != 1 || !fake.fixAttempts[0].IncrementRound {
		t.Fatalf("fixAttempts = %+v, want one round-incrementing attempt", fake.fixAttempts)
	}
}

func TestHandleTaskMRCIAutomation_UnchangedSnapshotDispatchesNothing(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	mr := &gitlab.TaskMR{TaskID: "task-1", ProjectPath: "group/widget", MRIID: 1, State: gitlabMRStateOpen}
	snapshot := &gitlab.MRAutomationSnapshot{
		MR:             &gitlab.MR{State: gitlabMRStateOpen, IID: 1, ProjectPath: "group/widget"},
		PipelineStatus: "failed",
		FailingJobs:    []gitlab.PipelineJob{{Name: "unit", Status: "failed"}},
	}
	checkpointJSON, signature := encodeMRAutoFixCheckpoint(mrAutoFixBuildDelta(snapshot, mrAutoFixCheckpoint{}))
	fake := &mockGitLabMRAutomationService{
		snapshot:   snapshot,
		checkpoint: &gitlab.TaskMRLifecycleState{LastFixSignature: signature, LastFixCheckpointJSON: checkpointJSON},
	}
	svc.SetGitLabMRAutomationService(fake)

	svc.handleTaskMRCIAutomation(ctx, mr, &gitlab.TaskMRAutomationResponse{
		TaskID: "task-1", AutoFixEnabled: true, EffectiveAutoFixPrompt: "Fix it\n\n{{mr.feedback}}",
	})

	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 0 {
		t.Fatalf("expected no queued prompt for an unchanged snapshot, got %+v", status)
	}
	if len(fake.fixAttempts) != 0 {
		t.Fatalf("fixAttempts = %+v, want none", fake.fixAttempts)
	}
}

// TestHandleTaskMRCIAutomation_EmptyDeltaWithRecentFixStillBlocksAutoMerge
// is the regression for the missing duplicate-blocks-merge guard on the
// empty-delta path (parity with GitHub's handleTaskPRCIAutoFixEmptyDelta).
// The snapshot is simultaneously "otherwise merge-ready" and "has the same
// failing job as the checkpoint" — an artificial combination, as in
// TestHandleTaskMRCIAutomation_AutoFixBlocksAutoMergeInSamePass above, to
// isolate this guard from the pipeline-status gate that would otherwise
// mask it.
func TestHandleTaskMRCIAutomation_EmptyDeltaWithRecentFixStillBlocksAutoMerge(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	mr := &gitlab.TaskMR{TaskID: "task-1", Host: "https://gitlab.example.com", ProjectPath: "group/widget", MRIID: 1, State: gitlabMRStateOpen}
	snapshot := &gitlab.MRAutomationSnapshot{
		MR: &gitlab.MR{
			State: gitlabMRStateOpen, IID: 1, ProjectPath: "group/widget",
			MergeStatus: "can_be_merged", DetailedMergeStatus: "mergeable",
		},
		PipelineStatus: "success",
		FailingJobs:    []gitlab.PipelineJob{{Name: "unit", Status: "failed"}},
	}
	checkpointJSON, signature := encodeMRAutoFixCheckpoint(mrAutoFixBuildDelta(snapshot, mrAutoFixCheckpoint{}))
	recent := time.Now().Add(-10 * time.Minute)
	fake := &mockGitLabMRAutomationService{
		snapshot: snapshot,
		checkpoint: &gitlab.TaskMRLifecycleState{
			LastFixSignature: signature, LastFixCheckpointJSON: checkpointJSON, LastFixEnqueuedAt: &recent,
		},
	}
	svc.SetGitLabMRAutomationService(fake)

	svc.handleTaskMRCIAutomation(ctx, mr, &gitlab.TaskMRAutomationResponse{
		TaskID: "task-1", AutoFixEnabled: true, AutoMergeEnabled: true, WorkspaceID: "ws-1",
		EffectiveAutoFixPrompt: "Fix it",
	})

	if fake.mergeCalls.Load() != 0 {
		t.Fatalf("MergeMRForAutomation calls = %d, want 0 — a recently-dispatched fix must still block auto-merge on the empty-delta path", fake.mergeCalls.Load())
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 0 {
		t.Fatalf("expected no new dispatch for an unchanged snapshot, got %+v", status)
	}
}

func TestHandleTaskMRCIAutomation_RunningPipelineDispatchesNothing(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	mr := &gitlab.TaskMR{TaskID: "task-1", ProjectPath: "group/widget", MRIID: 1, State: gitlabMRStateOpen}
	fake := &mockGitLabMRAutomationService{
		snapshot: &gitlab.MRAutomationSnapshot{
			MR:             &gitlab.MR{State: gitlabMRStateOpen, IID: 1},
			PipelineStatus: "running",
			FailingJobs:    []gitlab.PipelineJob{{Name: "unit", Status: "failed"}},
		},
	}
	svc.SetGitLabMRAutomationService(fake)

	svc.handleTaskMRCIAutomation(ctx, mr, &gitlab.TaskMRAutomationResponse{
		TaskID: "task-1", AutoFixEnabled: true, EffectiveAutoFixPrompt: "Fix it",
	})

	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 0 {
		t.Fatalf("expected no dispatch while the pipeline is still running, got %+v", status)
	}
}

func TestHandleTaskMRCIAutomation_TerminalStateSkipsAutoFix(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	mr := &gitlab.TaskMR{TaskID: "task-1", ProjectPath: "group/widget", MRIID: 1, State: gitlabMRStateMerged}
	fake := &mockGitLabMRAutomationService{
		snapshot: &gitlab.MRAutomationSnapshot{
			MR:             &gitlab.MR{State: gitlabMRStateMerged},
			PipelineStatus: "failed",
			FailingJobs:    []gitlab.PipelineJob{{Name: "unit", Status: "failed"}},
		},
	}
	svc.SetGitLabMRAutomationService(fake)

	svc.handleTaskMRCIAutomation(ctx, mr, &gitlab.TaskMRAutomationResponse{
		TaskID: "task-1", AutoFixEnabled: true, EffectiveAutoFixPrompt: "Fix it",
	})

	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 0 {
		t.Fatalf("expected no dispatch for a merged MR, got %+v", status)
	}
}

// TestHandleTaskMRCIAutomation_StaleOpenRowSkipsAutoFixWhenSnapshotIsTerminal
// pins a fix for a real bug: the passed-in TaskMR row (from the DB, possibly
// a poll cycle stale) can still read "open" for an MR GitLab already reports
// merged/closed/locked when the immediate options-updated trigger fires
// before the next lightweight sync. The fresh snapshot fetched inside this
// call must be the terminal-state source of truth, not just the row.
func TestHandleTaskMRCIAutomation_StaleOpenRowSkipsAutoFixWhenSnapshotIsTerminal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	mr := &gitlab.TaskMR{TaskID: "task-1", ProjectPath: "group/widget", MRIID: 1, State: gitlabMRStateOpen}
	fake := &mockGitLabMRAutomationService{
		snapshot: &gitlab.MRAutomationSnapshot{
			MR:             &gitlab.MR{State: gitlabMRStateMerged},
			PipelineStatus: "failed",
			FailingJobs:    []gitlab.PipelineJob{{Name: "unit", Status: "failed"}},
		},
	}
	svc.SetGitLabMRAutomationService(fake)

	svc.handleTaskMRCIAutomation(ctx, mr, &gitlab.TaskMRAutomationResponse{
		TaskID: "task-1", AutoFixEnabled: true, EffectiveAutoFixPrompt: "Fix it",
	})

	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 0 {
		t.Fatalf("expected no dispatch when the fresh snapshot reports a merged MR, got %+v", status)
	}
}

func TestHandleTaskMRCIAutomation_MergesWhenReady(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	mr := &gitlab.TaskMR{TaskID: "task-1", Host: "https://gitlab.example.com", ProjectPath: "group/widget", MRIID: 1, State: gitlabMRStateOpen}
	fake := &mockGitLabMRAutomationService{
		snapshot: &gitlab.MRAutomationSnapshot{
			MR: &gitlab.MR{
				State: gitlabMRStateOpen, IID: 1, ProjectPath: "group/widget",
				MergeStatus: "can_be_merged", DetailedMergeStatus: "mergeable",
			},
			PipelineStatus:        "success",
			UnresolvedDiscussions: 0,
		},
	}
	svc.SetGitLabMRAutomationService(fake)

	svc.handleTaskMRCIAutomation(ctx, mr, &gitlab.TaskMRAutomationResponse{
		TaskID: "task-1", AutoMergeEnabled: true, WorkspaceID: "ws-1",
	})

	if fake.mergeCalls.Load() != 1 {
		t.Fatalf("MergeMRForAutomation calls = %d, want 1", fake.mergeCalls.Load())
	}
	if len(fake.mergeAttempts) != 1 {
		t.Fatalf("mergeAttempts = %+v, want one recorded attempt", fake.mergeAttempts)
	}
}

func TestHandleTaskMRCIAutomation_UnchangedMergeSignatureDoesNotReattempt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	mr := &gitlab.TaskMR{TaskID: "task-1", Host: "https://gitlab.example.com", ProjectPath: "group/widget", MRIID: 1, State: gitlabMRStateOpen}
	snapshot := &gitlab.MRAutomationSnapshot{
		MR: &gitlab.MR{
			State: gitlabMRStateOpen, IID: 1, ProjectPath: "group/widget",
			MergeStatus: "can_be_merged", DetailedMergeStatus: "mergeable",
		},
		PipelineStatus: "success",
	}
	fake := &mockGitLabMRAutomationService{
		snapshot:   snapshot,
		checkpoint: &gitlab.TaskMRLifecycleState{LastMergeSignature: mrAutoMergeSignature(snapshot)},
	}
	svc.SetGitLabMRAutomationService(fake)

	svc.handleTaskMRCIAutomation(ctx, mr, &gitlab.TaskMRAutomationResponse{
		TaskID: "task-1", AutoMergeEnabled: true, WorkspaceID: "ws-1",
	})

	if fake.mergeCalls.Load() != 0 {
		t.Fatalf("MergeMRForAutomation calls = %d, want 0 for an unchanged merge signature", fake.mergeCalls.Load())
	}
}

// TestHandleTaskMRCIAutomation_AutoFixBlocksAutoMergeInSamePass covers AC16:
// a fresh auto-fix dispatch must block auto-merge in the same evaluation
// pass, even though the readiness gate alone would otherwise pass.
func TestHandleTaskMRCIAutomation_AutoFixBlocksAutoMergeInSamePass(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	mr := &gitlab.TaskMR{TaskID: "task-1", Host: "https://gitlab.example.com", ProjectPath: "group/widget", MRIID: 1, State: gitlabMRStateOpen}
	// Snapshot is simultaneously "has a failing job auto-fix will dispatch
	// for" and "otherwise merge-ready" (success/mergeable/no discussions) —
	// the pipeline status being "success" while a job is in FailingJobs is
	// an artificial combination for this test only, to isolate AC16 from
	// AC9's checks-settled gate (which would otherwise mask this case).
	fake := &mockGitLabMRAutomationService{
		snapshot: &gitlab.MRAutomationSnapshot{
			MR: &gitlab.MR{
				State: gitlabMRStateOpen, IID: 1, ProjectPath: "group/widget",
				MergeStatus: "can_be_merged", DetailedMergeStatus: "mergeable",
			},
			PipelineStatus: "success",
			FailingJobs:    []gitlab.PipelineJob{{Name: "unit", Status: "failed"}},
		},
	}
	svc.SetGitLabMRAutomationService(fake)

	svc.handleTaskMRCIAutomation(ctx, mr, &gitlab.TaskMRAutomationResponse{
		TaskID: "task-1", AutoFixEnabled: true, AutoMergeEnabled: true, WorkspaceID: "ws-1",
		EffectiveAutoFixPrompt: "Fix it",
	})

	if fake.mergeCalls.Load() != 0 {
		t.Fatalf("MergeMRForAutomation calls = %d, want 0 — auto-fix dispatch must block auto-merge this pass", fake.mergeCalls.Load())
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 {
		t.Fatalf("expected the auto-fix prompt to still dispatch, got %+v", status)
	}
}

// TestHandleTaskMRCIAutomation_RoundCapOverflowMarksExhausted covers the
// transition into exhaustion, distinct from
// TestHandleTaskMRCIAutomation_ExhaustedAutoFixStillMergesWhenReady (which
// seeds an already-exhausted checkpoint and never reaches the dispatch
// layer). Here AutoFixRoundCount is already at the cap but
// AutoFixExhaustedAt is still nil, so a new failing job's dispatch attempt
// must reach dispatchCIAutomationPrompt, receive errCIAutoFixRoundCapReached
// (no pending queued entry to replace for a Running session with
// AllowNewRound=false), and cause handleTaskMRCIAutoFix to call
// markMRAutoFixExhausted.
func TestHandleTaskMRCIAutomation_RoundCapOverflowMarksExhausted(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	mr := &gitlab.TaskMR{TaskID: "task-1", ProjectPath: "group/widget", MRIID: 1, State: gitlabMRStateOpen}
	fake := &mockGitLabMRAutomationService{
		snapshot: &gitlab.MRAutomationSnapshot{
			MR:             &gitlab.MR{State: gitlabMRStateOpen, IID: 1, ProjectPath: "group/widget"},
			PipelineStatus: "failed",
			FailingJobs:    []gitlab.PipelineJob{{Name: "unit", Status: "failed"}},
		},
		checkpoint: &gitlab.TaskMRLifecycleState{
			TaskID: "task-1", ProjectPath: "group/widget", MRIID: 1,
			AutoFixRoundCount: gitlab.TaskMRAutoFixMaxRounds,
		},
	}
	svc.SetGitLabMRAutomationService(fake)

	svc.handleTaskMRCIAutomation(ctx, mr, &gitlab.TaskMRAutomationResponse{
		TaskID: "task-1", AutoFixEnabled: true, EffectiveAutoFixPrompt: "Fix it",
	})

	if len(fake.exhaustedCalls) != 1 || fake.exhaustedCalls[0] != "task-1" {
		t.Fatalf("exhaustedCalls = %+v, want exactly one call for task-1", fake.exhaustedCalls)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 0 {
		t.Fatalf("expected no queued prompt once the round cap is reached, got %+v", status)
	}
	if len(fake.fixAttempts) != 0 {
		t.Fatalf("fixAttempts = %+v, want none — a capped dispatch must not record a new round", fake.fixAttempts)
	}
}

// TestHandleTaskMRCIAutomation_ExhaustedAutoFixStillMergesWhenReady is the
// regression for the QA finding that a spent auto-fix round cap stranded
// auto-merge permanently. Once AutoFixExhaustedAt is set, a human can still
// fix CI by hand; the MR then satisfies every readiness gate and auto-merge
// must fire. Before the fix, handleTaskMRCIAutoFix returned true
// unconditionally here, so autoFixBlockedMerge stayed true on every
// subsequent poll and the only recovery was toggling auto-fix off.
func TestHandleTaskMRCIAutomation_ExhaustedAutoFixStillMergesWhenReady(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	mr := &gitlab.TaskMR{TaskID: "task-1", Host: "https://gitlab.example.com", ProjectPath: "group/widget", MRIID: 1, State: gitlabMRStateOpen}
	exhaustedAt := time.Now().UTC()
	fake := &mockGitLabMRAutomationService{
		snapshot: &gitlab.MRAutomationSnapshot{
			MR: &gitlab.MR{
				State: gitlabMRStateOpen, IID: 1, ProjectPath: "group/widget",
				MergeStatus: "can_be_merged", DetailedMergeStatus: "mergeable",
			},
			PipelineStatus:        "success",
			UnresolvedDiscussions: 0,
		},
		checkpoint: &gitlab.TaskMRLifecycleState{AutoFixExhaustedAt: &exhaustedAt},
	}
	svc.SetGitLabMRAutomationService(fake)

	svc.handleTaskMRCIAutomation(ctx, mr, &gitlab.TaskMRAutomationResponse{
		TaskID: "task-1", AutoFixEnabled: true, AutoMergeEnabled: true, WorkspaceID: "ws-1",
		EffectiveAutoFixPrompt: "Fix it",
	})

	if fake.mergeCalls.Load() != 1 {
		t.Fatalf("MergeMRForAutomation calls = %d, want 1 — an exhausted round cap must not strand a ready MR", fake.mergeCalls.Load())
	}
}

// TestHandleTaskMRCIAutomation_ExhaustedAutoFixStillBlocksUnreadyMerge pins
// the other half of the same fix: deferring to the readiness gate must not
// become "merge anything once exhausted". A failing pipeline still blocks.
func TestHandleTaskMRCIAutomation_ExhaustedAutoFixStillBlocksUnreadyMerge(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	mr := &gitlab.TaskMR{TaskID: "task-1", Host: "https://gitlab.example.com", ProjectPath: "group/widget", MRIID: 1, State: gitlabMRStateOpen}
	exhaustedAt := time.Now().UTC()
	fake := &mockGitLabMRAutomationService{
		snapshot: &gitlab.MRAutomationSnapshot{
			MR: &gitlab.MR{
				State: gitlabMRStateOpen, IID: 1, ProjectPath: "group/widget",
				MergeStatus: "can_be_merged", DetailedMergeStatus: "mergeable",
			},
			PipelineStatus: "failed",
		},
		checkpoint: &gitlab.TaskMRLifecycleState{AutoFixExhaustedAt: &exhaustedAt},
	}
	svc.SetGitLabMRAutomationService(fake)

	svc.handleTaskMRCIAutomation(ctx, mr, &gitlab.TaskMRAutomationResponse{
		TaskID: "task-1", AutoFixEnabled: true, AutoMergeEnabled: true, WorkspaceID: "ws-1",
		EffectiveAutoFixPrompt: "Fix it",
	})

	if fake.mergeCalls.Load() != 0 {
		t.Fatalf("MergeMRForAutomation calls = %d, want 0 — a failing pipeline must still block auto-merge", fake.mergeCalls.Load())
	}
}

// TestHandleTaskMRLifecycleAutomation_AutoMergeOnOneMRDoesNotMergeAnother is
// the orchestrator-level statement of the whole per-MR change: auto-merge
// enabled on one linked MR must not merge a sibling MR on the same task.
//
// It deliberately enters through handleTaskMRLifecycleAutomation rather than
// calling handleTaskMRCIAutomation with hand-built options, because the
// per-MR resolution being verified happens in GetTaskMRAutomationEvaluation —
// options passed in directly would assume away the thing under test.
func TestHandleTaskMRLifecycleAutomation_AutoMergeOnOneMRDoesNotMergeAnother(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	mergeableSnapshot := func(iid int) *gitlab.MRAutomationSnapshot {
		return &gitlab.MRAutomationSnapshot{
			MR: &gitlab.MR{
				State: gitlabMRStateOpen, IID: iid, ProjectPath: "group/widget",
				MergeStatus: "can_be_merged", DetailedMergeStatus: "mergeable",
			},
			PipelineStatus:        "success",
			UnresolvedDiscussions: 0,
		}
	}
	// Auto-merge is on for MR !1 only; !2 is left all-off.
	fake := &mockGitLabMRAutomationService{
		optionsByMRIID: map[int]*gitlab.TaskMRAutomationResponse{
			1: {TaskID: "task-1", AutoMergeEnabled: true, WorkspaceID: "ws-1"},
			2: {TaskID: "task-1", WorkspaceID: "ws-1"},
		},
		snapshot: mergeableSnapshot(2),
	}
	svc.SetGitLabMRAutomationService(fake)

	mrTwo := &gitlab.TaskMR{
		TaskID: "task-1", Host: "https://gitlab.example.com",
		ProjectPath: "group/widget", MRIID: 2, State: gitlabMRStateOpen,
	}
	if err := svc.handleTaskMRLifecycleAutomation(ctx, mrTwo); err != nil {
		t.Fatalf("evaluate MR !2: %v", err)
	}
	if got := fake.mergeCalls.Load(); got != 0 {
		t.Fatalf("MergeMRForAutomation calls for MR !2 = %d, want 0 — a sibling MR's auto-merge must not merge this one", got)
	}

	// Same task, same fully-mergeable state, but this MR owns the switch.
	fake.snapshot = mergeableSnapshot(1)
	mrOne := &gitlab.TaskMR{
		TaskID: "task-1", Host: "https://gitlab.example.com",
		ProjectPath: "group/widget", MRIID: 1, State: gitlabMRStateOpen,
	}
	if err := svc.handleTaskMRLifecycleAutomation(ctx, mrOne); err != nil {
		t.Fatalf("evaluate MR !1: %v", err)
	}
	if got := fake.mergeCalls.Load(); got != 1 {
		t.Fatalf("MergeMRForAutomation calls after MR !1 = %d, want 1 — the MR that owns the switch must still merge", got)
	}
}
