import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { watchWs } from "../../helpers/causal-waits";

// Touch/narrow-viewport coverage for REQ-TASKS-PRIORITY-VISIBILITY-001..003
// (docs/specs/tasks/requirements/task-priority-visibility.md), run on the
// mobile-chrome (Pixel 5) project: the card indicator, the full-screen create
// form's priority control, and AC-003.3's dots-dropdown priority action,
// which is the only card-menu trigger reachable on a touch device.

test.describe("Mobile kanban — task priority", () => {
  test("shows the priority indicator, offers priority on create, and changes it from the dots menu", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile Priority Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      priority: "critical",
    });

    const mobile = new MobileKanbanPage(testPage);
    const kanban = new KanbanPage(testPage);
    const ws = watchWs(testPage);
    await mobile.goto();

    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible({ timeout: 20_000 });
    await expect(card.getByTestId("kanban-card-priority-indicator")).toHaveAttribute(
      "aria-label",
      "Priority Critical",
    );

    // Full-screen creation form: the priority control is reachable and
    // submits with the board rendered at a phone viewport.
    await mobile.mobileFab.click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const select = dialog.getByTestId("task-create-priority-select");
    await expect(select).toContainText("Medium");
    await select.click();
    const listbox = testPage.getByRole("listbox");
    await listbox.getByTestId("task-create-priority-option-high").click();
    await expect(select).toContainText("High");
    await testPage.getByTestId("task-title-input").fill("Mobile Priority Create Task");
    await testPage.getByTestId("task-description-input").fill("verifies mobile submitted priority");
    // Below `sm`, the split "start task" button collapses to two full-width
    // buttons instead of a button+dropdown pair (submit-start-agent-chevron
    // is `hidden sm:flex`), so the alt action has no separate testid here.
    await testPage.getByRole("button", { name: "Create only", exact: true }).click();
    await expect(dialog).toHaveCount(0);

    const createdCard = kanban.taskCardByTitle("Mobile Priority Create Task");
    await expect(createdCard).toBeVisible({ timeout: 20_000 });
    await expect(createdCard.getByTestId("kanban-card-priority-indicator")).toHaveAttribute(
      "aria-label",
      "Priority High",
    );

    // Dots-dropdown trigger (AC-003.3): the only card-menu trigger reachable
    // on a touch device, since the right-click context menu is desktop-only.
    const setLow = ws.waitForEvent("task.updated", {
      where: (payload) => payload.task_id === task.id && payload.priority === "low",
    });
    await kanban.openTaskActionsMenu(task.id);
    await expect(kanban.contextPriority()).toBeVisible();
    await kanban.openPrioritySubmenu();
    await expect(kanban.contextPriorityCurrent("critical")).toBeVisible();
    await testPage.keyboard.press("Escape");
    await kanban.setPriorityFromActionsMenu(task.id, "low");
    await setLow;

    await expect(card.getByTestId("kanban-card-priority-indicator")).toHaveAttribute(
      "aria-label",
      "Priority Low",
    );
    expect((await apiClient.getTask(task.id)).priority).toBe("low");
  });
});
