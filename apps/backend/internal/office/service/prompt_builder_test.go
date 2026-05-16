package service_test

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
)

func TestBuildPrompt_TaskAssigned(t *testing.T) {
	pc := &service.PromptContext{
		Reason:          service.RunReasonTaskAssigned,
		TaskIdentifier:  "KAN-3",
		TaskTitle:       "Implement login page",
		ProjectName:     "Frontend",
		TaskDescription: "Build the login form with OAuth.",
		TaskPriority:    "medium",
	}
	prompt := service.BuildPrompt(pc)

	checks := []string{
		"[KAN-3]",
		"Implement login page",
		"Project: Frontend",
		"Description: Build the login form",
		"Priority: medium",
		"start working",
	}
	for _, c := range checks {
		if !strings.Contains(prompt, c) {
			t.Errorf("prompt missing %q:\n%s", c, prompt)
		}
	}
}

func TestBuildPrompt_TaskComment(t *testing.T) {
	pc := &service.PromptContext{
		Reason:            service.RunReasonTaskComment,
		TaskIdentifier:    "KAN-5",
		TaskTitle:         "Fix bug",
		CommentBody:       "Please add a test for the edge case.",
		CommentAuthor:     "Alice",
		CommentAuthorType: "user",
	}
	prompt := service.BuildPrompt(pc)

	if !strings.Contains(prompt, "New comment") {
		t.Errorf("missing 'New comment' in prompt:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Alice (user)") {
		t.Errorf("missing author info:\n%s", prompt)
	}
	if !strings.Contains(prompt, "edge case") {
		t.Errorf("missing comment body:\n%s", prompt)
	}
}

func TestBuildPrompt_BlockersResolved(t *testing.T) {
	pc := &service.PromptContext{
		Reason:                service.RunReasonTaskBlockersResolved,
		TaskIdentifier:        "KAN-10",
		TaskTitle:             "Build API",
		ResolvedBlockerTitles: []string{"Write spec", "Design schema"},
	}
	prompt := service.BuildPrompt(pc)

	if !strings.Contains(prompt, "All blockers") {
		t.Errorf("missing 'All blockers':\n%s", prompt)
	}
	if !strings.Contains(prompt, "Write spec, Design schema") {
		t.Errorf("missing blocker titles:\n%s", prompt)
	}
	if !strings.Contains(prompt, "proceed") {
		t.Errorf("missing 'proceed':\n%s", prompt)
	}
}

func TestBuildPrompt_ApprovalResolved(t *testing.T) {
	pc := &service.PromptContext{
		Reason:         service.RunReasonApprovalResolved,
		ApprovalType:   "task_review",
		ApprovalStatus: "rejected",
		ApprovalNote:   "Needs more tests.",
	}
	prompt := service.BuildPrompt(pc)

	if !strings.Contains(prompt, "rejected") {
		t.Errorf("missing 'rejected':\n%s", prompt)
	}
	if !strings.Contains(prompt, "Needs more tests") {
		t.Errorf("missing decision note:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Address the feedback") {
		t.Errorf("missing action for rejected review:\n%s", prompt)
	}
}

func TestBuildPrompt_Heartbeat(t *testing.T) {
	pc := &service.PromptContext{
		Reason:          service.RunReasonHeartbeat,
		AgentsIdle:      2,
		AgentsWorking:   3,
		AgentsPaused:    1,
		TasksInProgress: 5,
		TasksCompleted:  10,
		TasksPending:    3,
		BudgetUsedPct:   45,
		RecentErrors:    []string{"session timeout"},
	}
	prompt := service.BuildPrompt(pc)

	checks := []string{
		"2 idle",
		"3 working",
		"1 paused",
		"5 in progress",
		"45%",
		"session timeout",
		"take action",
	}
	for _, c := range checks {
		if !strings.Contains(prompt, c) {
			t.Errorf("heartbeat prompt missing %q:\n%s", c, prompt)
		}
	}
}

func TestBuildPrompt_ChildrenCompleted_WithSummaries(t *testing.T) {
	pc := &service.PromptContext{
		Reason:         service.RunReasonTaskChildrenCompleted,
		TaskIdentifier: "KAN-1",
		TaskTitle:      "Add OAuth2",
		ChildSummaries: []service.ChildSummaryPrompt{
			{Identifier: "KAN-2", Title: "Auth service", State: "COMPLETED", LastComment: "Implemented JWT generation"},
			{Identifier: "KAN-3", Title: "API gateway", State: "COMPLETED", LastComment: "Added rate limiting"},
			{Identifier: "KAN-4", Title: "QA", State: "CANCELLED", LastComment: ""},
		},
	}
	prompt := service.BuildPrompt(pc)

	checks := []string{
		"All child tasks",
		"[KAN-1]",
		"KAN-2 (Auth service) [COMPLETED]",
		"JWT generation",
		"KAN-3 (API gateway) [COMPLETED]",
		"KAN-4 (QA) [CANCELLED]",
		"determine next steps",
	}
	for _, c := range checks {
		if !strings.Contains(prompt, c) {
			t.Errorf("children completed prompt missing %q:\n%s", c, prompt)
		}
	}

	// KAN-4 has no comment — should not have a quote.
	if strings.Contains(prompt, `KAN-4 (QA) [CANCELLED] —`) {
		t.Errorf("child with no comment should not have quote separator:\n%s", prompt)
	}
}

func TestBuildPrompt_ChildrenCompleted_Truncated(t *testing.T) {
	pc := &service.PromptContext{
		Reason:                  service.RunReasonTaskChildrenCompleted,
		TaskIdentifier:          "KAN-1",
		TaskTitle:               "Big task",
		ChildSummaries:          []service.ChildSummaryPrompt{{Identifier: "KAN-2", Title: "Child", State: "COMPLETED"}},
		ChildSummariesTruncated: true,
	}
	prompt := service.BuildPrompt(pc)
	if !strings.Contains(prompt, "showing first 20") {
		t.Errorf("truncated prompt should mention limit:\n%s", prompt)
	}
}

func TestBuildPrompt_ChildrenCompleted_NoSummaries(t *testing.T) {
	pc := &service.PromptContext{
		Reason:         service.RunReasonTaskChildrenCompleted,
		TaskIdentifier: "KAN-1",
		TaskTitle:      "Simple task",
	}
	prompt := service.BuildPrompt(pc)
	if !strings.Contains(prompt, "All child tasks") {
		t.Errorf("prompt missing header:\n%s", prompt)
	}
	if strings.Contains(prompt, "Completed children:") {
		t.Errorf("no summaries section when children are empty:\n%s", prompt)
	}
}

func TestBuildPrompt_UnknownReason(t *testing.T) {
	pc := &service.PromptContext{Reason: "custom_reason"}
	prompt := service.BuildPrompt(pc)
	if !strings.Contains(prompt, "custom_reason") {
		t.Errorf("unknown reason prompt should mention the reason:\n%s", prompt)
	}
}

func TestBuildPrompt_TaskIDFallback(t *testing.T) {
	pc := &service.PromptContext{
		Reason:    service.RunReasonTaskAssigned,
		TaskID:    "abc-123",
		TaskTitle: "Some task",
	}
	prompt := service.BuildPrompt(pc)
	if !strings.Contains(prompt, "[abc-123]") {
		t.Errorf("should fallback to task ID when no identifier:\n%s", prompt)
	}
}

func TestBuildAgentPrompt_FreshSession(t *testing.T) {
	svc := newTestService(t)

	// AGENTS.md content is passed in-memory after sibling refs have
	// been rewritten to absolute paths by resolveInstructionsForPrompt.
	// The builder itself no longer appends a resolver footer.
	dir := t.TempDir()
	agentsMD := "# You are the CEO.\nRead `" + dir + "/HEARTBEAT.md` first."
	wakeContext := "You have been assigned task [KAN-1]: Build it."
	res := svc.BuildAgentPrompt(nil, nil, dir, agentsMD, false, wakeContext, "task-1", "")
	prompt := res.Prompt

	if !strings.Contains(prompt, "# You are the CEO.") {
		t.Errorf("prompt should contain AGENTS.md content:\n%s", prompt)
	}
	if !strings.Contains(prompt, dir+"/HEARTBEAT.md") {
		t.Errorf("prompt should reference sibling file by absolute path:\n%s", prompt)
	}
	if strings.Contains(prompt, "./HEARTBEAT.md") {
		t.Errorf("prompt should not contain relative sibling refs:\n%s", prompt)
	}
	if strings.Contains(prompt, "Resolve any relative file references") {
		t.Errorf("prompt should not contain resolver footer:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Build it") {
		t.Errorf("prompt should contain wake context:\n%s", prompt)
	}
	if !strings.Contains(prompt, "---") {
		t.Errorf("prompt should have section separator:\n%s", prompt)
	}
}

func TestBuildAgentPrompt_ResumeSkipsInstructions(t *testing.T) {
	svc := newTestService(t)

	dir := t.TempDir()
	wakeContext := "New comment on your task."
	res := svc.BuildAgentPrompt(nil, nil, dir, "# Instructions", true, wakeContext, "task-1", "")
	prompt := res.Prompt

	if strings.Contains(prompt, "# Instructions") {
		t.Errorf("resume should NOT include AGENTS.md:\n%s", prompt)
	}
	if !strings.Contains(prompt, "New comment") {
		t.Errorf("resume should include wake context:\n%s", prompt)
	}
}

func TestBuildAgentPrompt_NoInstructionsDir(t *testing.T) {
	svc := newTestService(t)

	wakeContext := "Heartbeat check."
	res := svc.BuildAgentPrompt(nil, nil, "", "", false, wakeContext, "task-1", "")
	prompt := res.Prompt

	if !strings.Contains(prompt, "Heartbeat check") {
		t.Errorf("prompt should contain wake context even without instructions dir:\n%s", prompt)
	}
}

func TestBuildAgentPrompt_TasklessRunPrependsContinuationSummary(t *testing.T) {
	svc := newTestService(t)

	summary := "## Active focus\nMonitoring CI flakiness.\n## Open blockers\nNone.\n## Next action\nContinue monitoring.\n"
	wakeContext := "Heartbeat fired at 10:00."
	res := svc.BuildAgentPrompt(nil, nil, "", "# AGENTS", false, wakeContext, "", summary)

	if !strings.Contains(res.Prompt, "Monitoring CI flakiness.") {
		t.Errorf("expected continuation summary prepended to prompt:\n%s", res.Prompt)
	}
	if !strings.Contains(res.Prompt, "Heartbeat fired at 10:00.") {
		t.Errorf("expected wake context retained:\n%s", res.Prompt)
	}
	if res.SummaryInjected == "" {
		t.Errorf("SummaryInjected should not be empty when summary was prepended")
	}
	if !strings.HasPrefix(res.Prompt, "## Active focus") {
		t.Errorf("continuation summary should be the first section:\n%s", res.Prompt)
	}
}

func TestBuildAgentPrompt_TasklessRunWithEmptySummaryDoesNotPrepend(t *testing.T) {
	svc := newTestService(t)

	res := svc.BuildAgentPrompt(nil, nil, "", "# AGENTS", false, "Wake.", "", "")
	if res.SummaryInjected != "" {
		t.Errorf("SummaryInjected should be empty when no summary provided, got %q", res.SummaryInjected)
	}
	if strings.HasPrefix(res.Prompt, "## ") {
		t.Errorf("prompt should not start with a markdown section when no summary:\n%s", res.Prompt)
	}
}

func TestBuildAgentPrompt_TaskBoundRunDoesNotPrependSummary(t *testing.T) {
	svc := newTestService(t)

	summary := "## Active focus\nDoing things.\n"
	res := svc.BuildAgentPrompt(nil, nil, "", "# AGENTS", false, "Wake.", "task-1", summary)
	if res.SummaryInjected != "" {
		t.Errorf("SummaryInjected should be empty for task-bound runs, got %q", res.SummaryInjected)
	}
	if strings.Contains(res.Prompt, "Doing things.") {
		t.Errorf("task-bound run should not see continuation summary:\n%s", res.Prompt)
	}
}

func TestBuildAgentPrompt_TasklessRunSummarySlicedAt1500Chars(t *testing.T) {
	svc := newTestService(t)

	huge := strings.Repeat("x", 2000)
	res := svc.BuildAgentPrompt(nil, nil, "", "", false, "", "", huge)
	if len(res.SummaryInjected) > 1500 {
		t.Errorf("expected summary sliced to 1500 chars, got %d", len(res.SummaryInjected))
	}
}

func TestBuildPrompt_ReviewStage(t *testing.T) {
	pc := &service.PromptContext{
		Reason:          service.RunReasonTaskAssigned,
		TaskIdentifier:  "KAN-10",
		TaskTitle:       "Auth service",
		TaskDescription: "Implement OAuth2 flow.",
		StageID:         "stage-review-1",
		StageType:       "review",
		BuilderComments: []string{"Done the implementation", "Added tests"},
	}
	prompt := service.BuildPrompt(pc)

	checks := []string{
		"You are reviewing task",
		"[KAN-10]",
		"Auth service",
		"Task description:",
		"Implement OAuth2 flow.",
		"Builder's comments:",
		"Done the implementation",
		"Added tests",
		"Review the implementation carefully",
		"approve if the work is satisfactory",
		"reject with specific feedback",
	}
	for _, c := range checks {
		if !strings.Contains(prompt, c) {
			t.Errorf("review stage prompt missing %q:\n%s", c, prompt)
		}
	}

	// Should NOT contain the default work assignment phrasing.
	if strings.Contains(prompt, "You have been assigned") {
		t.Errorf("review prompt should not contain assignment phrasing:\n%s", prompt)
	}
}

func TestBuildPrompt_ShipStage(t *testing.T) {
	pc := &service.PromptContext{
		Reason:         service.RunReasonTaskAssigned,
		TaskIdentifier: "KAN-20",
		TaskTitle:      "Deploy to production",
		StageID:        "stage-ship-1",
		StageType:      "ship",
	}
	prompt := service.BuildPrompt(pc)

	checks := []string{
		"[KAN-20]",
		"Deploy to production",
		"approved by all reviewers",
		"pull request",
		"format, lint, test",
	}
	for _, c := range checks {
		if !strings.Contains(prompt, c) {
			t.Errorf("ship stage prompt missing %q:\n%s", c, prompt)
		}
	}

	if strings.Contains(prompt, "You have been assigned") {
		t.Errorf("ship prompt should not contain assignment phrasing:\n%s", prompt)
	}
}

func TestBuildPrompt_WorkWithFeedback(t *testing.T) {
	pc := &service.PromptContext{
		Reason:         service.RunReasonTaskAssigned,
		TaskIdentifier: "KAN-30",
		TaskTitle:      "Fix the login bug",
		StageID:        "stage-work-1",
		StageType:      "work",
		ReviewFeedback: "[reject] reviewer-1: Missing error handling\n[reject] reviewer-2: No tests added",
	}
	prompt := service.BuildPrompt(pc)

	checks := []string{
		"[KAN-30]",
		"Fix the login bug",
		"returned by reviewers with feedback",
		"Reviewer feedback:",
		"Missing error handling",
		"No tests added",
		"Address the feedback and resubmit",
	}
	for _, c := range checks {
		if !strings.Contains(prompt, c) {
			t.Errorf("rework prompt missing %q:\n%s", c, prompt)
		}
	}

	if strings.Contains(prompt, "You have been assigned") {
		t.Errorf("rework prompt should not contain default assignment phrasing:\n%s", prompt)
	}
}

func TestBuildPrompt_WorkNoFeedback(t *testing.T) {
	pc := &service.PromptContext{
		Reason:          service.RunReasonTaskAssigned,
		TaskIdentifier:  "KAN-40",
		TaskTitle:       "Build the dashboard",
		TaskDescription: "Create the metrics dashboard.",
		StageID:         "stage-work-1",
		StageType:       "work",
		TaskPriority:    "low",
	}
	prompt := service.BuildPrompt(pc)

	// Should behave exactly like the default work prompt.
	checks := []string{
		"You have been assigned task",
		"[KAN-40]",
		"Build the dashboard",
		"Description: Create the metrics dashboard.",
		"Priority: low",
		"start working",
	}
	for _, c := range checks {
		if !strings.Contains(prompt, c) {
			t.Errorf("default work prompt missing %q:\n%s", c, prompt)
		}
	}

	if strings.Contains(prompt, "returned by reviewers") {
		t.Errorf("no-feedback work prompt should not mention reviewers:\n%s", prompt)
	}
}
