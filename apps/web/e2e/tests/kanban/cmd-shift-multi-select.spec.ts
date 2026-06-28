/**
 * E2E: cmd/ctrl-click and shift-click multi-select on the Kanban board.
 *
 * Covers the modifier-driven entry into multi-select mode (no toggle button
 * needed), per-card toggle, same-column shift range, the cross-column fallback,
 * and plain-click toggle behaviour while in multi-select mode.
 */
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

// Scope DOM reads to the active board so a hidden/stale mounted layout can't
// satisfy these selectors (dock/mobile layouts can coexist in the DOM).
function activeColumn(page: Page, stepId: string) {
  return page.getByTestId("kanban-board").getByTestId(`kanban-column-${stepId}`);
}

/** Rendered (display-order) task ids inside a column. */
async function columnTaskIds(page: Page, stepId: string): Promise<string[]> {
  return activeColumn(page, stepId)
    .locator("[data-testid^='task-card-']:not([data-testid='task-card-title'])")
    .evaluateAll((els: Element[]) =>
      els.map((e) => (e.getAttribute("data-testid") ?? "").replace("task-card-", "")),
    );
}

/** Task ids of the selected (primary-ring) cards in a column. */
async function columnSelectedIds(page: Page, stepId: string): Promise<string[]> {
  return activeColumn(page, stepId)
    .locator("[data-testid^='task-card-']:not([data-testid='task-card-title'])")
    .evaluateAll((els: Element[]) =>
      els
        .filter((e) => e.className.includes("ring-primary"))
        .map((e) => (e.getAttribute("data-testid") ?? "").replace("task-card-", "")),
    );
}

test.describe("Kanban cmd/shift multi-select", () => {
  test("cmd-click enters multi-select and shows the toolbar without the toggle", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Cmd Enter Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.multiSelectToolbar).not.toBeVisible();

    await kanban.cmdClickCard(task.id);

    await expect(kanban.multiSelectToolbar).toBeVisible();
    await expect(kanban.multiSelectToolbar).toContainText("1 selected");
    await kanban.expectCardSelected(task.id);
  });

  test("cmd-click toggles individual cards in and out of the selection", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const t1 = await apiClient.createTask(seedData.workspaceId, "Cmd Toggle 1", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const t2 = await apiClient.createTask(seedData.workspaceId, "Cmd Toggle 2", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.cmdClickCard(t1.id);
    await kanban.cmdClickCard(t2.id);
    await expect(kanban.multiSelectToolbar).toContainText("2 selected");

    // Cmd-click an already-selected card removes it from the selection.
    await kanban.cmdClickCard(t1.id);
    await expect(kanban.multiSelectToolbar).toContainText("1 selected");
    await kanban.expectCardSelected(t1.id, false);
    await kanban.expectCardSelected(t2.id, true);
  });

  test("shift-click selects a contiguous range within the column", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Four tasks in the same column; create sequentially for a stable order.
    for (const title of ["Range A", "Range B", "Range C", "Range D"]) {
      await apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      });
    }
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle("Range A")).toBeVisible({ timeout: 10_000 });

    const order = await columnTaskIds(testPage, seedData.startStepId);
    expect(order.length).toBeGreaterThanOrEqual(4);

    // Anchor on the first card, shift-click the third → first three selected.
    await kanban.cmdClickCard(order[0]);
    await kanban.shiftClickCard(order[2]);

    await expect(kanban.multiSelectToolbar).toContainText("3 selected");
    const selected = await columnSelectedIds(testPage, seedData.startStepId);
    expect(selected.sort()).toEqual(order.slice(0, 3).sort());
    await kanban.expectCardSelected(order[3], false);
  });

  test("shift-click does not extend the range across columns", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const otherStep = seedData.steps.find((s) => s.id !== seedData.startStepId);
    test.skip(!otherStep, "Workflow needs at least two steps");

    const a1 = await apiClient.createTask(seedData.workspaceId, "Col A One", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Col A Two", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const b1 = await apiClient.createTask(seedData.workspaceId, "Col B One", {
      workflow_id: seedData.workflowId,
      workflow_step_id: otherStep!.id,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(b1.id)).toBeVisible({ timeout: 10_000 });

    // Anchor in column A, shift-click into column B. The anchor isn't in B's
    // column order, so the range falls back to union-with-previous and re-anchors
    // to the clicked card — A stays selected, B is added, A's column isn't spanned.
    await kanban.cmdClickCard(a1.id);
    await kanban.shiftClickCard(b1.id);

    await expect(kanban.multiSelectToolbar).toContainText("2 selected");
    await kanban.expectCardSelected(a1.id, true);
    await kanban.expectCardSelected(b1.id, true);
  });

  test("plain click toggles selection while in multi-select mode", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const t1 = await apiClient.createTask(seedData.workspaceId, "Plain Toggle 1", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const t2 = await apiClient.createTask(seedData.workspaceId, "Plain Toggle 2", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Enter mode via cmd-click, then plain-click another card to add it.
    await kanban.cmdClickCard(t1.id);
    await kanban.plainClickCard(t2.id);
    await expect(kanban.multiSelectToolbar).toContainText("2 selected");
    // Still on the board — a plain in-mode click must not navigate away.
    await expect(kanban.board).toBeVisible();

    // Plain-click the same card again toggles it back out.
    await kanban.plainClickCard(t2.id);
    await expect(kanban.multiSelectToolbar).toContainText("1 selected");
  });

  test("the multi-select toggle button still works alongside cmd-click", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Toggle Coexist", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(task.id)).toBeVisible({ timeout: 10_000 });

    // Enter multi-select via the toggle button, then a Cmd-click must still
    // select a card (the two entry paths coexist).
    await kanban.multiSelectToggle.first().click();
    await expect(testPage.locator('[data-multi-select-active="true"]').first()).toBeVisible();

    await kanban.cmdClickCard(task.id);
    await kanban.expectCardSelected(task.id, true);
    await expect(kanban.multiSelectToolbar).toContainText("1 selected");
  });
});
