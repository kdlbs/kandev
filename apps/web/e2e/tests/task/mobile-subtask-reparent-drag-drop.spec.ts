import { type CDPSession, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import { settledBoundingBox } from "../../helpers/settled-box";

/**
 * Touch-drags a task row's handle onto a nest drop zone via CDP touch events.
 * dnd-kit's TouchSensor needs the 250ms delay to elapse and then >5px of
 * travel before the drag activates; the nest zone renders only once the drag
 * is live, so its box is resolved mid-drag. The sheet scrolls, so both the
 * source handle and the target zone are scrolled into view before their
 * coordinates are read.
 */
async function touchDragRowToNestZone(
  page: Page,
  cdp: CDPSession,
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

  test("long-press opens the context menu without dragging the row", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const placement = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const parent = await apiClient.createTask(
      seedData.workspaceId,
      "Menu long-press parent",
      placement,
    );
    const child = await apiClient.createTask(
      seedData.workspaceId,
      "Menu long-press child",
      placement,
    );

    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").click();

    const taskSheet = testPage.getByRole("dialog", { name: "Tasks" });
    const taskList = taskSheet.getByTestId("mobile-task-switcher-list");
    // The rendered order is the DOM order of the blocks inside the list.
    // Bounding-box comparisons are unreliable here: content-visibility rows
    // settle lazily, and a hidden desktop-sidebar copy of the tree duplicates
    // the test ids outside the sheet list.
    const orderOf = async () =>
      (
        await taskList
          .locator('[data-testid="sortable-task-block"]')
          .evaluateAll((els) => els.map((el) => el.getAttribute("data-task-id")))
      ).filter((id) => id === parent.id || id === child.id);
    // Wait until both rows are in the DOM: an empty before/after pair would
    // make the order assertion vacuous.
    await expect
      .poll(async () => await orderOf(), { timeout: 10_000 })
      .toEqual(expect.arrayContaining([parent.id, child.id]));
    const beforeOrder = await orderOf();
    expect(beforeOrder).toHaveLength(2);

    // Long-press the row: the TouchSensor arms at 250ms and the context menu
    // opens at ~700ms; opening the menu must cancel that drag.
    const cdp = await testPage.context().newCDPSession(testPage);
    const handle = taskList
      .locator(`[data-testid="sortable-task-block"][data-task-id="${child.id}"]`)
      .getByTestId("sortable-task-handle");
    await handle.scrollIntoViewIfNeeded();
    // Sheet scroll and Radix open animation settle on their own timers before
    // the handle box is measured.
    await dwell(testPage, 100, "library-timer", "sheet scroll settles before measuring the handle");
    const handleBox = await handle.boundingBox();
    if (!handleBox) throw new Error("drag source handle has no bounding box");
    const startX = handleBox.x + handleBox.width / 2;
    const startY = handleBox.y + handleBox.height / 2;
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x: startX, y: startY }],
    });
    // Hold past the dnd-kit TouchSensor arming (250ms) and the long-press
    // contextmenu (~700ms) so the menu opens while the drag is live; the menu
    // must then cancel it.
    await dwell(
      testPage,
      1200,
      "library-timer",
      "long-press hold outlasts the 250ms sensor and ~700ms menu",
    );
    // Scope to the menu this gesture opened: another mounted menu must not
    // satisfy the assertion.
    const openMenu = testPage.getByRole("menu");
    await expect(openMenu.getByRole("menuitem", { name: /color/i })).toBeVisible();
    // The long-press drag was cancelled when the menu opened: no nest zones
    // render and the source row is not dimmed.
    await expect(taskList.getByTestId("nest-drop-zone")).toHaveCount(0);
    await expect(handle).not.toHaveCSS("opacity", "0.5");

    // Keep the finger down and drag inside the open menu: the row must not
    // move or reorder. Aim at an inert point in the menu content — Radix
    // items click on pointerup when they did not receive the pointerdown, so
    // ending on an item could run its action. Scan the content (p-1 padding)
    // for a point that is not over a menu item.
    const menuContent = testPage.locator('[data-slot="context-menu-content"]');
    await expect(menuContent).toBeVisible();
    const inertPoint = await testPage.evaluate(() => {
      const menu = document.querySelector('[data-slot="context-menu-content"]');
      if (!menu) return null;
      const r = menu.getBoundingClientRect();
      for (let dy = 1; dy <= 8; dy += 1) {
        for (let dx = 1; dx <= 8; dx += 1) {
          const el = document.elementFromPoint(r.x + dx, r.y + dy);
          if (el && !el.closest('[role="menuitem"]')) {
            return { x: r.x + dx, y: r.y + dy };
          }
        }
      }
      return null;
    });
    if (!inertPoint) throw new Error("no inert point inside the context menu content");
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x: inertPoint.x, y: inertPoint.y }],
    });
    // Let dnd-kit's sensor process the move before the touch ends.
    await dwell(testPage, 100, "library-timer", "sensor move processing settles before touch end");
    await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });

    // The gesture ended inside the menu: no item action ran, the menu is
    // still open, and neither task was reordered or removed.
    await expect(menuContent).toBeVisible();
    await expect.poll(async () => await orderOf()).toEqual(beforeOrder);
    await expect.poll(async () => (await apiClient.getTask(child.id)).parent_id ?? "").toBe("");
  });
});
