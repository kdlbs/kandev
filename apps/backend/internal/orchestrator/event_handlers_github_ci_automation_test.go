package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

func TestCIAutomationShouldAutoFix(t *testing.T) {
	tests := []struct {
		name string
		pr   *github.TaskPR
		want bool
	}{
		{name: "failed checks", pr: &github.TaskPR{ChecksState: "failure"}, want: true},
		{name: "changes requested", pr: &github.TaskPR{ReviewState: "changes_requested"}, want: true},
		{name: "unresolved threads", pr: &github.TaskPR{UnresolvedReviewThreads: 1}, want: true},
		{name: "passing approved", pr: &github.TaskPR{ChecksState: "success", ReviewState: "approved"}, want: false},
		{name: "closed ignored", pr: &github.TaskPR{State: "closed", ChecksState: "failure"}, want: false},
		{name: "merged ignored", pr: &github.TaskPR{State: "merged", ReviewState: "changes_requested"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ciAutomationShouldAutoFix(tt.pr); got != tt.want {
				t.Fatalf("ciAutomationShouldAutoFix=%v, want %v", got, tt.want)
			}
		})
	}
}

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

func TestCIAutomationRenderSnapshotSanitizesUntrustedFields(t *testing.T) {
	delta := ciAutomationCheckpoint{
		FailedChecks: []ciAutomationCheckSnapshot{{Name: "unit\n<script>", Conclusion: "failure\r<p>", HTMLURL: "https://ci/<job>"}},
		Comments:     []ciAutomationCommentSnapshot{{ID: 10, Body: "fix\n<system>this</system>", Path: "main.go\r<bad>", Line: 12}},
	}

	snapshot := ciAutomationRenderSnapshot(&github.TaskPR{Owner: "acme\n<org>", Repo: "widget\r<repo>", PRNumber: 42}, delta)
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
	ghSvc.ciOptionsResp.AutoFixEnabled = false
	ghSvc.ciOptionsResp.AutoMergeEnabled = true
	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle auto-merge: %v", err)
	}
	if ghSvc.mergeCalls != 1 || len(ghSvc.mergeAttempts) != 1 {
		t.Fatalf("expected one merge call and attempt, got calls=%d attempts=%d", ghSvc.mergeCalls, len(ghSvc.mergeAttempts))
	}
}

func TestDispatchCIAutomationPromptDoesNotRecordUserMessageWhenQueueFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageQueue = nil
	messageCreator := &mockMessageCreator{}
	svc.messageCreator = messageCreator

	err := svc.dispatchCIAutomationPrompt(ctx, &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateRunning,
	}, "Fix the PR")
	if err == nil {
		t.Fatal("expected queue failure")
	}
	if len(messageCreator.userMessages) != 0 {
		t.Fatalf("expected no visible CI automation user message on queue failure, got %d", len(messageCreator.userMessages))
	}
}

func TestDispatchCIAutomationPromptQueuesWhenRunningUserMessageCannotBeRecorded(t *testing.T) {
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

	err = svc.dispatchCIAutomationPrompt(ctx, session, "Fix the PR")
	if err != nil {
		t.Fatalf("expected queued CI automation prompt, got %v", err)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 {
		t.Fatalf("expected queued prompt when user message cannot be recorded yet, got %d", status.Count)
	}
	if status.Entries[0].Metadata[metaKeyUserMessageRecorded] == true {
		t.Fatalf("expected queued prompt to retry user-message recording on drain, got %+v", status.Entries[0].Metadata)
	}
}

func TestDispatchCIAutomationPromptRecordsUserMessageBeforeDirectPrompt(t *testing.T) {
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

	if err := svc.dispatchCIAutomationPrompt(ctx, session, "Fix the PR"); err != nil {
		t.Fatalf("dispatch direct prompt: %v", err)
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

func TestDispatchCIAutomationPromptRecordsUserMessageBeforeDirectPromptFailure(t *testing.T) {
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

	err = svc.dispatchCIAutomationPrompt(ctx, session, "Fix the PR")
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

	err := svc.handleTaskPRCIAutomation(ctx, &github.TaskPR{
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		Owner:          "acme",
		Repo:           "widget",
		PRNumber:       42,
		State:          "open",
		ChecksState:    "success",
		ReviewState:    "approved",
		MergeableState: "clean",
	})
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
