package service_test

import (
	"os"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/service"
)

func TestBuildPrompt_TaskAssigned(t *testing.T) {
	pc := &service.PromptContext{
		Reason:          service.WakeupReasonTaskAssigned,
		TaskIdentifier:  "KAN-3",
		TaskTitle:       "Implement login page",
		ProjectName:     "Frontend",
		TaskDescription: "Build the login form with OAuth.",
		TaskPriority:    2,
	}
	prompt := service.BuildPrompt(pc)

	checks := []string{
		"[KAN-3]",
		"Implement login page",
		"Project: Frontend",
		"Description: Build the login form",
		"Priority: 2",
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
		Reason:            service.WakeupReasonTaskComment,
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
		Reason:                service.WakeupReasonTaskBlockersResolved,
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
		Reason:         service.WakeupReasonApprovalResolved,
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
		Reason:          service.WakeupReasonHeartbeat,
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

func TestBuildPrompt_UnknownReason(t *testing.T) {
	pc := &service.PromptContext{Reason: "custom_reason"}
	prompt := service.BuildPrompt(pc)
	if !strings.Contains(prompt, "custom_reason") {
		t.Errorf("unknown reason prompt should mention the reason:\n%s", prompt)
	}
}

func TestBuildPrompt_TaskIDFallback(t *testing.T) {
	pc := &service.PromptContext{
		Reason:    service.WakeupReasonTaskAssigned,
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

	// Write AGENTS.md to a temp dir.
	dir := t.TempDir()
	if err := writeTestFile(t, dir, "AGENTS.md", "# You are the CEO."); err != nil {
		t.Fatal(err)
	}

	wakeContext := "You have been assigned task [KAN-1]: Build it."
	prompt := svc.BuildAgentPrompt(nil, nil, dir, false, wakeContext)

	if !strings.Contains(prompt, "# You are the CEO.") {
		t.Errorf("prompt should contain AGENTS.md content:\n%s", prompt)
	}
	if !strings.Contains(prompt, dir+"/AGENTS.md") {
		t.Errorf("prompt should contain path directive:\n%s", prompt)
	}
	if !strings.Contains(prompt, "./HEARTBEAT.md") {
		t.Errorf("prompt should reference sibling files:\n%s", prompt)
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
	if err := writeTestFile(t, dir, "AGENTS.md", "# Instructions"); err != nil {
		t.Fatal(err)
	}

	wakeContext := "New comment on your task."
	prompt := svc.BuildAgentPrompt(nil, nil, dir, true, wakeContext)

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
	prompt := svc.BuildAgentPrompt(nil, nil, "", false, wakeContext)

	if !strings.Contains(prompt, "Heartbeat check") {
		t.Errorf("prompt should contain wake context even without instructions dir:\n%s", prompt)
	}
}

func writeTestFile(t *testing.T, dir, name, content string) error {
	t.Helper()
	return os.WriteFile(dir+"/"+name, []byte(content), 0o644)
}
