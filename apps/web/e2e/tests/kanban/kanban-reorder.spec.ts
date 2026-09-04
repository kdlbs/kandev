import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { settledBoundingBox } from "../../helpers/settled-box";
import { waitForHttp } from "../../helpers/causal-waits";

const REORDER_PATH = /\/workflow-steps\/.+\/tasks\/reorder$/;

/**
 * Drags `fromCard` (the whole card is the drag source — REQ-TASKS-KANBAN-TASK-REORDERING-001.6,
 * no grab handle) and drops it directly on `toCard`'s row. dnd-kit's
 * `pointerWithin` collision detection resolves `over.id` to the smaller,
 * nested card droppable rather than the enclosing column, so landing the
 * pointer on a card's center targets that exact card.
 */
async function dragCardOntoCard(page: Page, fromCard: Locator, toCard: Locator) {
  const from = await settledBoundingBox(fromCard);
  const startX = from.x + from.width / 2;
  const startY = from.y + from.height / 2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  // Exceed the 8px PointerSensor activation distance so the drag starts.
  await page.mouse.move(startX + 12, startY, { steps: 3 });
  const to = await settledBoundingBox(toCard);
  await page.mouse.move(to.x + to.width / 2, to.y + to.height / 2, { steps: 12 });
  await page.mouse.up();
}

/** Titles of the cards rendered in a column, top to bottom. */
async function columnOrder(kanban: KanbanPage, stepId: string): Promise<string[]> {
  return kanban.columnByStepId(stepId).locator('[data-testid="task-card-title"]').allTextContents();
}

test.describe("Kanban card reordering", () => {
  test("pointer drag reorders cards within a column and persists across reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const placement = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    // Created sequentially: arrival position is assigned max+1 per create, so
    // this fixes the starting order A, B, C (position ascending, AC.1).
    const taskA = await apiClient.createTask(seedData.workspaceId, "Reorder drag A", placement);
    const taskB = await apiClient.createTask(seedData.workspaceId, "Reorder drag B", placement);
    const taskC = await apiClient.createTask(seedData.workspaceId, "Reorder drag C", placement);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.taskCard(taskC.id)).toBeVisible();
    await expect
      .poll(() => columnOrder(kanban, seedData.startStepId))
      .toEqual(["Reorder drag A", "Reorder drag B", "Reorder drag C"]);

    const reorderResponse = waitForHttp(testPage, "PUT", REORDER_PATH);
    await dragCardOntoCard(testPage, kanban.taskCard(taskC.id), kanban.taskCard(taskA.id));
    await reorderResponse;

    await expect
      .poll(() => columnOrder(kanban, seedData.startStepId))
      .toEqual(["Reorder drag C", "Reorder drag A", "Reorder drag B"]);
    await expect
      .poll(async () => {
        const [a, b, c] = await Promise.all([
          apiClient.getTask(taskA.id),
          apiClient.getTask(taskB.id),
          apiClient.getTask(taskC.id),
        ]);
        return { a: a.position, b: b.position, c: c.position };
      })
      .toEqual({ c: 0, a: 1, b: 2 });

    await testPage.reload();
    await kanban.board.waitFor({ state: "visible" });
    await expect
      .poll(() => columnOrder(kanban, seedData.startStepId))
      .toEqual(["Reorder drag C", "Reorder drag A", "Reorder drag B"]);
  });

  test("keyboard reorder moves a card and persists the new order", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const placement = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const taskD = await apiClient.createTask(seedData.workspaceId, "Reorder keys D", placement);
    const taskE = await apiClient.createTask(seedData.workspaceId, "Reorder keys E", placement);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const cardD = kanban.taskCard(taskD.id);
    await expect(cardD).toBeVisible();
    await expect
      .poll(() => columnOrder(kanban, seedData.startStepId))
      .toEqual(["Reorder keys D", "Reorder keys E"]);

    // Space picks the card up: announce its place in the band (position 1 of 2).
    await cardD.focus();
    await testPage.keyboard.press(" ");
    await expect(cardD).toHaveAttribute("aria-grabbed", "true");
    const announcement = testPage.getByTestId("kanban-reorder-announcement");
    await expect(announcement).toContainText("position 1 of 2", { timeout: 5_000 });

    // Arrow Down moves it one place — announce position 2 of 2.
    await testPage.keyboard.press("ArrowDown");
    await expect(announcement).toContainText("position 2 of 2", { timeout: 5_000 });

    const reorderResponse = waitForHttp(testPage, "PUT", REORDER_PATH);
    await testPage.keyboard.press(" ");
    await reorderResponse;
    await expect(cardD).not.toHaveAttribute("aria-grabbed", "true");

    await expect
      .poll(() => columnOrder(kanban, seedData.startStepId))
      .toEqual(["Reorder keys E", "Reorder keys D"]);
    await expect
      .poll(async () => {
        const [d, e] = await Promise.all([
          apiClient.getTask(taskD.id),
          apiClient.getTask(taskE.id),
        ]);
        return { d: d.position, e: e.position };
      })
      .toEqual({ e: 0, d: 1 });
  });
});
