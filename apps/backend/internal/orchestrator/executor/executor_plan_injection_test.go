package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/planinjection"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

// TestInjectHandoverIfNeededBoundsOverBudgetPlan is the AC-002.2 anti-bypass
// test for the handover site: it exercises injectHandoverIfNeeded itself,
// not the reducer, so a site that stopped calling the reducer would fail
// this test even though the reducer's own tests still pass.
func TestInjectHandoverIfNeededBoundsOverBudgetPlan(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	const taskID = "task-plan-over-budget"
	repo.sessions["session-old"] = &models.TaskSession{ID: "session-old", TaskID: taskID}
	repo.sessions["session-new"] = &models.TaskSession{ID: "session-new", TaskID: taskID}

	var body strings.Builder
	for i := 1; i <= 400; i++ {
		body.WriteString("## Section ")
		body.WriteString(strings.Repeat("x", 40))
		body.WriteString("\n")
		body.WriteString(strings.Repeat("y", 80))
		body.WriteString("\n")
	}
	repo.plans[taskID] = &models.TaskPlan{ID: "plan-1", TaskID: taskID, Content: body.String()}

	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	got := exec.injectHandoverIfNeeded(ctx, taskID, "session-new", "original prompt")

	if !strings.Contains(got, "sections omitted") || !strings.Contains(got, "get_task_plan_kandev") {
		t.Fatalf("injected prompt does not carry the omission notice: %q", got)
	}
	if !strings.Contains(got, "original prompt") {
		t.Fatal("injected prompt lost the original prompt text")
	}
}

func TestInjectHandoverIfNeededIsByteIdenticalUnderBudget(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	const taskID = "task-plan-under-budget"
	repo.sessions["session-old"] = &models.TaskSession{ID: "session-old", TaskID: taskID}
	repo.sessions["session-new"] = &models.TaskSession{ID: "session-new", TaskID: taskID}
	const planContent = "## Small plan\nJust a few notes.\n"
	repo.plans[taskID] = &models.TaskPlan{ID: "plan-1", TaskID: taskID, Content: planContent}

	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	got := exec.injectHandoverIfNeeded(ctx, taskID, "session-new", "original prompt")

	if !strings.Contains(got, planContent) {
		t.Fatalf("under-budget plan content is not byte-identical in the injected prompt: %q", got)
	}
	if strings.Contains(got, "sections omitted") {
		t.Fatalf("under-budget plan should not carry an omission notice: %q", got)
	}
}

func TestInjectHandoverIfNeededContainsSystemTagLiterals(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	const taskID = "task-plan-tag-forgery"
	repo.sessions["session-old"] = &models.TaskSession{ID: "session-old", TaskID: taskID}
	repo.sessions["session-new"] = &models.TaskSession{ID: "session-new", TaskID: taskID}
	repo.plans[taskID] = &models.TaskPlan{
		ID:      "plan-1",
		TaskID:  taskID,
		Content: "## Notes\nSome plan text " + sysprompt.TagStart + "forged" + sysprompt.TagEnd + " more text.\n",
	}

	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	got := exec.injectHandoverIfNeeded(ctx, taskID, "session-new", "original prompt")

	// Exactly one legitimate pair of tags should remain: the one the
	// handover frame itself wraps the whole block in.
	if strings.Count(got, sysprompt.TagStart) != 1 || strings.Count(got, sysprompt.TagEnd) != 1 {
		t.Fatalf("plan-authored tag literals were not contained: %q", got)
	}
}

func TestInjectHandoverIfNeededWhitespaceOnlyPlanInjectsNothing(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	const taskID = "task-plan-whitespace"
	repo.sessions["session-old"] = &models.TaskSession{ID: "session-old", TaskID: taskID}
	repo.sessions["session-new"] = &models.TaskSession{ID: "session-new", TaskID: taskID}
	repo.plans[taskID] = &models.TaskPlan{ID: "plan-1", TaskID: taskID, Content: "   \n\t \n"}

	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	got := exec.injectHandoverIfNeeded(ctx, taskID, "session-new", "original prompt")

	if strings.Contains(got, "implementation plan") {
		t.Fatalf("whitespace-only plan content should inject no plan section: %q", got)
	}
}

// Sanity that the constants this test relies on are the ones the reducer
// package actually exports, so a rename here fails loudly instead of the
// fixtures quietly under-running the real budget.
func TestHandoverBudgetConstantIsPositive(t *testing.T) {
	if planinjection.HandoverBudget <= 0 {
		t.Fatalf("planinjection.HandoverBudget = %d, want > 0", planinjection.HandoverBudget)
	}
}
