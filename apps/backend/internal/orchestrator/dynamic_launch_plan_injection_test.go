package orchestrator

import (
	"context"
	"strings"
	"testing"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/task/models"
)

// TestAddDynamicPlanBoundsOverBudgetPlan is the AC-002.2 anti-bypass test for
// the dynamic continuation site: it exercises addDynamicPlan itself, not the
// reducer, so a site that stopped calling the reducer would fail this test
// even though the reducer's own tests still pass.
func TestAddDynamicPlanBoundsOverBudgetPlan(t *testing.T) {
	ctx := context.Background()
	const taskID = "task-dynamic-plan-over-budget"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, "session-dynamic-plan-over-budget", models.TaskSessionStateRunning)

	var body strings.Builder
	for i := 1; i <= 400; i++ {
		body.WriteString("## Section ")
		body.WriteString(strings.Repeat("x", 40))
		body.WriteString("\n")
		body.WriteString(strings.Repeat("y", 80))
		body.WriteString("\n")
	}
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-dynamic-over-budget", TaskID: taskID, Title: "Big plan", Content: body.String(),
	}); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}

	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

	var input dynamicruntime.ContinuationInput
	if err := svc.addDynamicPlan(ctx, taskID, &input); err != nil {
		t.Fatalf("addDynamicPlan: %v", err)
	}

	if !strings.Contains(input.PlanSummary, "sections omitted") || !strings.Contains(input.PlanSummary, "get_task_plan_kandev") {
		t.Fatalf("PlanSummary does not carry the omission notice: %q", input.PlanSummary)
	}
	if len(input.PlanSummary) > 4000 {
		t.Fatalf("PlanSummary len = %d, exceeds the dynamic budget", len(input.PlanSummary))
	}
}

func TestAddDynamicPlanIsByteIdenticalUnderBudget(t *testing.T) {
	ctx := context.Background()
	const taskID = "task-dynamic-plan-under-budget"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, "session-dynamic-plan-under-budget", models.TaskSessionStateRunning)

	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-dynamic-under-budget", TaskID: taskID, Title: "Small plan", Content: "## Notes\nJust a few notes.\n",
	}); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}

	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

	var input dynamicruntime.ContinuationInput
	if err := svc.addDynamicPlan(ctx, taskID, &input); err != nil {
		t.Fatalf("addDynamicPlan: %v", err)
	}

	want := "Small plan\n## Notes\nJust a few notes."
	if input.PlanSummary != want {
		t.Fatalf("PlanSummary = %q, want %q (byte-identical to today's composition)", input.PlanSummary, want)
	}
}
