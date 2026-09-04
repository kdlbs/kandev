import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { waitForHttp, watchWs } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/animations";

// Surfaces task priority in the kanban UI: REQ-TASKS-PRIORITY-VISIBILITY-001
// (card indicator), -002 (create dialog control) and -003 (card menu
// action), per docs/specs/tasks/requirements/task-priority-visibility.md.
//
// AC-001.3 / AC-003.2's absent-or-unrecognized-priority branch is not
// exercised here: the live REST update path applies the CHECK-constrained
// vocabulary or the `medium` default on every write a real user flow can
// reach, so there is no legitimate way to leave a task's priority absent or
// out-of-vocabulary through the UI. That branch is unit-tested directly
// (kanban-card-priority-indicator.test.tsx, kanban-card-menu-items.test.tsx).

test.describe("Kanban card — priority indicator", () => {
  test("renders for critical/high/low, omits for medium, and survives first paint", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const critical = await apiClient.createTask(seedData.workspaceId, "Priority Critical Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      priority: "critical",
    });
    const low = await apiClient.createTask(seedData.workspaceId, "Priority Low Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      priority: "low",
    });
    const medium = await apiClient.createTask(seedData.workspaceId, "Priority Medium Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      priority: "medium",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Reload before the first assertion: the boot payload (Go
    // mapKanbanTaskState) must carry priority on first paint, not just after
    // a later task.updated event patches the store (AC-001.7).
    await testPage.reload();
    await kanban.board.waitFor({ state: "visible" });

    const criticalCard = kanban.taskCard(critical.id);
    await expect(criticalCard).toBeVisible({ timeout: 20_000 });
    const criticalIndicator = criticalCard.getByTestId("kanban-card-priority-indicator");
    await expect(criticalIndicator).toBeVisible();
    await expect(criticalIndicator).toHaveAttribute("aria-label", "Priority Critical");

    const lowCard = kanban.taskCard(low.id);
    await expect(lowCard).toBeVisible({ timeout: 20_000 });
    const lowIndicator = lowCard.getByTestId("kanban-card-priority-indicator");
    await expect(lowIndicator).toBeVisible();
    await expect(lowIndicator).toHaveAttribute("aria-label", "Priority Low");

    // Distinguishable by more than color (AC-001.4): different icons render
    // different inner markup, so the two indicators' HTML must differ.
    expect(await criticalIndicator.innerHTML()).not.toBe(await lowIndicator.innerHTML());

    // `medium` is the default/majority case: no indicator at all (AC-001.2).
    const mediumCard = kanban.taskCard(medium.id);
    await expect(mediumCard).toBeVisible({ timeout: 20_000 });
    await expect(mediumCard.getByTestId("kanban-card-priority-indicator")).toHaveCount(0);

    // Rendering the indicator must not reorder, regroup or re-position any
    // card (AC-001.6): the column's order is identical before and after a
    // repaint that recomputes every card's indicator from scratch.
    const titlesLocator = kanban
      .columnByStepId(seedData.startStepId)
      .locator('[data-testid="task-card-title"]');
    const orderBeforeReload = await titlesLocator.allTextContents();
    await testPage.reload();
    await kanban.board.waitFor({ state: "visible" });
    await expect(kanban.taskCard(medium.id)).toBeVisible({ timeout: 20_000 });
    const orderAfterReload = await titlesLocator.allTextContents();
    expect(orderAfterReload).toEqual(orderBeforeReload);
  });
});

test.describe("Task creation — priority control", () => {
  test("defaults to Medium, offers all four tokens, and the selection is persisted", async ({
    testPage,
    apiClient,
  }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await dialog.getByTestId("task-create-advanced-settings-trigger").click();
    await waitForFiniteAnimations(dialog);
    const select = dialog.getByTestId("task-create-priority-select");
    await expect(select).toContainText("Medium");
    const dependencyRow = dialog.getByTestId("task-create-dependency-setting-row");
    const priorityRow = dialog.getByTestId("task-create-priority-setting-row");
    const [dependencyBox, priorityBox] = await Promise.all([
      dependencyRow.boundingBox(),
      priorityRow.boundingBox(),
    ]);
    expect(dependencyBox).not.toBeNull();
    expect(priorityBox).not.toBeNull();
    expect(priorityBox!.x).toBeGreaterThan(dependencyBox!.x);
    await expect(select).toHaveClass(/bg-muted\/30/);

    const priorityInfo = dialog.getByTestId("task-create-priority-setting-info");
    await priorityInfo.hover();
    await expect(testPage.getByRole("tooltip")).toContainText(
      "Priority shows how urgent this task is on the board.",
    );

    await select.click();
    const listbox = testPage.getByRole("listbox");
    await expect(listbox.getByTestId("task-create-priority-option-critical")).toBeVisible();
    await expect(listbox.getByTestId("task-create-priority-option-high")).toBeVisible();
    await expect(listbox.getByTestId("task-create-priority-option-medium")).toBeVisible();
    await expect(listbox.getByTestId("task-create-priority-option-low")).toBeVisible();
    await listbox.getByTestId("task-create-priority-option-high").click();
    await expect(select).toContainText("High");

    await testPage.getByTestId("task-title-input").fill("Priority Create Dialog Task");
    await testPage.getByTestId("task-description-input").fill("verifies submitted priority");

    // Selecting a priority does not gate or reorder any other step of
    // creation (AC-002.5): the split "start without agent" path still works.
    await expect(testPage.getByTestId("submit-start-agent-chevron")).toBeEnabled({
      timeout: 30_000,
    });
    await testPage.getByTestId("submit-start-agent-chevron").click();
    const createdResponse = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
    await testPage.getByTestId("submit-create-without-agent").click();
    const response = await createdResponse;
    const created = (await response.json()) as { id: string; priority?: string };

    expect(created.priority).toBe("high");
    const fetched = await apiClient.getTask(created.id);
    expect(fetched.priority).toBe("high");

    await kanban.goto();
    const card = kanban.taskCardByTitle("Priority Create Dialog Task");
    await expect(card).toBeVisible({ timeout: 20_000 });
    const indicator = card.getByTestId("kanban-card-priority-indicator");
    await expect(indicator).toBeVisible();
    await expect(indicator).toHaveAttribute("aria-label", "Priority High");
  });
});

test.describe("Kanban card menu — priority action", () => {
  test("both triggers show the current token, change it live, and converge on remote writes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Priority Menu Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      priority: "medium",
    });

    const kanban = new KanbanPage(testPage);
    const ws = watchWs(testPage);
    await kanban.goto();

    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible({ timeout: 20_000 });

    // Right-click context menu (desktop trigger): current token indicated,
    // all four remain selectable, medium renders no indicator on the card.
    await kanban.openTaskContextMenu(task.id);
    await expect(kanban.contextPriority()).toBeVisible();
    await kanban.openPrioritySubmenu();
    await expect(kanban.contextPriorityCurrent("medium")).toBeVisible();
    await expect(kanban.contextPriorityOption("critical")).toBeEnabled();
    await testPage.keyboard.press("Escape");

    // Changing priority from the context menu persists only that field
    // (AC-003.4) and reflects live via task.updated (AC-001.8), no reload.
    const setCritical = ws.waitForEvent("task.updated", {
      where: (payload) => payload.task_id === task.id && payload.priority === "critical",
    });
    await kanban.setPriorityFromContextMenu(task.id, "critical");
    await setCritical;

    await expect(card.getByTestId("kanban-card-priority-indicator")).toBeVisible({
      timeout: 10_000,
    });
    const afterFirstChange = await apiClient.getTask(task.id);
    expect(afterFirstChange.priority).toBe("critical");
    expect(afterFirstChange.title).toBe("Priority Menu Fixture");
    expect(afterFirstChange.workflow_step_id).toBe(seedData.startStepId);

    // Dots-dropdown trigger (the mobile-reachable path, AC-003.3): reselecting
    // the same token completes idempotently with no error (AC-003.5).
    await kanban.openTaskActionsMenu(task.id);
    await expect(kanban.contextPriority()).toBeVisible();
    await kanban.openPrioritySubmenu();
    await expect(kanban.contextPriorityCurrent("critical")).toBeVisible();
    await testPage.keyboard.press("Escape");

    const reselectSameEvent = ws.waitForEvent("task.updated", {
      where: (payload) => payload.task_id === task.id && payload.priority === "critical",
    });
    await kanban.setPriorityFromActionsMenu(task.id, "critical");
    await reselectSameEvent;
    await expect(card.getByTestId("kanban-card-priority-indicator")).toBeVisible();
    expect((await apiClient.getTask(task.id)).priority).toBe("critical");

    // A write from another REST caller (simulating another browser client)
    // must reach this open board without a reload or menu interaction.
    const remoteWrite = ws.waitForEvent("task.updated", {
      where: (payload) => payload.task_id === task.id && payload.priority === "low",
    });
    await apiClient.updateTaskPriority(task.id, "low");
    await remoteWrite;
    await expect(card.getByTestId("kanban-card-priority-indicator")).toHaveAttribute(
      "aria-label",
      "Priority Low",
    );

    // A failed persist surfaces a toast and leaves the card showing the last
    // known stored value (AC-003.7) — never silently "succeeding".
    const backendErrorMessage = "simulated priority update failure";
    await testPage.route(`**/api/v1/tasks/${task.id}`, async (route) => {
      const request = route.request();
      if (request.method() !== "PATCH") {
        await route.continue();
        return;
      }
      const body = request.postDataJSON() as { priority?: string };
      if (!body.priority) {
        await route.continue();
        return;
      }
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: backendErrorMessage }),
      });
    });

    await kanban.setPriorityFromContextMenu(task.id, "high");
    const toast = testPage
      .getByTestId("toast-message")
      .filter({ hasText: "Failed to update task" });
    await expect(toast).toBeVisible({ timeout: 5_000 });
    await expect(toast).toContainText(backendErrorMessage);

    // The card never claimed the failed change succeeded, and the backend
    // value is unchanged.
    await expect(card.getByTestId("kanban-card-priority-indicator")).toHaveAttribute(
      "aria-label",
      "Priority Low",
    );
    expect((await apiClient.getTask(task.id)).priority).toBe("low");
  });
});
