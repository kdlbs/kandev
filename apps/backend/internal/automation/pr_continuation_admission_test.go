package automation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakePRTaskController struct {
	busy     bool
	busyErr  error
	archived []string
	deleted  []string
}

func (f *fakePRTaskController) AutomationPRTaskBusy(context.Context, string) (bool, error) {
	return f.busy, f.busyErr
}

func (f *fakePRTaskController) ArchiveAutomationPRTask(_ context.Context, taskID string) error {
	f.archived = append(f.archived, taskID)
	return nil
}

func (f *fakePRTaskController) DeleteAutomationPRTask(_ context.Context, taskID, _ string) error {
	f.deleted = append(f.deleted, taskID)
	return nil
}

type stepLocatorFunc func(workflowID, stepID string) bool

func (f stepLocatorFunc) WorkflowStepBelongs(_ context.Context, _, workflowID, stepID string) (bool, error) {
	return f(workflowID, stepID), nil
}

func perPRRequest(name string) *CreateAutomationRequest {
	return &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: name, AgentProfileID: "agent-1", ExecutorProfileID: "exec-1",
		TaskMode: TaskModeNormalTask, WorkflowID: "wf-1", ContinuationPolicy: ContinuationPolicyReusePerPR,
		MaxConcurrentRuns: 3,
	}
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.1
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.8
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.1
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.2
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008.4
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.1
func TestReusePerPRValidation(t *testing.T) {
	svc := newTestService(t)
	svc.SetWorkflowStepLocator(stepLocatorFunc(func(workflowID, stepID string) bool {
		return workflowID == "wf-1" && stepID == "review"
	}))
	ctx := context.Background()

	req := perPRRequest("per-pr")
	req.FollowUpPrompt = "Review {{pr.previous_head_sha}}..{{pr.head_sha}}"
	req.PRContinuationStepID = "review"
	req.PRMergedAction = PREndActionArchive
	req.PRClosedAction = PREndActionDelete
	a, err := svc.CreateAutomation(ctx, req)
	require.NoError(t, err)
	require.Equal(t, 3, a.MaxConcurrentRuns, "reuse_per_pr does not force a concurrency limit")
	got, err := svc.GetAutomation(ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, ContinuationPolicyReusePerPR, got.ContinuationPolicy)
	require.Equal(t, req.PRContinuationSettings, got.PRContinuationSettings)

	defaults, err := svc.CreateAutomation(ctx, perPRRequest("defaults"))
	require.NoError(t, err)
	require.Equal(t, PREndActionKeep, defaults.PRMergedAction)
	require.Equal(t, PREndActionKeep, defaults.PRClosedAction)

	invalid := map[string]func(*CreateAutomationRequest){
		"managed conversation target": func(r *CreateAutomationRequest) { r.TaskMode = TaskModeManagedConversation },
		"follow-up without per-PR": func(r *CreateAutomationRequest) {
			r.ContinuationPolicy = ContinuationPolicyNewTask
			r.FollowUpPrompt = "x"
		},
		"end action without per-PR": func(r *CreateAutomationRequest) {
			r.ContinuationPolicy = ContinuationPolicyNewTask
			r.PRMergedAction = PREndActionArchive
		},
		"unknown end action": func(r *CreateAutomationRequest) { r.PRClosedAction = "explode" },
		"step on hidden target": func(r *CreateAutomationRequest) {
			r.TaskMode = TaskModeAutomationRun
			r.PRContinuationStepID = "review"
		},
		"foreign step": func(r *CreateAutomationRequest) { r.PRContinuationStepID = "elsewhere" },
	}
	for name, mutate := range invalid {
		t.Run(name, func(t *testing.T) {
			r := perPRRequest(name)
			mutate(r)
			_, err := svc.CreateAutomation(ctx, r)
			require.Error(t, err)
		})
	}

	newTask := ContinuationPolicyNewTask
	_, err = svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{ContinuationPolicy: &newTask})
	require.ErrorIs(t, err, ErrInvalidPRContinuationSettings, "per-PR options must be cleared before switching policy")
	empty, keep := "", PREndActionKeep
	updated, err := svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{
		ContinuationPolicy: &newTask, FollowUpPrompt: &empty, PRContinuationStepID: &empty,
		PRMergedAction: &keep, PRClosedAction: &keep,
	})
	require.NoError(t, err)
	require.Equal(t, ContinuationPolicyNewTask, updated.ContinuationPolicy)
	require.Empty(t, updated.FollowUpPrompt)
}

func TestParsePRSubject(t *testing.T) {
	owner, repo, number, ok := ParsePRSubject("acme/widget#17")
	require.True(t, ok)
	require.Equal(t, "acme", owner)
	require.Equal(t, "widget", repo)
	require.Equal(t, 17, number)
	for _, bad := range []string{"", "acme/widget", "acme#1", "/widget#1", "acme/widget#x", "acme/widget#0", "acme/widget#12abc"} {
		_, _, _, ok := ParsePRSubject(bad)
		require.False(t, ok, bad)
	}
}

func perPRFiringData(subject string) json.RawMessage {
	data, _ := json.Marshal(map[string]any{"pr_subject": subject, "event": PREventNewCommits})
	return data
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.5
func TestReusePerPRDefersWhileTaskHasOpenRun(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a, err := svc.CreateAutomation(ctx, perPRRequest("per-pr"))
	require.NoError(t, err)
	trigger := addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPR, map[string]any{"events": []string{"new_commits"}})
	require.NoError(t, svc.SetPRContinuationTaskID(ctx, a.ID, "acme/widget#17", "task-17"))

	first, err := svc.FireTrigger(ctx, a.ID, trigger.ID, TriggerTypeGitHubPR, perPRFiringData("acme/widget#17"), DedupKey("pr:acme/widget#17@aaa"))
	require.NoError(t, err)
	require.NotEmpty(t, first.RunID)
	require.NoError(t, svc.BindRunTask(ctx, first.RunID, "task-17", ""))

	deferred, err := svc.FireTrigger(ctx, a.ID, trigger.ID, TriggerTypeGitHubPR, perPRFiringData("acme/widget#17"), DedupKey("pr:acme/widget#17@bbb"))
	require.NoError(t, err)
	require.True(t, deferred.Deferred)
	require.Empty(t, deferred.RunID)
	runs, err := svc.Store().ListRuns(ctx, a.ID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1, "a deferral records no run")
	reserved, err := svc.Store().HasRunWithDedupKey(ctx, a.ID, "pr:acme/widget#17@bbb")
	require.NoError(t, err)
	require.False(t, reserved, "a deferral does not consume the dedup key")

	other, err := svc.FireTrigger(ctx, a.ID, trigger.ID, TriggerTypeGitHubPR, perPRFiringData("acme/widget#18"), DedupKey("pr:acme/widget#18@ccc"))
	require.NoError(t, err)
	require.NotEmpty(t, other.RunID, "a different PR is not blocked")

	require.NoError(t, svc.Store().MarkRunTerminal(ctx, first.RunID, "", "", RunStatusSucceeded, ""))
	retried, err := svc.FireTrigger(ctx, a.ID, trigger.ID, TriggerTypeGitHubPR, perPRFiringData("acme/widget#17"), DedupKey("pr:acme/widget#17@bbb"))
	require.NoError(t, err)
	require.NotEmpty(t, retried.RunID, "the firing proceeds once the open run ends")
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.5
func TestReusePerPRDefersWhileSessionBusy(t *testing.T) {
	cases := map[string]*fakePRTaskController{
		"busy session":  {busy: true},
		"checker error": {busyErr: errors.New("boom")},
	}
	for name, controller := range cases {
		t.Run(name, func(t *testing.T) {
			svc := newTestService(t)
			svc.SetPRTaskController(controller)
			ctx := context.Background()
			a, err := svc.CreateAutomation(ctx, perPRRequest("per-pr"))
			require.NoError(t, err)
			trigger := addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPR, map[string]any{"events": []string{"new_commits"}})
			require.NoError(t, svc.SetPRContinuationTaskID(ctx, a.ID, "acme/widget#17", "task-17"))

			result, err := svc.FireTrigger(ctx, a.ID, trigger.ID, TriggerTypeGitHubPR, perPRFiringData("acme/widget#17"), DedupKey("pr:acme/widget#17@bbb"))
			require.NoError(t, err)
			require.True(t, result.Deferred)

			// A firing without a PR subject is never deferred.
			manual, err := svc.FireTrigger(ctx, a.ID, trigger.ID, TriggerTypeGitHubPR, json.RawMessage(`{}`), DedupKey(""))
			require.NoError(t, err)
			require.False(t, manual.Deferred)
		})
	}
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.8
func TestReusePerPRRespectsMaxConcurrentRuns(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	req := perPRRequest("per-pr")
	req.MaxConcurrentRuns = 1
	a, err := svc.CreateAutomation(ctx, req)
	require.NoError(t, err)
	trigger := addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPR, map[string]any{"events": []string{"new_commits"}})

	first, err := svc.FireTrigger(ctx, a.ID, trigger.ID, TriggerTypeGitHubPR, perPRFiringData("acme/widget#17"), DedupKey("pr:acme/widget#17@a"))
	require.NoError(t, err)
	require.NotEmpty(t, first.RunID)
	second, err := svc.FireTrigger(ctx, a.ID, trigger.ID, TriggerTypeGitHubPR, perPRFiringData("acme/widget#18"), DedupKey("pr:acme/widget#18@a"))
	require.NoError(t, err)
	require.True(t, second.Skipped)
	require.False(t, second.Deferred, "the automation-wide cap skips rather than defers")
}

func TestListAutomationTaskIDsAndCleanupIncludesPRContinuations(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{
		ID:                 "a-pr-clean",
		WorkspaceID:        "ws-1",
		Name:               "PR Clean",
		Enabled:            true,
		ContinuationPolicy: ContinuationPolicyReusePerPR,
	}
	require.NoError(t, store.CreateAutomation(ctx, a))
	c := &PRContinuation{
		AutomationID: a.ID,
		PRSubject:    "owner/repo#42",
		RepoOwner:    "owner",
		RepoName:     "repo",
		PRNumber:     42,
		TaskID:       "task-pr-42",
	}
	require.NoError(t, store.UpsertPRContinuation(ctx, c))

	taskIDs, err := store.ListAutomationTaskIDs(ctx, a.ID)
	require.NoError(t, err)
	require.Contains(t, taskIDs, "task-pr-42")

	// Verify ClearContinuationAndDeleteAllRuns removes the continuation
	require.NoError(t, store.ClearContinuationAndDeleteAllRuns(ctx, a.ID))
	taskID, err := store.GetPRContinuationTaskID(ctx, a.ID, "owner/repo#42")
	require.NoError(t, err)
	require.Empty(t, taskID, "ClearContinuationAndDeleteAllRuns must clear PR continuations")
}
