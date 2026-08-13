import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { settledBoundingBox } from "../../helpers/settled-box";
import { dwell } from "../../helpers/causal-waits";

/**
 * Touch-drags a task row's handle onto a nest drop zone via CDP touch events.
 * dnd-kit's TouchSensor needs the 250ms delay to elapse and then >5px of
 * travel before the drag activates; the nest zone renders only once the drag
 * is live, so its box is resolved mid-drag. The sheet scrolls, so both the
 * source handle and the target zone are scrolled into view before their
 * coordinates are read.
 */
async function touchDragRowToNestZone(
  page: import("@playwright/test").Page,
  cdp: import("@playwright/test").CDPSession,
  handleLocator: ReturnType<Page["locator"]>,
  zoneLocator: ReturnType<Page["locator"]>,
) {
  const handleBox = await settledBoundingBox(handleLocator);
  const startX = handleBox.x + handleBox.width / 2;
  const startY = handleBox.y + handleBox.height / 2;
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x: startX, y: startY }],
  });
  await dwell(
    page,
    350,
    "library-timer",
    "dnd-kit's TouchSensor arms a 250ms activation timer on touchStart and only starts the drag if the touch is still held when it fires; nothing renders while that timer runs",
  );
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchMove",
    touchPoints: [{ x: startX + 14, y: startY }],
  });
  // The zone appearing is the signal that the drag went live, and asserting
  // it is itself the wait -- no settle needed before or after.
  await expect(zoneLocator).toBeVisible();
  const to = await settledBoundingBox(zoneLocator);
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

test.describe("Mobile subtask re-parenting by drag and drop", () => {
  test("re-parents a subtask onto another root with a touch drag", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const placement = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const oldParent = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile reparent old parent",
      placement,
    );
    const newParent = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile reparent new parent",
      placement,
    );
    const child = await apiClient.createTask(seedData.workspaceId, "Mobile reparent child", {
      ...placement,
      parent_id: oldParent.id,
      workspace_mode: "inherit_parent",
    });

    await testPage.goto(`/t/${newParent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").click();

    const taskSheet = testPage.getByRole("dialog", { name: "Tasks" });
    const taskBlock = (taskId: string) =>
      taskSheet.locator(`[data-testid="sortable-task-block"][data-task-id="${taskId}"]`);
    await expect(taskBlock(child.id)).toHaveAttribute("data-depth", "1");

    const cdp = await testPage.context().newCDPSession(testPage);
    await touchDragRowToNestZone(
      testPage,
      cdp,
      taskBlock(child.id).getByTestId("sortable-task-handle").first(),
      taskBlock(newParent.id).getByTestId("nest-drop-zone"),
    );

    await expect(taskBlock(newParent.id).locator(`[data-task-id="${child.id}"]`)).toHaveCount(1, {
      timeout: 10_000,
    });

    await expect
      .poll(async () => {
        const task = await apiClient.getTask(child.id);
        const workspace = task.metadata?.workspace as { mode?: string } | undefined;
        return { parentId: task.parent_id ?? "", workspaceMode: workspace?.mode };
      })
      .toEqual({ parentId: newParent.id, workspaceMode: "shared_group" });
  });
});
