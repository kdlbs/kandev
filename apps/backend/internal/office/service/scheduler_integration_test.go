package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

func TestSchedulerIntegration_TickProcessesRun(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-tick", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}

	// Create the integration and run a single tick via exposed service methods
	// (the tick loop is a background goroutine; we test the pipeline manually).
	run, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if run == nil {
		t.Fatal("expected a run, got nil")
	}

	// Guard should allow idle agent.
	ok, err := svc.ProcessRunGuard(ctx, run)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if !ok {
		t.Fatal("guard should allow idle agent")
	}

	// Finish the run.
	if err := svc.FinishRun(ctx, run.ID); err != nil {
		t.Fatalf("finish: %v", err)
	}

	// Queue should be empty now.
	next, _ := svc.ClaimNextRun(ctx)
	if next != nil {
		t.Error("expected no more runs after processing")
	}
}

func TestSchedulerIntegration_PausedAgentSkipped(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-paused", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Queue while agent is active.
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	// Pause agent.
	if _, err := svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusPaused, "test"); err != nil {
		t.Fatalf("pause: %v", err)
	}

	// Claim should return nil because the agent is paused (capacity check).
	run, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if run != nil {
		// Agent is paused so should not be claimable. If the DB-level claim
		// query allows it (it checks status IN ('idle','working')), this test
		// confirms the guard would catch it.
		ok, gErr := svc.ProcessRunGuard(ctx, run)
		if gErr != nil {
			t.Fatalf("guard: %v", gErr)
		}
		if ok {
			t.Error("guard should block paused agent")
		}
	}
}

func TestSchedulerIntegration_AtCapacityStaysQueued(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-busy", models.AgentRoleWorker)
	// max_concurrent_sessions defaults to 1
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Queue two runs.
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"t1"}`, "k1"); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskComment, `{"task_id":"t2"}`, "k2"); err != nil {
		t.Fatalf("queue second: %v", err)
	}

	// Claim the first run (agent at capacity now: 1 claimed).
	first, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim first: %v", err)
	}
	if first == nil {
		t.Fatal("expected first run")
	}

	// Second claim should return nil (at capacity).
	second, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim second: %v", err)
	}
	if second != nil {
		t.Error("expected nil (agent at capacity), got a run")
	}

	// Finish the first run.
	if err := svc.FinishRun(ctx, first.ID); err != nil {
		t.Fatalf("finish: %v", err)
	}

	// Now the second should be claimable.
	second, err = svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim second after finish: %v", err)
	}
	if second == nil {
		t.Fatal("expected second run to be claimable after first finished")
	}
}

func TestSchedulerIntegration_PromptBuiltCorrectly(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Insert a test task.
	insertTaskForPrompt(t, svc, "task-1", "ws-1", "Build feature X", "Implement the API endpoint", 3)

	tests := []struct {
		name     string
		reason   string
		payload  string
		contains string
	}{
		{
			name:     "task_assigned",
			reason:   service.RunReasonTaskAssigned,
			payload:  `{"task_id":"task-1"}`,
			contains: "Build feature X",
		},
		{
			name:     "task_comment",
			reason:   service.RunReasonTaskComment,
			payload:  `{"task_id":"task-1"}`,
			contains: "Build feature X",
		},
		{
			name:     "approval_resolved",
			reason:   service.RunReasonApprovalResolved,
			payload:  `{"approval_id":"a1","status":"approved","decision_note":"looks good"}`,
			contains: "approved",
		},
		{
			name:     "heartbeat",
			reason:   service.RunReasonHeartbeat,
			payload:  `{}`,
			contains: "status update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := makeAgent("worker-"+tt.name, models.AgentRoleWorker)
			if err := svc.CreateAgentInstance(ctx, agent); err != nil {
				t.Fatalf("create agent: %v", err)
			}

			if err := svc.QueueRun(ctx, agent.ID, tt.reason, tt.payload, ""); err != nil {
				t.Fatalf("queue: %v", err)
			}

			run, err := svc.ClaimNextRun(ctx)
			if err != nil {
				t.Fatalf("claim: %v", err)
			}
			if run == nil {
				t.Fatal("expected run")
			}

			pc := service.BuildPromptContextForTest(svc, ctx, run.Reason, run.Payload)
			prompt := service.BuildPrompt(pc)
			if !containsIgnoreCase(prompt, tt.contains) {
				t.Errorf("prompt should contain %q, got: %s", tt.contains, prompt)
			}

			_ = svc.FinishRun(ctx, run.ID)
		})
	}
}

func TestSchedulerIntegration_BuildPromptContext_TaskComment(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Insert task and a user-authored comment.
	insertTaskForPrompt(t, svc, "task-c1", "ws-1", "Investigate", "Look into bug", 3)
	svc.ExecSQL(t, `INSERT INTO task_comments (id, task_id, author_type, author_id, body, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		"cmt-1", "task-c1", "user", "user-1", "say the current date", "user")

	payload := `{"task_id":"task-c1","comment_id":"cmt-1"}`
	pc := service.BuildPromptContextForTest(svc, ctx, service.RunReasonTaskComment, payload)

	if pc.CommentBody != "say the current date" {
		t.Errorf("CommentBody = %q, want %q", pc.CommentBody, "say the current date")
	}
	if pc.CommentAuthorType != "user" {
		t.Errorf("CommentAuthorType = %q, want %q", pc.CommentAuthorType, "user")
	}
	if pc.CommentAuthor != "User" {
		t.Errorf("CommentAuthor = %q, want %q", pc.CommentAuthor, "User")
	}

	// The fully-built prompt should now include the body.
	prompt := service.BuildPrompt(pc)
	if !containsIgnoreCase(prompt, "say the current date") {
		t.Errorf("prompt missing comment body, got: %s", prompt)
	}
}

func TestSchedulerIntegration_BuildPromptContext_TaskComment_AgentAuthor(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create an agent that will author the comment.
	agent := makeAgent("commenter-bot", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	insertTaskForPrompt(t, svc, "task-c2", "ws-1", "Investigate", "Look into bug", 3)
	svc.ExecSQL(t, `INSERT INTO task_comments (id, task_id, author_type, author_id, body, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		"cmt-2", "task-c2", "agent", agent.ID, "needs more detail", "user")

	payload := `{"task_id":"task-c2","comment_id":"cmt-2"}`
	pc := service.BuildPromptContextForTest(svc, ctx, service.RunReasonTaskComment, payload)

	if pc.CommentBody != "needs more detail" {
		t.Errorf("CommentBody = %q, want %q", pc.CommentBody, "needs more detail")
	}
	if pc.CommentAuthorType != "agent" {
		t.Errorf("CommentAuthorType = %q, want %q", pc.CommentAuthorType, "agent")
	}
	if pc.CommentAuthor != "commenter-bot" {
		t.Errorf("CommentAuthor = %q, want %q", pc.CommentAuthor, "commenter-bot")
	}
}

func TestSchedulerIntegration_BuildPromptContext_TaskComment_MissingComment(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	insertTaskForPrompt(t, svc, "task-c3", "ws-1", "Title", "Desc", 3)

	// comment_id refers to a row that does not exist; should not panic and
	// should leave comment fields empty.
	payload := `{"task_id":"task-c3","comment_id":"does-not-exist"}`
	pc := service.BuildPromptContextForTest(svc, ctx, service.RunReasonTaskComment, payload)

	if pc.CommentBody != "" || pc.CommentAuthor != "" || pc.CommentAuthorType != "" {
		t.Errorf("expected empty comment fields on missing comment, got body=%q author=%q type=%q",
			pc.CommentBody, pc.CommentAuthor, pc.CommentAuthorType)
	}
}

func TestSchedulerIntegration_StartStopsOnContextCancel(t *testing.T) {
	svc := newTestService(t)
	si := service.NewSchedulerIntegration(svc, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		si.Start(ctx)
		close(done)
	}()

	// Let a few ticks run.
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// OK - Start returned.
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

// insertTaskForPrompt inserts a task into the test database for prompt building.
// `priority` is accepted as int for legacy callers; the value is mapped to the
// nearest TEXT priority label since the column type is now TEXT.
func insertTaskForPrompt(t *testing.T, svc *service.Service, id, wsID, title, desc string, priority int) {
	t.Helper()
	label := "medium"
	switch {
	case priority >= 8:
		label = "critical"
	case priority >= 4:
		label = "high"
	case priority >= 1:
		label = "low"
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, priority, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, id, wsID, title, desc, label)
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		containsLower(s, substr)
}

func containsLower(s, substr string) bool {
	sl := toLower(s)
	subl := toLower(substr)
	for i := 0; i <= len(sl)-len(subl); i++ {
		if sl[i:i+len(subl)] == subl {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
