package orchestrator

//revive:disable:file-length-limit // CI automation regression coverage is intentionally scenario-heavy.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

func TestCIAutomationReadyToMerge(t *testing.T) {
	required := 1
	ready := github.TaskPR{
		State:                   "open",
		ChecksState:             "success",
		ReviewState:             "approved",
		MergeableState:          "clean",
		ReviewCount:             1,
		PendingReviewCount:      0,
		RequiredReviews:         &required,
		UnresolvedReviewThreads: 0,
	}
	tests := []struct {
		name   string
		mutate func(*github.TaskPR)
		want   bool
	}{
		{name: "ready", want: true},
		{name: "failing checks", mutate: func(pr *github.TaskPR) { pr.ChecksState = "failure" }, want: false},
		{name: "dirty", mutate: func(pr *github.TaskPR) { pr.MergeableState = "dirty" }, want: false},
		{name: "pending review", mutate: func(pr *github.TaskPR) { pr.PendingReviewCount = 1 }, want: false},
		{name: "not enough approvals", mutate: func(pr *github.TaskPR) { pr.ReviewCount = 0 }, want: false},
		{name: "unresolved threads", mutate: func(pr *github.TaskPR) { pr.UnresolvedReviewThreads = 1 }, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := ready
			if tt.mutate != nil {
				tt.mutate(&pr)
			}
			if got := ciAutomationReadyToMerge(&pr); got != tt.want {
				t.Fatalf("ciAutomationReadyToMerge=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestCIAutomationFeedbackDelta(t *testing.T) {
	feedback := &github.PRFeedback{
		Checks: []github.CheckRun{
			{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/1"},
			{Name: "lint", Status: "completed", Conclusion: "success", HTMLURL: "https://ci/2"},
		},
		Comments: []github.PRComment{
			{ID: 10, Body: "fix this", Path: "main.go", Line: 12},
			{ID: 11, Body: "also this", Path: "main.go", Line: 20},
		},
	}
	checkpoint := ciAutomationCheckpoint{
		FailedChecks: []ciAutomationCheckSnapshot{{Name: "unit", Conclusion: "failure", HTMLURL: "https://ci/1"}},
		Comments:     []ciAutomationCommentSnapshot{{ID: 10, Body: "fix this", Path: "main.go", Line: 12}},
	}

	delta := ciAutomationBuildDelta(feedback, checkpoint)
	if len(delta.FailedChecks) != 0 {
		t.Fatalf("expected no new failed checks, got %+v", delta.FailedChecks)
	}
	if len(delta.Comments) != 1 || delta.Comments[0].ID != 11 {
		t.Fatalf("expected only comment 11, got %+v", delta.Comments)
	}
	prompt := ciAutomationRenderPrompt("Base instructions\n\n{{pr.feedback}}", &github.TaskPR{Owner: "acme", Repo: "widget", PRNumber: 42}, delta)
	if !strings.Contains(prompt, "Base instructions") || !strings.Contains(prompt, "acme/widget#42") || !strings.Contains(prompt, "also this") {
		t.Fatalf("rendered prompt missing expected content:\n%s", prompt)
	}
	if strings.Contains(prompt, "{{pr.feedback}}") {
		t.Fatalf("rendered prompt should replace PR feedback placeholder:\n%s", prompt)
	}
	visible := sysprompt.StripSystemContent(prompt)
	if strings.Contains(visible, "Base instructions") {
		t.Fatalf("shared CI prompt should be hidden from visible chat content, got:\n%s", visible)
	}
	if !strings.Contains(visible, "acme/widget#42") || !strings.Contains(visible, "also this") {
		t.Fatalf("PR snapshot should remain visible, got:\n%s", visible)
	}
}

func TestCIAutomationPromptOmitsSnapshotWithoutPlaceholder(t *testing.T) {
	delta := ciAutomationCheckpoint{
		FailedChecks: []ciAutomationCheckSnapshot{{Name: "unit", Conclusion: "failure", HTMLURL: "https://ci/unit"}},
		Comments:     []ciAutomationCommentSnapshot{{ID: 10, Body: "fix this", Path: "main.go", Line: 12}},
	}

	prompt := ciAutomationRenderPrompt("Pull the branch and inspect the PR yourself.", &github.TaskPR{Owner: "acme", Repo: "widget", PRNumber: 42}, delta)
	if strings.Contains(prompt, "acme/widget#42") || strings.Contains(prompt, "unit") || strings.Contains(prompt, "fix this") {
		t.Fatalf("rendered prompt should omit PR snapshot without placeholder:\n%s", prompt)
	}
	if visible := sysprompt.StripSystemContent(prompt); strings.TrimSpace(visible) != "" {
		t.Fatalf("expected no visible PR snapshot without placeholder, got:\n%s", visible)
	}
}

func TestCIAutomationCheckpointPrunesResolvedFailures(t *testing.T) {
	failed := &github.PRFeedback{
		Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/stable"}},
	}
	previous := ciAutomationCurrentCheckpoint(failed)

	passing := &github.PRFeedback{
		Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "success", HTMLURL: "https://ci/stable"}},
	}
	pruned := ciAutomationCurrentCheckpoint(passing)
	if len(pruned.FailedChecks) != 0 {
		t.Fatalf("expected passing check to be pruned, got %+v", pruned.FailedChecks)
	}

	regressed := ciAutomationBuildDelta(failed, pruned)
	if len(regressed.FailedChecks) != 1 {
		t.Fatalf("expected same check to retrigger after prune, got %+v", regressed.FailedChecks)
	}
	if again := ciAutomationBuildDelta(failed, previous); len(again.FailedChecks) != 0 {
		t.Fatalf("expected unchanged failure to remain deduped, got %+v", again.FailedChecks)
	}
}

func TestCIAutomationFeedbackDeltaIncludesEditedComments(t *testing.T) {
	previous := ciAutomationCheckpoint{
		Comments: []ciAutomationCommentSnapshot{{ID: 10, Body: "old body", Path: "main.go", Line: 12}},
	}
	feedback := &github.PRFeedback{
		Comments: []github.PRComment{{ID: 10, Body: "new body", Path: "main.go", Line: 12}},
	}

	delta := ciAutomationBuildDelta(feedback, previous)
	if len(delta.Comments) != 1 || delta.Comments[0].Body != "new body" {
		t.Fatalf("expected edited comment in delta, got %+v", delta.Comments)
	}
}

func TestCIAutomationFilterFeedbackForPRSkipsReviewCommentsWithoutUnresolvedThreads(t *testing.T) {
	feedback := &github.PRFeedback{
		Comments: []github.PRComment{
			{ID: 1, Body: "resolved review comment", Path: "main.go", Line: 12},
			{ID: 2, Body: "plain PR comment"},
			{ID: 3, Body: "bot status comment", AuthorIsBot: true},
		},
	}
	filtered := ciAutomationFilterFeedbackForPR(&github.TaskPR{}, feedback)
	if len(filtered.Comments) != 1 || filtered.Comments[0].ID != 2 {
		t.Fatalf("expected only human plain PR comment, got %+v", filtered.Comments)
	}
	withThreads := ciAutomationFilterFeedbackForPR(&github.TaskPR{UnresolvedReviewThreads: 1}, feedback)
	if len(withThreads.Comments) != 2 || withThreads.Comments[0].ID != 1 || withThreads.Comments[1].ID != 2 {
		t.Fatalf("expected unresolved review threads to keep review comments without bot status comments, got %+v", withThreads.Comments)
	}
}

func TestCIAutomationFeedbackDeltaIncludesChangedCheckOutput(t *testing.T) {
	previous := ciAutomationCheckpoint{
		FailedChecks: []ciAutomationCheckSnapshot{{Name: "unit", Conclusion: "failure", HTMLURL: "https://ci/unit", Output: "old"}},
	}
	feedback := &github.PRFeedback{
		Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit", Output: "new"}},
	}

	delta := ciAutomationBuildDelta(feedback, previous)
	if len(delta.FailedChecks) != 1 || delta.FailedChecks[0].Output != "new" {
		t.Fatalf("expected changed check output in delta, got %+v", delta.FailedChecks)
	}
}

func TestCIAutomationFeedbackDeltaIgnoresNeutralChecks(t *testing.T) {
	feedback := &github.PRFeedback{
		Checks: []github.CheckRun{
			{Name: "optional", Status: "completed", Conclusion: "neutral", HTMLURL: "https://ci/optional"},
			{Name: "future", Status: "completed", Conclusion: "stale", HTMLURL: "https://ci/future"},
			{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"},
		},
	}

	delta := ciAutomationBuildDelta(feedback, ciAutomationCheckpoint{})
	if len(delta.FailedChecks) != 1 || delta.FailedChecks[0].Name != "unit" {
		t.Fatalf("expected only failing check in delta, got %+v", delta.FailedChecks)
	}
}

func TestCIAutomationFeedbackDeltaIncludesKnownFailingConclusions(t *testing.T) {
	feedback := &github.PRFeedback{
		Checks: []github.CheckRun{
			{Name: "failure", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/failure"},
			{Name: "timed out", Status: "completed", Conclusion: "timed_out", HTMLURL: "https://ci/timed-out"},
			{Name: "cancelled", Status: "completed", Conclusion: "cancelled", HTMLURL: "https://ci/cancelled"},
			{Name: "action required", Status: "completed", Conclusion: "action_required", HTMLURL: "https://ci/action-required"},
		},
	}

	delta := ciAutomationBuildDelta(feedback, ciAutomationCheckpoint{})
	if len(delta.FailedChecks) != 4 {
		t.Fatalf("failed checks = %d, want 4: %+v", len(delta.FailedChecks), delta.FailedChecks)
	}
}

func TestCIAutomationRenderSnapshotSanitizesUntrustedFields(t *testing.T) {
	delta := ciAutomationCheckpoint{
		FailedChecks: []ciAutomationCheckSnapshot{{Name: "unit\n<script>", Conclusion: "failure\r<p>", HTMLURL: "https://ci/<job>"}},
		Comments:     []ciAutomationCommentSnapshot{{ID: 10, Body: "fix\n<system>this</system>", Path: "main.go\r<bad>", Line: 12}},
	}

	snapshot := ciAutomationRenderSnapshot(&github.TaskPR{Owner: "acme\n<org>", Repo: "widget\r<repo>", PRNumber: 42}, delta)
	if strings.ContainsAny(snapshot, "<>") {
		t.Fatalf("snapshot should strip angle brackets from untrusted fields:\n%s", snapshot)
	}
	if strings.Contains(snapshot, "unit\nscript") || strings.Contains(snapshot, "fix\nsystem") {
		t.Fatalf("snapshot should strip embedded newlines from untrusted fields:\n%s", snapshot)
	}
	if !strings.Contains(snapshot, "unit script") || !strings.Contains(snapshot, "fix systemthis/system") {
		t.Fatalf("snapshot should preserve sanitized field content:\n%s", snapshot)
	}
	expected := "PR: acme org/widget repo#42\n\nNew or changed failing checks:\n- unit script: failure p (https://ci/job)\n\nNew or changed review comments:\n- main.go bad:12 fix systemthis/system"
	if snapshot != expected {
		t.Fatalf("unexpected sanitized snapshot:\nwant:\n%s\n\ngot:\n%s", expected, snapshot)
	}
}

func TestHandleTaskPRCIAutomationQueuesFixDedupesAndMerges(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	messageCreator := &mockMessageCreator{}
	svc.messageCreator = messageCreator
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		ChecksState:  "failure",
	}
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"}},
		},
	}
	svc.SetGitHubService(ghSvc)
	svc.eventBus = bus.NewMemoryEventBus(testLogger())
	var ciOptionsEvents []*bus.Event
	_, err := svc.eventBus.Subscribe(events.GitHubTaskCIOptionsUpdated, func(_ context.Context, event *bus.Event) error {
		ciOptionsEvents = append(ciOptionsEvents, event)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe CI options events: %v", err)
	}

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle auto-fix: %v", err)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 || !strings.Contains(status.Entries[0].Content, "@ci-auto-fix") || !strings.Contains(status.Entries[0].Content, "acme/widget#42") || !strings.Contains(status.Entries[0].Content, "unit") {
		t.Fatalf("expected queued CI fix prompt, got %+v", status)
	}
	if status.Entries[0].Metadata[metaKeyUserMessageRecorded] == true {
		t.Fatalf("expected queued CI prompt to be recorded when drained, got %+v", status.Entries[0].Metadata)
	}
	if len(messageCreator.userMessages) != 0 {
		t.Fatalf("expected queued CI automation to record chat message on drain, got %d", len(messageCreator.userMessages))
	}
	if len(ghSvc.fixAttempts) != 1 {
		t.Fatalf("expected one fix attempt, got %d", len(ghSvc.fixAttempts))
	}
	if len(ciOptionsEvents) != 1 || ciOptionsEvents[0].Source != ciAutomationStateEventSource {
		t.Fatalf("expected one CI options state refresh event, got %+v", ciOptionsEvents)
	}

	_, signature := encodeCIAutomationCheckpoint(ciAutomationCurrentCheckpoint(ghSvc.prFeedback))
	ghSvc.ciPRState = &github.TaskCIPRAutomationState{LastFixSignature: signature, LastFixCheckpointJSON: ghSvc.fixAttempts[0].CheckpointJSON}
	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle dedupe: %v", err)
	}
	if got := svc.messageQueue.GetStatus(ctx, "session-1").Count; got != 1 {
		t.Fatalf("expected dedupe to avoid second queued prompt, got %d", got)
	}
	if len(messageCreator.userMessages) != 0 {
		t.Fatalf("expected dedupe to avoid second chat message, got %d", len(messageCreator.userMessages))
	}

	pr.ChecksState = "success"
	pr.ReviewState = "approved"
	pr.MergeableState = "clean"
	now := time.Now().UTC()
	pr.LastSyncedAt = &now
	ghSvc.ciOptionsResp.AutoFixEnabled = false
	ghSvc.ciOptionsResp.AutoMergeEnabled = true
	ghSvc.triggerPRSyncAllPRs = []*github.TaskPR{pr}
	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle auto-merge: %v", err)
	}
	if ghSvc.mergeCalls != 1 || len(ghSvc.mergeAttempts) != 1 {
		t.Fatalf("expected one merge call and attempt, got calls=%d attempts=%d", ghSvc.mergeCalls, len(ghSvc.mergeAttempts))
	}
}

func TestHandleTaskPRCIAutomationAutoFixUsesFreshSyncAndFullFeedback(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		ChecksState:  "success",
	}
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		triggerPRSyncAllPRs: []*github.TaskPR{{
			TaskID:       "task-1",
			RepositoryID: "repo-1",
			Owner:        "acme",
			Repo:         "widget",
			PRNumber:     42,
			State:        "open",
			ChecksState:  "success",
		}},
		prFeedback: &github.PRFeedback{
			Comments: []github.PRComment{{ID: 99, Body: "plain PR comment should trigger auto-fix"}},
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle auto-fix: %v", err)
	}
	if ghSvc.triggerPRSyncAllCalls != 1 {
		t.Fatalf("TriggerPRSyncAll calls = %d, want 1", ghSvc.triggerPRSyncAllCalls)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 || !strings.Contains(status.Entries[0].Content, "plain PR comment should trigger auto-fix") {
		t.Fatalf("expected queued auto-fix prompt from full feedback, got %+v", status)
	}
}

func TestHandleTaskPRCIAutomationAutoFixSkipsBotIssueCommentWithoutActionableFeedback(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	pr := &github.TaskPR{
		TaskID:                  "task-1",
		RepositoryID:            "repo-1",
		Owner:                   "acme",
		Repo:                    "widget",
		PRNumber:                42,
		State:                   "open",
		ChecksState:             "success",
		UnresolvedReviewThreads: 0,
	}
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		triggerPRSyncAllPRs: []*github.TaskPR{pr},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "success"}},
			Comments: []github.PRComment{{
				ID:          99,
				Author:      "coderabbitai[bot]",
				AuthorIsBot: true,
				Body:        "Review in progress. No actionable comments posted yet.",
			}},
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle auto-fix: %v", err)
	}
	if ghSvc.prFeedbackCalls != 1 {
		t.Fatalf("expected one feedback fetch, got %d", ghSvc.prFeedbackCalls)
	}
	if status := svc.messageQueue.GetStatus(ctx, "session-1"); status.Count != 0 {
		t.Fatalf("expected no queued auto-fix prompt for bot status comment, got %+v", status)
	}
	if len(ghSvc.fixAttempts) != 0 {
		t.Fatalf("expected no fix round for bot status comment, got %+v", ghSvc.fixAttempts)
	}
}

func TestHandleTaskPRCIAutomationAutoFixWaitsForPendingChecks(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	pr := &github.TaskPR{
		TaskID:                  "task-1",
		RepositoryID:            "repo-1",
		Owner:                   "acme",
		Repo:                    "widget",
		PRNumber:                42,
		State:                   "open",
		ChecksState:             "pending",
		UnresolvedReviewThreads: 1,
	}
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		triggerPRSyncAllPRs: []*github.TaskPR{pr},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{
				{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"},
				{Name: "integration", Status: "in_progress", HTMLURL: "https://ci/integration"},
			},
			Comments: []github.PRComment{{ID: 100, Body: "please address this", Path: "main.go", Line: 12}},
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle auto-fix: %v", err)
	}
	if ghSvc.prFeedbackCalls != 1 {
		t.Fatalf("expected one feedback fetch, got %d", ghSvc.prFeedbackCalls)
	}
	if status := svc.messageQueue.GetStatus(ctx, "session-1"); status.Count != 0 {
		t.Fatalf("expected no queued auto-fix prompt while checks are pending, got %+v", status)
	}
	if len(ghSvc.fixAttempts) != 0 {
		t.Fatalf("expected no fix round while checks are pending, got %+v", ghSvc.fixAttempts)
	}
}

func TestHandleTaskPRCIAutomationAutoFixSkipsClosedFetchedPR(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	now := time.Now().UTC()
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		ChecksState:  "success",
		LastSyncedAt: &now,
	}
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		triggerPRSyncAllPRs: []*github.TaskPR{pr},
		prFeedback: &github.PRFeedback{
			PR:       &github.PR{State: "closed"},
			Comments: []github.PRComment{{ID: 99, Body: "plain PR comment"}},
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle auto-fix: %v", err)
	}
	if status := svc.messageQueue.GetStatus(ctx, "session-1"); status.Count != 0 {
		t.Fatalf("expected no prompt for closed fetched PR, got %+v", status)
	}
}

func TestHandleTaskPRCIAutomationAutoMergeUsesFreshSync(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	stale := &github.TaskPR{
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		Owner:          "acme",
		Repo:           "widget",
		PRNumber:       42,
		State:          "open",
		ChecksState:    "failure",
		MergeableState: "dirty",
	}
	fresh := *stale
	fresh.ChecksState = "success"
	fresh.ReviewState = "approved"
	fresh.MergeableState = "clean"
	now := time.Now().UTC()
	fresh.LastSyncedAt = &now
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:           "task-1",
			AutoMergeEnabled: true,
		},
		triggerPRSyncAllPRs: []*github.TaskPR{&fresh},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, stale); err != nil {
		t.Fatalf("handle auto-merge: %v", err)
	}
	if ghSvc.triggerPRSyncAllCalls != 1 {
		t.Fatalf("TriggerPRSyncAll calls = %d, want 1", ghSvc.triggerPRSyncAllCalls)
	}
	if ghSvc.mergeCalls != 1 {
		t.Fatalf("expected merge from fresh synced PR state, got %d", ghSvc.mergeCalls)
	}
}

func TestHandleTaskPRCIAutomationAutoMergeUsesPartialSyncMatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	now := time.Now().UTC()
	pr := &github.TaskPR{
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		Owner:          "acme",
		Repo:           "widget",
		PRNumber:       42,
		State:          "open",
		ChecksState:    "success",
		ReviewState:    "approved",
		MergeableState: "clean",
		LastSyncedAt:   &now,
	}
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:           "task-1",
			AutoMergeEnabled: true,
		},
		triggerPRSyncAllPRs: []*github.TaskPR{pr},
		triggerPRSyncAllErr: &github.PartialPRSyncError{Err: errors.New("sibling repo unavailable")},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle auto-merge: %v", err)
	}
	if ghSvc.mergeCalls != 1 {
		t.Fatalf("expected matching partial sync result to merge, got %d calls", ghSvc.mergeCalls)
	}
	if len(ghSvc.ciErrors) != 0 {
		t.Fatalf("expected no CI error on matching partial result, got %+v", ghSvc.ciErrors)
	}
}

func TestHandleTaskPRCIAutomationAutoMergeRequiresFreshSync(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	staleReady := &github.TaskPR{
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		Owner:          "acme",
		Repo:           "widget",
		PRNumber:       42,
		State:          "open",
		ChecksState:    "success",
		ReviewState:    "approved",
		MergeableState: "clean",
	}
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:           "task-1",
			AutoMergeEnabled: true,
		},
		triggerPRSyncAllPRs: []*github.TaskPR{staleReady},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, staleReady); err != nil {
		t.Fatalf("handle auto-merge: %v", err)
	}
	if ghSvc.mergeCalls != 0 {
		t.Fatalf("expected stale synced state not to merge, got %d merge calls", ghSvc.mergeCalls)
	}
	if len(ghSvc.ciErrors) != 1 || ghSvc.ciErrors[0].LastError == nil || !strings.Contains(*ghSvc.ciErrors[0].LastError, "not freshly synced") {
		t.Fatalf("expected stale sync error to be recorded, got %+v", ghSvc.ciErrors)
	}
}

func TestCIAutomationHasFreshPRStatusAt(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	exactEdge := now.Add(-github.PRSyncFreshnessWindow)
	stale := exactEdge.Add(-time.Nanosecond)
	fresh := now.Add(-github.PRSyncFreshnessWindow + time.Nanosecond)
	tests := []struct {
		name string
		pr   *github.TaskPR
		want bool
	}{
		{name: "nil PR", pr: nil, want: false},
		{name: "nil last synced", pr: &github.TaskPR{}, want: false},
		{name: "fresh within window", pr: &github.TaskPR{LastSyncedAt: &fresh}, want: true},
		{name: "fresh at exact edge", pr: &github.TaskPR{LastSyncedAt: &exactEdge}, want: true},
		{name: "stale older than edge", pr: &github.TaskPR{LastSyncedAt: &stale}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ciAutomationHasFreshPRStatusAt(tt.pr, now); got != tt.want {
				t.Fatalf("ciAutomationHasFreshPRStatusAt=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestCIAutomationDuplicateFixAttemptBlocksMergeAt(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-ciAutomationFixBlockWindow)
	stale := fresh.Add(-time.Nanosecond)
	if ciAutomationDuplicateFixAttemptBlocksMergeAt(nil, now) {
		t.Fatal("nil state should not block")
	}
	if ciAutomationDuplicateFixAttemptBlocksMergeAt(&github.TaskCIPRAutomationState{}, now) {
		t.Fatal("state without enqueue time should not block")
	}
	if !ciAutomationDuplicateFixAttemptBlocksMergeAt(&github.TaskCIPRAutomationState{LastFixEnqueuedAt: &fresh}, now) {
		t.Fatal("fresh duplicate fix attempt should block")
	}
	if ciAutomationDuplicateFixAttemptBlocksMergeAt(&github.TaskCIPRAutomationState{LastFixEnqueuedAt: &stale}, now) {
		t.Fatal("stale duplicate fix attempt should not block")
	}
}

func TestHandleTaskPRCIAutomationAutoFixBlocksSameCycleMerge(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	now := time.Now().UTC()
	pr := &github.TaskPR{
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		Owner:          "acme",
		Repo:           "widget",
		PRNumber:       42,
		State:          "open",
		ChecksState:    "success",
		ReviewState:    "approved",
		MergeableState: "clean",
		LastSyncedAt:   &now,
	}
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			AutoMergeEnabled:       true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		triggerPRSyncAllPRs: []*github.TaskPR{pr},
		prFeedback: &github.PRFeedback{
			Comments: []github.PRComment{{ID: 100, Body: "please address before merge"}},
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle CI automation: %v", err)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 {
		t.Fatalf("expected auto-fix prompt to be queued, got %+v", status)
	}
	if ghSvc.mergeCalls != 0 {
		t.Fatalf("expected auto-merge to wait for a later cycle after auto-fix prompt, got %d merge calls", ghSvc.mergeCalls)
	}
}

func TestHandleTaskPRCIAutomationDuplicateFixAttemptBlocksMerge(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	now := time.Now().UTC()
	pr := &github.TaskPR{
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		Owner:          "acme",
		Repo:           "widget",
		PRNumber:       42,
		State:          "open",
		ChecksState:    "success",
		ReviewState:    "approved",
		MergeableState: "clean",
		LastSyncedAt:   &now,
	}
	feedback := &github.PRFeedback{
		Comments: []github.PRComment{{ID: 100, Body: "please address before merge"}},
	}
	_, signature := encodeCIAutomationCheckpoint(ciAutomationCurrentCheckpoint(feedback))
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			AutoMergeEnabled:       true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		triggerPRSyncAllPRs: []*github.TaskPR{pr},
		prFeedback:          feedback,
		ciPRState: &github.TaskCIPRAutomationState{
			LastFixSignature:      signature,
			LastFixCheckpointJSON: `{"comments":[{"id":100,"body":"please address before merge"}]}`,
			LastFixEnqueuedAt:     &now,
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle CI automation: %v", err)
	}
	if status := svc.messageQueue.GetStatus(ctx, "session-1"); status.Count != 0 {
		t.Fatalf("expected duplicate fix signature not to queue another prompt, got %+v", status)
	}
	if ghSvc.mergeCalls != 0 {
		t.Fatalf("expected duplicate pending fix attempt to block merge, got %d merge calls", ghSvc.mergeCalls)
	}
}

func TestHandleTaskPRCIAutomationCoalescesQueuedAutoFixForRunningSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	now := time.Now().UTC()
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		LastSyncedAt: &now,
	}
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		triggerPRSyncAllPRs: []*github.TaskPR{pr},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"}},
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle first auto-fix: %v", err)
	}
	if len(ghSvc.fixAttempts) != 1 {
		t.Fatalf("expected first fix attempt, got %+v", ghSvc.fixAttempts)
	}
	ghSvc.ciPRState = &github.TaskCIPRAutomationState{
		TaskID:                "task-1",
		RepositoryID:          "repo-1",
		PRNumber:              42,
		LastFixSignature:      ghSvc.fixAttempts[0].Signature,
		LastFixCheckpointJSON: ghSvc.fixAttempts[0].CheckpointJSON,
		LastFixEnqueuedAt:     &now,
		LastFixSessionID:      ptrString("session-1"),
	}
	ghSvc.prFeedback = &github.PRFeedback{
		Checks: []github.CheckRun{
			{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"},
			{Name: "lint", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/lint"},
		},
	}

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle second auto-fix: %v", err)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 {
		t.Fatalf("expected queued CI auto-fix to be replaced, got %+v", status)
	}
	if !strings.Contains(status.Entries[0].Content, "lint") {
		t.Fatalf("expected queued prompt to contain latest CI feedback, got %q", status.Entries[0].Content)
	}
	if strings.Contains(status.Entries[0].Content, "https://ci/unit") {
		t.Fatalf("expected replacement to avoid stale appended feedback, got %q", status.Entries[0].Content)
	}
}

func TestHandleTaskPRCIAutomationStartsAutoFixOnPrimarySession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	if err := repo.SetSessionPrimary(ctx, "session-1"); err != nil {
		t.Fatalf("set primary session: %v", err)
	}
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        "session-2",
		TaskID:    "task-1",
		State:     models.TaskSessionStateRunning,
		StartedAt: now.Add(time.Minute),
		UpdatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("create newer session: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		LastSyncedAt: &now,
	}
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		triggerPRSyncAllPRs: []*github.TaskPR{pr},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{
				{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"},
			},
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle auto-fix: %v", err)
	}
	if status := svc.messageQueue.GetStatus(ctx, "session-2"); status.Count != 0 {
		t.Fatalf("expected newer non-primary session not to receive first auto-fix, got %+v", status)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 || !strings.Contains(status.Entries[0].Content, "unit") {
		t.Fatalf("expected primary session to receive first auto-fix, got %+v", status)
	}
	if len(ghSvc.fixAttempts) != 1 || ghSvc.fixAttempts[0].SessionID != "session-1" {
		t.Fatalf("expected first fix attempt to use primary session, got %+v", ghSvc.fixAttempts)
	}
}

func TestHandleTaskPRCIAutomationKeepsAutoFixOnPreviousSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        "session-2",
		TaskID:    "task-1",
		State:     models.TaskSessionStateRunning,
		StartedAt: now.Add(time.Minute),
		UpdatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("create newer session: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		LastSyncedAt: &now,
	}
	previousCheckpoint := ciAutomationCheckpoint{
		FailedChecks: []ciAutomationCheckSnapshot{
			{Name: "unit", Conclusion: "failure", HTMLURL: "https://ci/unit"},
		},
	}
	previousJSON, previousSignature := encodeCIAutomationCheckpoint(previousCheckpoint)
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		triggerPRSyncAllPRs: []*github.TaskPR{pr},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{
				{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"},
				{Name: "lint", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/lint"},
			},
		},
		ciPRState: &github.TaskCIPRAutomationState{
			TaskID:                "task-1",
			RepositoryID:          "repo-1",
			PRNumber:              42,
			LastFixSignature:      previousSignature,
			LastFixCheckpointJSON: previousJSON,
			LastFixEnqueuedAt:     &now,
			LastFixSessionID:      ptrString("session-1"),
			AutoFixRoundCount:     1,
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle auto-fix: %v", err)
	}
	if status := svc.messageQueue.GetStatus(ctx, "session-2"); status.Count != 0 {
		t.Fatalf("expected newer session not to receive auto-fix, got %+v", status)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 || !strings.Contains(status.Entries[0].Content, "lint") {
		t.Fatalf("expected previous auto-fix session to receive latest feedback, got %+v", status)
	}
	if len(ghSvc.fixAttempts) != 1 || ghSvc.fixAttempts[0].SessionID != "session-1" {
		t.Fatalf("expected fix attempt to stay on session-1, got %+v", ghSvc.fixAttempts)
	}
}

func TestHandleTaskPRCIAutomationFallsBackWhenPreviousSessionInactive(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateCompleted)
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        "session-2",
		TaskID:    "task-1",
		State:     models.TaskSessionStateRunning,
		StartedAt: now.Add(time.Minute),
		UpdatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("create active fallback session: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		LastSyncedAt: &now,
	}
	previousCheckpoint := ciAutomationCheckpoint{
		FailedChecks: []ciAutomationCheckSnapshot{
			{Name: "unit", Conclusion: "failure", HTMLURL: "https://ci/unit"},
		},
	}
	previousJSON, previousSignature := encodeCIAutomationCheckpoint(previousCheckpoint)
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		triggerPRSyncAllPRs: []*github.TaskPR{pr},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{
				{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"},
				{Name: "lint", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/lint"},
			},
		},
		ciPRState: &github.TaskCIPRAutomationState{
			TaskID:                "task-1",
			RepositoryID:          "repo-1",
			PRNumber:              42,
			LastFixSignature:      previousSignature,
			LastFixCheckpointJSON: previousJSON,
			LastFixSessionID:      ptrString("session-1"),
			AutoFixRoundCount:     1,
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle auto-fix: %v", err)
	}
	if status := svc.messageQueue.GetStatus(ctx, "session-1"); status.Count != 0 {
		t.Fatalf("expected inactive previous session not to receive auto-fix, got %+v", status)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-2")
	if status.Count != 1 || !strings.Contains(status.Entries[0].Content, "lint") {
		t.Fatalf("expected active fallback session to receive auto-fix, got %+v", status)
	}
	if len(ghSvc.fixAttempts) != 1 || ghSvc.fixAttempts[0].SessionID != "session-2" {
		t.Fatalf("expected fix attempt to use fallback session-2, got %+v", ghSvc.fixAttempts)
	}
}

func TestHandleTaskPRCIAutomationStopsBeforeEleventhAutoFixRound(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	now := time.Now().UTC()
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		LastSyncedAt: &now,
	}
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		triggerPRSyncAllPRs: []*github.TaskPR{pr},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"}},
		},
		ciPRState: &github.TaskCIPRAutomationState{
			TaskID:            "task-1",
			RepositoryID:      "repo-1",
			PRNumber:          42,
			AutoFixRoundCount: ciAutomationMaxFixRounds,
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle capped auto-fix: %v", err)
	}
	if status := svc.messageQueue.GetStatus(ctx, "session-1"); status.Count != 0 {
		t.Fatalf("expected no 11th auto-fix prompt, got %+v", status)
	}
	if len(ghSvc.fixAttempts) != 0 {
		t.Fatalf("expected no fix attempt after cap, got %+v", ghSvc.fixAttempts)
	}
	if len(ghSvc.ciExhausted) != 1 || ghSvc.ciExhausted[0].LastError == nil || !strings.Contains(*ghSvc.ciExhausted[0].LastError, "10 rounds") {
		t.Fatalf("expected exhausted CI state, got %+v", ghSvc.ciExhausted)
	}
}

func TestHandleTaskPRCIAutomationSkipsAlreadyExhaustedPRBeforeFeedbackFetch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	exhaustedAt := time.Now().UTC()
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		ChecksState:  "failure",
	}
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		ciPRState: &github.TaskCIPRAutomationState{
			TaskID:             "task-1",
			RepositoryID:       "repo-1",
			PRNumber:           42,
			AutoFixRoundCount:  ciAutomationMaxFixRounds,
			AutoFixExhaustedAt: &exhaustedAt,
		},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"}},
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomationWithRefresh(ctx, pr, false); err != nil {
		t.Fatalf("handle exhausted auto-fix: %v", err)
	}
	if ghSvc.prFeedbackCalls != 0 {
		t.Fatalf("expected exhausted PR to skip full feedback fetch, got %d calls", ghSvc.prFeedbackCalls)
	}
	if len(ghSvc.ciExhausted) != 0 {
		t.Fatalf("expected no repeated exhaustion write, got %+v", ghSvc.ciExhausted)
	}
	if status := svc.messageQueue.GetStatus(ctx, "session-1"); status.Count != 0 {
		t.Fatalf("expected no queued prompt for exhausted PR, got %+v", status)
	}
}

func TestHandleTaskPRCIAutomationAutoMergeRunsAfterAutoFixExhaustion(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	now := time.Now().UTC()
	exhaustedAt := now.Add(-time.Minute)
	pr := &github.TaskPR{
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		Owner:          "acme",
		Repo:           "widget",
		PRNumber:       42,
		State:          "open",
		ChecksState:    "success",
		ReviewState:    "approved",
		MergeableState: "clean",
		LastSyncedAt:   &now,
	}
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:           "task-1",
			AutoFixEnabled:   true,
			AutoMergeEnabled: true,
		},
		ciPRState: &github.TaskCIPRAutomationState{
			TaskID:             "task-1",
			RepositoryID:       "repo-1",
			PRNumber:           42,
			AutoFixRoundCount:  ciAutomationMaxFixRounds,
			AutoFixExhaustedAt: &exhaustedAt,
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomationWithRefresh(ctx, pr, false); err != nil {
		t.Fatalf("handle exhausted auto-merge: %v", err)
	}
	if ghSvc.prFeedbackCalls != 0 {
		t.Fatalf("expected exhausted auto-fix to skip feedback fetch, got %d calls", ghSvc.prFeedbackCalls)
	}
	if ghSvc.mergeCalls != 1 {
		t.Fatalf("expected auto-merge after exhausted auto-fix, got %d calls", ghSvc.mergeCalls)
	}
}

func TestDispatchCIAutomationPromptForPRIdleRoundCapUsesDedicatedError(t *testing.T) {
	ctx := context.Background()
	svc := &Service{}
	session := &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateIdle,
	}
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
	}

	_, err := svc.dispatchCIAutomationPromptForPR(ctx, session, pr, "Fix the PR", "signature", false)
	if !errors.Is(err, errCIAutoFixRoundCapReached) {
		t.Fatalf("expected auto-fix round cap error, got %v", err)
	}
	if errors.Is(err, messagequeue.ErrEntryNotFound) {
		t.Fatalf("round cap should not reuse queue entry-not-found sentinel")
	}
}

func TestHandleTaskPRCIAutomationAtRoundCapReplacesPendingAutoFix(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	now := time.Now().UTC()
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		LastSyncedAt: &now,
	}
	_, _, err := svc.messageQueue.QueueMessageWithCoalesceKey(ctx, "session-1", "task-1", "@ci-auto-fix\n\nold feedback", "", messagequeue.QueuedByWorkflow, false, nil, ciAutomationMessageMetadataForPR(pr, "old"), ciAutomationCoalesceKey(pr), true)
	if err != nil {
		t.Fatalf("seed pending auto-fix: %v", err)
	}
	previous := ciAutomationCurrentCheckpoint(&github.PRFeedback{
		Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"}},
	})
	previousJSON, previousSignature := encodeCIAutomationCheckpoint(previous)
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		triggerPRSyncAllPRs: []*github.TaskPR{pr},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{
				{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"},
				{Name: "lint", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/lint"},
			},
		},
		ciPRState: &github.TaskCIPRAutomationState{
			TaskID:                "task-1",
			RepositoryID:          "repo-1",
			PRNumber:              42,
			LastFixSignature:      previousSignature,
			LastFixCheckpointJSON: previousJSON,
			LastFixEnqueuedAt:     &now,
			AutoFixRoundCount:     ciAutomationMaxFixRounds,
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle capped replacement: %v", err)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 || !strings.Contains(status.Entries[0].Content, "lint") || strings.Contains(status.Entries[0].Content, "old feedback") {
		t.Fatalf("expected pending auto-fix replacement with latest feedback, got %+v", status)
	}
	if len(ghSvc.fixAttempts) != 1 || ghSvc.fixAttempts[0].IncrementRound {
		t.Fatalf("expected replacement checkpoint without another round, got %+v", ghSvc.fixAttempts)
	}
	if len(ghSvc.ciExhausted) != 0 {
		t.Fatalf("expected pending round replacement not exhaustion, got %+v", ghSvc.ciExhausted)
	}
}

func TestHandleTaskPRCIAutomationAtRoundCapReplacesPendingAutoFixForWaitingSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo, promptDone: make(chan struct{})}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentExecutionID = "exec-1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session execution: %v", err)
	}
	now := time.Now().UTC()
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		LastSyncedAt: &now,
	}
	_, _, err = svc.messageQueue.QueueMessageWithCoalesceKey(ctx, "session-1", "task-1", "@ci-auto-fix\n\nold feedback", "", messagequeue.QueuedByWorkflow, false, nil, ciAutomationMessageMetadataForPR(pr, "old"), ciAutomationCoalesceKey(pr), true)
	if err != nil {
		t.Fatalf("seed pending auto-fix: %v", err)
	}
	previous := ciAutomationCurrentCheckpoint(&github.PRFeedback{
		Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"}},
	})
	previousJSON, previousSignature := encodeCIAutomationCheckpoint(previous)
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		triggerPRSyncAllPRs: []*github.TaskPR{pr},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{
				{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"},
				{Name: "lint", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/lint"},
			},
		},
		ciPRState: &github.TaskCIPRAutomationState{
			TaskID:                "task-1",
			RepositoryID:          "repo-1",
			PRNumber:              42,
			LastFixSignature:      previousSignature,
			LastFixCheckpointJSON: previousJSON,
			LastFixEnqueuedAt:     &now,
			AutoFixRoundCount:     ciAutomationMaxFixRounds,
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle capped waiting replacement: %v", err)
	}
	select {
	case <-agentMgr.promptDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replaced auto-fix prompt dispatch")
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 0 {
		t.Fatalf("expected replaced auto-fix prompt to drain immediately, got %+v", status)
	}
	if len(agentMgr.capturedPrompts) != 1 || !strings.Contains(agentMgr.capturedPrompts[0], "lint") || strings.Contains(agentMgr.capturedPrompts[0], "old feedback") {
		t.Fatalf("expected latest replaced auto-fix prompt to dispatch, got %+v", agentMgr.capturedPrompts)
	}
	if len(ghSvc.fixAttempts) != 1 || ghSvc.fixAttempts[0].IncrementRound {
		t.Fatalf("expected replacement checkpoint without another round, got %+v", ghSvc.fixAttempts)
	}
	if len(ghSvc.ciExhausted) != 0 {
		t.Fatalf("expected pending round replacement not exhaustion, got %+v", ghSvc.ciExhausted)
	}
}

func TestHandleTaskPRCIAutomationReplacesPendingAutoFixBeforeDirectPrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo, promptDone: make(chan struct{})}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentExecutionID = "exec-1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session execution: %v", err)
	}
	now := time.Now().UTC()
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		LastSyncedAt: &now,
	}
	_, _, err = svc.messageQueue.QueueMessageWithCoalesceKey(ctx, "session-1", "task-1", "@ci-auto-fix\n\nold feedback", "", messagequeue.QueuedByWorkflow, false, nil, ciAutomationMessageMetadataForPR(pr, "old"), ciAutomationCoalesceKey(pr), true)
	if err != nil {
		t.Fatalf("seed pending auto-fix: %v", err)
	}
	previous := ciAutomationCurrentCheckpoint(&github.PRFeedback{
		Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"}},
	})
	previousJSON, previousSignature := encodeCIAutomationCheckpoint(previous)
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
		},
		triggerPRSyncAllPRs: []*github.TaskPR{pr},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{
				{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"},
				{Name: "lint", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/lint"},
			},
		},
		ciPRState: &github.TaskCIPRAutomationState{
			TaskID:                "task-1",
			RepositoryID:          "repo-1",
			PRNumber:              42,
			LastFixSignature:      previousSignature,
			LastFixCheckpointJSON: previousJSON,
			LastFixEnqueuedAt:     &now,
			AutoFixRoundCount:     3,
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle waiting replacement: %v", err)
	}
	select {
	case <-agentMgr.promptDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replaced auto-fix prompt dispatch")
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 0 {
		t.Fatalf("expected replaced auto-fix prompt to drain before direct prompt, got %+v", status)
	}
	if len(agentMgr.capturedPrompts) != 1 || !strings.Contains(agentMgr.capturedPrompts[0], "lint") || strings.Contains(agentMgr.capturedPrompts[0], "old feedback") {
		t.Fatalf("expected latest replaced auto-fix prompt to dispatch, got %+v", agentMgr.capturedPrompts)
	}
	if len(ghSvc.fixAttempts) != 1 || ghSvc.fixAttempts[0].IncrementRound {
		t.Fatalf("expected replacement checkpoint without consuming another round, got %+v", ghSvc.fixAttempts)
	}
}

func TestCIAutomationFindMatchingPRRequiresRepositoryIDWhenPresent(t *testing.T) {
	target := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-back",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
	}
	wrongRepo := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-front",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
	}
	matchingRepo := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-back",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
	}

	got := ciAutomationFindMatchingPR([]*github.TaskPR{wrongRepo, matchingRepo}, target)
	if got != matchingRepo {
		t.Fatalf("expected repository_id match, got %+v", got)
	}

	got = ciAutomationFindMatchingPR([]*github.TaskPR{wrongRepo}, target)
	if got != nil {
		t.Fatalf("expected no owner/repo fallback when target repository_id is set, got %+v", got)
	}
}

func TestDispatchCIAutomationPromptForPRRunningRoundCapUsesDedicatedError(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	session, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
	}

	_, err = svc.dispatchCIAutomationPromptForPR(ctx, session, pr, "Fix the PR", "signature", false)
	if !errors.Is(err, errCIAutoFixRoundCapReached) {
		t.Fatalf("expected auto-fix round cap error, got %v", err)
	}
	if errors.Is(err, messagequeue.ErrEntryNotFound) {
		t.Fatalf("round cap should not expose queue entry-not-found sentinel")
	}
}

func TestDispatchCIAutomationPromptForPRDoesNotRecordUserMessageWhenQueueFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageQueue = nil
	messageCreator := &mockMessageCreator{}
	svc.messageCreator = messageCreator
	pr := &github.TaskPR{TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "widget", PRNumber: 42}

	_, err := svc.dispatchCIAutomationPromptForPR(ctx, &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateRunning,
	}, pr, "Fix the PR", "signature", true)
	if err == nil {
		t.Fatal("expected queue failure")
	}
	if len(messageCreator.userMessages) != 0 {
		t.Fatalf("expected no visible CI automation user message on queue failure, got %d", len(messageCreator.userMessages))
	}
}

func TestDispatchCIAutomationPromptForPRQueuesWhenRunningUserMessageCannotBeRecorded(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageQueue = messagequeue.NewServiceMemory(testLogger())
	messageCreator := &mockMessageCreator{userMessageErr: errors.New("message db unavailable")}
	svc.messageCreator = messageCreator
	session, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	pr := &github.TaskPR{TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "widget", PRNumber: 42}

	result, err := svc.dispatchCIAutomationPromptForPR(ctx, session, pr, "Fix the PR", "signature", true)
	if err != nil {
		t.Fatalf("expected queued CI automation prompt, got %v", err)
	}
	if !result.consumesRound() {
		t.Fatalf("expected new queued prompt to consume a round, got %+v", result)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 {
		t.Fatalf("expected queued prompt when user message cannot be recorded yet, got %d", status.Count)
	}
	if status.Entries[0].Metadata[metaKeyUserMessageRecorded] == true {
		t.Fatalf("expected queued prompt to retry user-message recording on drain, got %+v", status.Entries[0].Metadata)
	}
}

func TestDispatchCIAutomationPromptForPRRecordsUserMessageBeforeDirectPrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.agentManager = agentMgr
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	messageCreator := &mockMessageCreator{}
	svc.messageCreator = messageCreator
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentExecutionID = "exec-1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session execution: %v", err)
	}
	pr := &github.TaskPR{TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "widget", PRNumber: 42}

	result, err := svc.dispatchCIAutomationPromptForPR(ctx, session, pr, "Fix the PR", "signature", true)
	if err != nil {
		t.Fatalf("dispatch direct prompt: %v", err)
	}
	if !result.consumesRound() {
		t.Fatalf("expected direct prompt to consume a round, got %+v", result)
	}
	if len(messageCreator.userMessages) != 1 {
		t.Fatalf("expected one visible CI automation user message, got %d", len(messageCreator.userMessages))
	}
	if len(agentMgr.capturedPrompts) != 1 {
		t.Fatalf("expected one direct prompt call, got %d", len(agentMgr.capturedPrompts))
	}
	if len(agentMgr.capturedPromptCalls) != 1 || !agentMgr.capturedPromptCalls[0].DispatchOnly {
		t.Fatalf("expected CI automation direct prompt to dispatch only, got %+v", agentMgr.capturedPromptCalls)
	}
	if messageCreator.userMessages[0].metadata["origin"] != ciAutomationOrigin {
		t.Fatalf("expected CI automation user message metadata, got %+v", messageCreator.userMessages[0].metadata)
	}
}

func TestDispatchCIAutomationPromptForPRRecordsUserMessageBeforeDirectPromptFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptErr:              errors.New("agent rejected prompt"),
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.agentManager = agentMgr
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	messageCreator := &mockMessageCreator{}
	svc.messageCreator = messageCreator
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentExecutionID = "exec-1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session execution: %v", err)
	}
	pr := &github.TaskPR{TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "widget", PRNumber: 42}

	_, err = svc.dispatchCIAutomationPromptForPR(ctx, session, pr, "Fix the PR", "signature", true)
	if err == nil || !strings.Contains(err.Error(), "agent rejected prompt") {
		t.Fatalf("expected prompt failure, got %v", err)
	}
	if len(messageCreator.userMessages) != 1 {
		t.Fatalf("expected visible CI automation user message before prompt failure, got %d", len(messageCreator.userMessages))
	}
}

func TestCIAutomationMergeSignatureIncludesPRContentVersion(t *testing.T) {
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		PRNumber:     42,
		HeadBranch:   "feature",
		ChecksState:  "success",
		ReviewState:  "approved",
		UpdatedAt:    time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC),
	}
	before := ciAutomationMergeSignature(pr)
	pr.UpdatedAt = pr.UpdatedAt.Add(time.Minute)
	pr.Additions++
	after := ciAutomationMergeSignature(pr)
	if before == after {
		t.Fatal("expected PR content/version change to alter merge signature")
	}
}

func TestCIAutomationMergeSignatureIgnoresVolatileUpdatedAt(t *testing.T) {
	pr := &github.TaskPR{
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		PRNumber:       42,
		HeadBranch:     "feature",
		ChecksState:    "success",
		ReviewState:    "approved",
		MergeableState: "clean",
		UpdatedAt:      time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC),
	}
	before := ciAutomationMergeSignature(pr)
	pr.UpdatedAt = pr.UpdatedAt.Add(time.Minute)
	if after := ciAutomationMergeSignature(pr); after != before {
		t.Fatalf("expected updated_at-only change not to alter signature: before=%s after=%s", before, after)
	}
	pr.ReviewCount++
	if after := ciAutomationMergeSignature(pr); after == before {
		t.Fatal("expected semantic readiness change to alter signature")
	}
}

func TestHandleTaskPRCIAutomationRecordsErrorWhenNoPromptableSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateCompleted)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR",
		},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"}},
		},
	}
	svc.SetGitHubService(ghSvc)

	err := svc.handleTaskPRCIAutomation(ctx, &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		ChecksState:  "failure",
	})
	if err != nil {
		t.Fatalf("handle auto-fix: %v", err)
	}
	if len(ghSvc.ciErrors) != 1 || ghSvc.ciErrors[0].LastError == nil || *ghSvc.ciErrors[0].LastError != "no promptable task session for CI auto-fix" {
		t.Fatalf("expected no-session CI automation error, got %+v", ghSvc.ciErrors)
	}
	if len(ghSvc.fixAttempts) != 0 {
		t.Fatalf("expected no fix attempt without promptable session, got %+v", ghSvc.fixAttempts)
	}
}

func TestHandleTaskPRCIAutomationMarksExhaustedWithoutPromptableSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateCompleted)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR",
		},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"}},
		},
		ciPRState: &github.TaskCIPRAutomationState{
			TaskID:            "task-1",
			RepositoryID:      "repo-1",
			PRNumber:          42,
			AutoFixRoundCount: ciAutomationMaxFixRounds,
		},
	}
	svc.SetGitHubService(ghSvc)

	err := svc.handleTaskPRCIAutomation(ctx, &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		ChecksState:  "failure",
	})
	if err != nil {
		t.Fatalf("handle auto-fix: %v", err)
	}
	if len(ghSvc.ciExhausted) != 1 {
		t.Fatalf("expected exhausted CI state without promptable session, got %+v", ghSvc.ciExhausted)
	}
	if len(ghSvc.ciErrors) != 0 {
		t.Fatalf("expected cap exhaustion instead of no-session error, got %+v", ghSvc.ciErrors)
	}
	if len(ghSvc.fixAttempts) != 0 {
		t.Fatalf("expected no fix attempt without promptable session, got %+v", ghSvc.fixAttempts)
	}
}

func TestHandleTaskPRCIAutomationMergesWhenStateReadFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:           "task-1",
			AutoMergeEnabled: true,
		},
		ciPRStateErr: errors.New("sqlite busy"),
	}
	svc.SetGitHubService(ghSvc)

	now := time.Now().UTC()
	pr := &github.TaskPR{
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		Owner:          "acme",
		Repo:           "widget",
		PRNumber:       42,
		State:          "open",
		ChecksState:    "success",
		ReviewState:    "approved",
		MergeableState: "clean",
		LastSyncedAt:   &now,
	}
	ghSvc.triggerPRSyncAllPRs = []*github.TaskPR{pr}

	err := svc.handleTaskPRCIAutomation(ctx, pr)
	if err != nil {
		t.Fatalf("handle auto-merge: %v", err)
	}
	if ghSvc.mergeCalls != 1 {
		t.Fatalf("expected merge to proceed when dedupe state is unavailable, got %d", ghSvc.mergeCalls)
	}
}

func TestHandleTaskPRCIAutomationRetriesMergeAfterTransientFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:           "task-1",
			AutoMergeEnabled: true,
		},
		mergeErr: errors.New("github unavailable"),
	}
	svc.SetGitHubService(ghSvc)
	pr := &github.TaskPR{
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		Owner:          "acme",
		Repo:           "widget",
		PRNumber:       42,
		State:          "open",
		ChecksState:    "success",
		ReviewState:    "approved",
		MergeableState: "clean",
	}
	now := time.Now().UTC()
	pr.LastSyncedAt = &now
	ghSvc.triggerPRSyncAllPRs = []*github.TaskPR{pr}

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle failed auto-merge: %v", err)
	}
	if ghSvc.mergeCalls != 1 {
		t.Fatalf("expected one merge call, got %d", ghSvc.mergeCalls)
	}
	if len(ghSvc.mergeAttempts) != 0 {
		t.Fatalf("expected failed merge not to record dedupe signature, got %+v", ghSvc.mergeAttempts)
	}

	ghSvc.mergeErr = nil
	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("retry auto-merge: %v", err)
	}
	if ghSvc.mergeCalls != 2 || len(ghSvc.mergeAttempts) != 1 {
		t.Fatalf("expected retry to merge and record one attempt, calls=%d attempts=%d", ghSvc.mergeCalls, len(ghSvc.mergeAttempts))
	}
}

func TestHandlePRFeedbackStartsAutomationForMatchingPR(t *testing.T) {
	ctx := context.Background()
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	started := make(chan struct{})
	block := make(chan struct{})
	ghSvc := &mockGitHubService{
		taskPRs: []*github.TaskPR{
			{TaskID: "task-1", RepositoryID: "repo-front", Owner: "acme", Repo: "front", PRNumber: 1},
			{TaskID: "task-1", RepositoryID: "repo-back", Owner: "acme", Repo: "back", PRNumber: 2},
		},
		ciOptionsResp:    &github.TaskCIOptionsResponse{TaskID: "task-1"},
		ciOptionsStarted: started,
		ciOptionsBlock:   block,
	}
	svc.SetGitHubService(ghSvc)

	err := svc.handlePRFeedback(ctx, &bus.Event{Data: &github.PRFeedbackEvent{
		TaskID:   "task-1",
		Owner:    "acme",
		Repo:     "back",
		PRNumber: 2,
	}})
	if err != nil {
		t.Fatalf("handle PR feedback: %v", err)
	}
	<-started
	if ghSvc.getTaskPRCalls != 0 || ghSvc.exactTaskPRCalls != 1 {
		t.Fatalf("expected exact PR lookup only, GetTaskPR=%d exact=%d", ghSvc.getTaskPRCalls, ghSvc.exactTaskPRCalls)
	}
	if ghSvc.lastExactPRLookup.Owner != "acme" || ghSvc.lastExactPRLookup.Repo != "back" || ghSvc.lastExactPRLookup.PRNumber != 2 {
		t.Fatalf("unexpected exact lookup: %+v", ghSvc.lastExactPRLookup)
	}
	if _, loaded := svc.ciAutomationInFlight.Load("task-1|repo-back|2"); !loaded {
		t.Fatal("expected automation to run for matching repo-back PR")
	}
	if _, loaded := svc.ciAutomationInFlight.Load("task-1|repo-front|1"); loaded {
		t.Fatal("unexpected automation for non-matching repo-front PR")
	}
	close(block)
	waitForCIAutomationIdle(t, svc, "task-1|repo-back|2", 200*time.Millisecond)
}

func TestHandleTaskCIOptionsUpdatedStartsAutomationForTaskPRs(t *testing.T) {
	ctx := context.Background()
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	started := make(chan struct{})
	block := make(chan struct{})
	ghSvc := &mockGitHubService{
		taskPRs: []*github.TaskPR{
			{TaskID: "task-1", RepositoryID: "repo-front", Owner: "acme", Repo: "front", PRNumber: 1},
			{TaskID: "task-1", RepositoryID: "repo-back", Owner: "acme", Repo: "back", PRNumber: 2},
		},
		ciOptionsResp:    &github.TaskCIOptionsResponse{TaskID: "task-1"},
		ciOptionsStarted: started,
		ciOptionsBlock:   block,
	}
	svc.SetGitHubService(ghSvc)

	err := svc.handleTaskCIOptionsUpdated(ctx, &bus.Event{Data: &github.TaskCIOptionsResponse{
		TaskID:         "task-1",
		AutoFixEnabled: true,
	}})
	if err != nil {
		t.Fatalf("handle CI options updated: %v", err)
	}
	<-started
	if _, loaded := svc.ciAutomationInFlight.Load("task-1|repo-front|1"); !loaded {
		t.Fatal("expected automation to run for repo-front PR")
	}
	if _, loaded := svc.ciAutomationInFlight.Load("task-1|repo-back|2"); !loaded {
		t.Fatal("expected automation to run for repo-back PR")
	}
	close(block)
	waitForCIAutomationIdle(t, svc, "task-1|repo-front|1", 200*time.Millisecond)
	waitForCIAutomationIdle(t, svc, "task-1|repo-back|2", 200*time.Millisecond)
	if ghSvc.triggerPRSyncAllCalls != 1 {
		t.Fatalf("expected one task-wide sync from option save, got %d", ghSvc.triggerPRSyncAllCalls)
	}
}

func TestHandleTaskCIOptionsUpdatedIgnoresStateRefreshEvents(t *testing.T) {
	ctx := context.Background()
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{TaskID: "task-1"},
	}
	svc.SetGitHubService(ghSvc)

	err := svc.handleTaskCIOptionsUpdated(ctx, &bus.Event{
		Source: ciAutomationStateEventSource,
		Data: &github.TaskCIOptionsResponse{
			TaskID:         "task-1",
			AutoFixEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("handle CI options state refresh: %v", err)
	}
	if ghSvc.triggerPRSyncAllCalls != 0 {
		t.Fatalf("state refresh should not start automation sync, got %d calls", ghSvc.triggerPRSyncAllCalls)
	}
}

func TestHandleTaskCIOptionsUpdatedRecordsSyncFailureForLinkedPRs(t *testing.T) {
	ctx := context.Background()
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mockGitHubService{
		taskPRs: []*github.TaskPR{{
			TaskID:       "task-1",
			RepositoryID: "repo-1",
			Owner:        "acme",
			Repo:         "widget",
			PRNumber:     42,
		}},
		triggerPRSyncAllErr: errors.New("gh unavailable"),
	}
	svc.SetGitHubService(ghSvc)

	err := svc.handleTaskCIOptionsUpdated(ctx, &bus.Event{Data: &github.TaskCIOptionsResponse{
		TaskID:           "task-1",
		AutoMergeEnabled: true,
	}})
	if err != nil {
		t.Fatalf("handle CI options updated: %v", err)
	}
	if len(ghSvc.ciErrors) != 1 || ghSvc.ciErrors[0].LastError == nil || !strings.Contains(*ghSvc.ciErrors[0].LastError, "sync PR status: gh unavailable") {
		t.Fatalf("expected sync failure to be recorded on linked PR, got %+v", ghSvc.ciErrors)
	}
}

func TestHandleTaskCIOptionsUpdatedStartsAutomationForPartialSyncResults(t *testing.T) {
	ctx := context.Background()
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	started := make(chan struct{})
	block := make(chan struct{})
	ghSvc := &mockGitHubService{
		taskPRs: []*github.TaskPR{{
			TaskID:       "task-1",
			RepositoryID: "repo-1",
			Owner:        "acme",
			Repo:         "widget",
			PRNumber:     42,
		}},
		triggerPRSyncAllPRs: []*github.TaskPR{{
			TaskID:       "task-1",
			RepositoryID: "repo-1",
			Owner:        "acme",
			Repo:         "widget",
			PRNumber:     42,
		}},
		triggerPRSyncAllErr: &github.PartialPRSyncError{Err: errors.New("sibling repo unavailable")},
		ciOptionsResp:       &github.TaskCIOptionsResponse{TaskID: "task-1"},
		ciOptionsStarted:    started,
		ciOptionsBlock:      block,
	}
	svc.SetGitHubService(ghSvc)

	err := svc.handleTaskCIOptionsUpdated(ctx, &bus.Event{Data: &github.TaskCIOptionsResponse{
		TaskID:           "task-1",
		AutoMergeEnabled: true,
	}})
	if err != nil {
		t.Fatalf("handle CI options updated: %v", err)
	}
	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for CI automation to start")
	}
	if _, loaded := svc.ciAutomationInFlight.Load("task-1|repo-1|42"); !loaded {
		t.Fatal("expected automation to run for partial sync result")
	}
	if len(ghSvc.ciErrors) != 0 {
		t.Fatalf("expected synced PR not to receive sibling sync error, got %+v", ghSvc.ciErrors)
	}
	close(block)
	waitForCIAutomationIdle(t, svc, "task-1|repo-1|42", 200*time.Millisecond)
}

func TestHandleTaskCIOptionsUpdatedRecordsPartialSyncFailureOnlyForUnsyncedPRs(t *testing.T) {
	ctx := context.Background()
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	synced := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-front",
		Owner:        "acme",
		Repo:         "front",
		PRNumber:     1,
	}
	unsynced := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-back",
		Owner:        "acme",
		Repo:         "back",
		PRNumber:     2,
	}
	ghSvc := &mockGitHubService{
		taskPRs:             []*github.TaskPR{synced, unsynced},
		triggerPRSyncAllPRs: []*github.TaskPR{synced},
		triggerPRSyncAllErr: &github.PartialPRSyncError{Err: errors.New("back repo unavailable")},
		ciOptionsResp:       &github.TaskCIOptionsResponse{TaskID: "task-1"},
	}
	svc.SetGitHubService(ghSvc)

	err := svc.handleTaskCIOptionsUpdated(ctx, &bus.Event{Data: &github.TaskCIOptionsResponse{
		TaskID:           "task-1",
		AutoMergeEnabled: true,
	}})
	if err != nil {
		t.Fatalf("handle CI options updated: %v", err)
	}
	waitForCIAutomationIdle(t, svc, "task-1|repo-front|1", 200*time.Millisecond)
	if len(ghSvc.ciErrors) != 1 {
		t.Fatalf("expected one sync error for unsynced PR, got %+v", ghSvc.ciErrors)
	}
	got := ghSvc.ciErrors[0]
	if got.RepositoryID != "repo-back" || got.PRNumber != 2 {
		t.Fatalf("expected sync error on repo-back#2, got %+v", got)
	}
	if got.LastError == nil || !strings.Contains(*got.LastError, "back repo unavailable") {
		t.Fatalf("expected sibling sync error message, got %+v", got.LastError)
	}
}

func TestStartTaskPRCIAutomationSkipsDuplicateInFlightPR(t *testing.T) {
	ctx := context.Background()
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	started := make(chan struct{})
	block := make(chan struct{})
	ghSvc := &mockGitHubService{
		ciOptionsResp:    &github.TaskCIOptionsResponse{TaskID: "task-1"},
		ciOptionsStarted: started,
		ciOptionsBlock:   block,
	}
	svc.SetGitHubService(ghSvc)
	pr := &github.TaskPR{TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42}

	svc.startTaskPRCIAutomation(ctx, pr)
	<-started
	svc.startTaskPRCIAutomation(ctx, pr)
	waitForCIOptionsCalls(t, ghSvc, 1, 200*time.Millisecond)

	close(block)
	waitForCIAutomationIdle(t, svc, "task-1|repo-1|42", 200*time.Millisecond)
	svc.startTaskPRCIAutomation(ctx, pr)
	waitForCIOptionsCalls(t, ghSvc, 2, 200*time.Millisecond)
}

func waitForCIOptionsCalls(t *testing.T, ghSvc *mockGitHubService, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		ghSvc.mu.Lock()
		got := ghSvc.ciOptionsCalls
		ghSvc.mu.Unlock()
		if got == want {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("ciOptionsCalls=%d, want %d", got, want)
		case <-ticker.C:
		}
	}
}

func waitForCIAutomationIdle(t *testing.T, svc *Service, key string, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if _, loaded := svc.ciAutomationInFlight.Load(key); !loaded {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("CI automation key %q remained in flight", key)
		case <-ticker.C:
		}
	}
}

func ptrString(value string) *string {
	return &value
}
