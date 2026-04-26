package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func TestSchedulerIntegration_TickProcessesWakeup(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-tick", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue wakeup: %v", err)
	}

	// Create the integration and run a single tick via exposed service methods
	// (the tick loop is a background goroutine; we test the pipeline manually).
	wakeup, err := svc.ClaimNextWakeup(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if wakeup == nil {
		t.Fatal("expected a wakeup, got nil")
	}

	// Guard should allow idle agent.
	ok, err := svc.ProcessWakeupGuard(ctx, wakeup)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if !ok {
		t.Fatal("guard should allow idle agent")
	}

	// Finish the wakeup.
	if err := svc.FinishWakeup(ctx, wakeup.ID); err != nil {
		t.Fatalf("finish: %v", err)
	}

	// Queue should be empty now.
	next, _ := svc.ClaimNextWakeup(ctx)
	if next != nil {
		t.Error("expected no more wakeups after processing")
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
	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	// Pause agent.
	if _, err := svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusPaused, "test"); err != nil {
		t.Fatalf("pause: %v", err)
	}

	// Claim should return nil because the agent is paused (capacity check).
	wakeup, err := svc.ClaimNextWakeup(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if wakeup != nil {
		// Agent is paused so should not be claimable. If the DB-level claim
		// query allows it (it checks status IN ('idle','working')), this test
		// confirms the guard would catch it.
		ok, gErr := svc.ProcessWakeupGuard(ctx, wakeup)
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

	// Queue two wakeups.
	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, `{"task_id":"t1"}`, "k1"); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskComment, `{"task_id":"t2"}`, "k2"); err != nil {
		t.Fatalf("queue second: %v", err)
	}

	// Claim the first wakeup (agent at capacity now: 1 claimed).
	first, err := svc.ClaimNextWakeup(ctx)
	if err != nil {
		t.Fatalf("claim first: %v", err)
	}
	if first == nil {
		t.Fatal("expected first wakeup")
	}

	// Second claim should return nil (at capacity).
	second, err := svc.ClaimNextWakeup(ctx)
	if err != nil {
		t.Fatalf("claim second: %v", err)
	}
	if second != nil {
		t.Error("expected nil (agent at capacity), got a wakeup")
	}

	// Finish the first wakeup.
	if err := svc.FinishWakeup(ctx, first.ID); err != nil {
		t.Fatalf("finish: %v", err)
	}

	// Now the second should be claimable.
	second, err = svc.ClaimNextWakeup(ctx)
	if err != nil {
		t.Fatalf("claim second after finish: %v", err)
	}
	if second == nil {
		t.Fatal("expected second wakeup to be claimable after first finished")
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
			reason:   service.WakeupReasonTaskAssigned,
			payload:  `{"task_id":"task-1"}`,
			contains: "Build feature X",
		},
		{
			name:     "task_comment",
			reason:   service.WakeupReasonTaskComment,
			payload:  `{"task_id":"task-1"}`,
			contains: "Build feature X",
		},
		{
			name:     "approval_resolved",
			reason:   service.WakeupReasonApprovalResolved,
			payload:  `{"approval_id":"a1","status":"approved","decision_note":"looks good"}`,
			contains: "approved",
		},
		{
			name:     "heartbeat",
			reason:   service.WakeupReasonHeartbeat,
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

			if err := svc.QueueWakeup(ctx, agent.ID, tt.reason, tt.payload, ""); err != nil {
				t.Fatalf("queue: %v", err)
			}

			wakeup, err := svc.ClaimNextWakeup(ctx)
			if err != nil {
				t.Fatalf("claim: %v", err)
			}
			if wakeup == nil {
				t.Fatal("expected wakeup")
			}

			pc := service.BuildPromptContextForTest(svc, ctx, wakeup.Reason, wakeup.Payload)
			prompt := service.BuildPrompt(pc)
			if !containsIgnoreCase(prompt, tt.contains) {
				t.Errorf("prompt should contain %q, got: %s", tt.contains, prompt)
			}

			_ = svc.FinishWakeup(ctx, wakeup.ID)
		})
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
func insertTaskForPrompt(t *testing.T, svc *service.Service, id, wsID, title, desc string, priority int) {
	t.Helper()
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, priority, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, id, wsID, title, desc, priority)
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
