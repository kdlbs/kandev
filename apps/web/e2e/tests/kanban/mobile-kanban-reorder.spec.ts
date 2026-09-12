import { type CDPSession, type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { settledBoundingBox } from "../../helpers/settled-box";
import { dwell, waitForHttp } from "../../helpers/causal-waits";

const REORDER_PATH = /\/workflow-steps\/.+\/tasks\/reorder$/;

/**
 * Touch-drags `fromCard` onto `toCard`'s row via CDP touch events. dnd-kit's
 * TouchSensor arms a 250ms activation timer on touchstart and only starts the
 * drag if the touch is still held when it fires and then moves >5px
 * (REQ-TASKS-KANBAN-TASK-REORDERING-001.6). Cards don't live-reorder mid-drag
 * (only the insertion indicator does), so both boxes are stable to read
 * up front.
 */
async function touchDragCardOntoCard(
  page: Page,
  cdp: CDPSession,
  fromCard: Locator,
  toCard: Locator,
) {
  const from = await settledBoundingBox(fromCard);
  const to = await settledBoundingBox(toCard);
  const startX = from.x + from.width / 2;
  const startY = from.y + from.height / 2;
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x: startX, y: startY }],
  });
  await dwell(
    page,
    350,
    "library-timer",
    "dnd-kit's TouchSensor arms a 250ms activation timer on touchStart and only starts the drag if the touch is still held when it fires",
  );
  const endX = to.x + to.width / 2;
  const endY = to.y + to.height / 2;
  for (let i = 1; i <= 10; i++) {
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [
        { x: startX + ((endX - startX) * i) / 10, y: startY + ((endY - startY) * i) / 10 },
      ],
    });
  }
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
}

async function columnOrder(page: Page): Promise<string[]> {
  return page
    .getByTestId("mobile-kanban-layout")
    .locator('[data-testid="task-card-title"]')
    .allTextContents();
}

test.describe("Mobile kanban card reordering", () => {
  test("touch drag reorders cards within a column and persists across reload", async ({
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
    const taskA = await apiClient.createTask(seedData.workspaceId, "Mobile reorder A", placement);
    const taskB = await apiClient.createTask(seedData.workspaceId, "Mobile reorder B", placement);
    const taskC = await apiClient.createTask(seedData.workspaceId, "Mobile reorder C", placement);

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await expect(mobile.taskCard(taskC.id)).toBeVisible();
    await expect
      .poll(() => columnOrder(testPage))
      .toEqual(["Mobile reorder A", "Mobile reorder B", "Mobile reorder C"]);

    const cdp = await testPage.context().newCDPSession(testPage);
    const reorderResponse = waitForHttp(testPage, "PUT", REORDER_PATH);
    await touchDragCardOntoCard(
      testPage,
      cdp,
      mobile.taskCard(taskC.id),
      mobile.taskCard(taskA.id),
    );
    await reorderResponse;

    await expect
      .poll(() => columnOrder(testPage))
      .toEqual(["Mobile reorder C", "Mobile reorder A", "Mobile reorder B"]);
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
    await mobile.board.waitFor({ state: "visible" });
    await expect
      .poll(() => columnOrder(testPage))
      .toEqual(["Mobile reorder C", "Mobile reorder A", "Mobile reorder B"]);
  });
});
