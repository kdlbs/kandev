import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { settledBoundingBox } from "../../helpers/settled-box";

/**
 * Drags a sidebar task row's handle onto a nest drop zone using pointer
 * events. dnd-kit's MouseSensor needs >8px of travel before the drag
 * activates, and the nest zone only renders once the drag is live, so the
 * target box is resolved mid-drag.
 */
async function dragRowToNestZone(
  page: Page,
  handleLocator: ReturnType<Page["locator"]>,
  zoneLocator: ReturnType<Page["locator"]>,
) {
  const from = await settledBoundingBox(handleLocator);
  const startX = from.x + from.width / 2;
  const startY = from.y + from.height / 2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  // Exceed the 8px activation distance so the drag starts.
  await page.mouse.move(startX + 24, startY, { steps: 3 });
  // The zone now exists; aim the pointer at its center.
  await expect(zoneLocator).toBeVisible();
  const to = await zoneLocator.boundingBox();
  if (!to) throw new Error("nest drop zone has no bounding box");
  await page.mouse.move(to.x + to.width / 2, to.y + to.height / 2, { steps: 12 });
  await page.mouse.up();
}

test.describe("Subtask re-parenting by drag and drop", () => {
  test("re-parents an inherited-workspace subtask onto another root", async ({
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
      "Reparent old parent",
      placement,
    );
    const newParent = await apiClient.createTask(
      seedData.workspaceId,
      "Reparent new parent",
      placement,
    );
    const child = await apiClient.createTask(seedData.workspaceId, "Reparent child", {
      ...placement,
      parent_id: oldParent.id,
      workspace_mode: "inherit_parent",
    });

    await testPage.goto(`/t/${newParent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const taskBlock = (taskId: string) =>
      session.sidebar.locator(`[data-testid="sortable-task-block"][data-task-id="${taskId}"]`);
    await expect(taskBlock(child.id)).toHaveAttribute("data-depth", "1");

    const childHandle = taskBlock(child.id).getByTestId("sortable-task-handle");
    const newParentZone = taskBlock(newParent.id).getByTestId("nest-drop-zone");
    await dragRowToNestZone(testPage, childHandle, newParentZone);

    // The tree moves live: child now sits inside the new parent's block.
    await expect(taskBlock(newParent.id).locator(`[data-task-id="${child.id}"]`)).toHaveCount(1, {
      timeout: 10_000,
    });
    await expect(taskBlock(child.id)).toHaveAttribute("data-depth", "1");

    // The composite semantics (un-nest then nest) apply: parent re-pointed and
    // the inherited workspace mode normalized to shared_group.
    await expect
      .poll(async () => {
        const task = await apiClient.getTask(child.id);
        const workspace = task.metadata?.workspace as { mode?: string } | undefined;
        return { parentId: task.parent_id ?? "", workspaceMode: workspace?.mode };
      })
      .toEqual({ parentId: newParent.id, workspaceMode: "shared_group" });
  });

  test("nests a root task under another root", async ({ testPage, apiClient, seedData }) => {
    const placement = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const parent = await apiClient.createTask(seedData.workspaceId, "Nest root parent", placement);
    const child = await apiClient.createTask(seedData.workspaceId, "Nest root child", placement);

    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const taskBlock = (taskId: string) =>
      session.sidebar.locator(`[data-testid="sortable-task-block"][data-task-id="${taskId}"]`);
    await expect(taskBlock(child.id)).toHaveAttribute("data-depth", "0");

    await dragRowToNestZone(
      testPage,
      taskBlock(child.id).getByTestId("sortable-task-handle"),
      taskBlock(parent.id).getByTestId("nest-drop-zone"),
    );

    await expect(taskBlock(parent.id).locator(`[data-task-id="${child.id}"]`)).toHaveCount(1, {
      timeout: 10_000,
    });
    await expect.poll(async () => (await apiClient.getTask(child.id)).parent_id).toBe(parent.id);
  });

  test("offers no nest zones for a task that already has children", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const placement = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const parentWithChild = await apiClient.createTask(
      seedData.workspaceId,
      "Nestless parent",
      placement,
    );
    await apiClient.createTask(seedData.workspaceId, "Nestless child", {
      ...placement,
      parent_id: parentWithChild.id,
    });

    await testPage.goto(`/t/${parentWithChild.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const taskBlock = (taskId: string) =>
      session.sidebar.locator(`[data-testid="sortable-task-block"][data-task-id="${taskId}"]`);

    // Start a drag of the parent-with-child; no row may offer a nest zone.
    const handle = taskBlock(parentWithChild.id).getByTestId("sortable-task-handle").first();
    const box = await handle.boundingBox();
    if (!box) throw new Error("drag source handle has no bounding box");
    await testPage.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await testPage.mouse.down();
    await testPage.mouse.move(box.x + box.width / 2 + 24, box.y + box.height / 2, { steps: 3 });
    await expect(session.sidebar.getByTestId("nest-drop-zone")).toHaveCount(0);
    await testPage.keyboard.press("Escape");

    await expect
      .poll(async () => (await apiClient.getTask(parentWithChild.id)).parent_id ?? "")
      .toBe("");
  });

  test("ignores a drop on a subtask row (not a valid target)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const placement = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const parentA = await apiClient.createTask(seedData.workspaceId, "Invalid A parent", placement);
    const parentB = await apiClient.createTask(seedData.workspaceId, "Invalid B parent", placement);
    const childA = await apiClient.createTask(seedData.workspaceId, "Invalid A child", {
      ...placement,
      parent_id: parentA.id,
    });
    const childB = await apiClient.createTask(seedData.workspaceId, "Invalid B child", {
      ...placement,
      parent_id: parentB.id,
    });

    await testPage.goto(`/t/${parentA.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const taskBlock = (taskId: string) =>
      session.sidebar.locator(`[data-testid="sortable-task-block"][data-task-id="${taskId}"]`);

    // Drag childA toward childB: childB (a subtask) is not a valid target, so
    // its row offers no nest zone (parentB's row does — it is a valid root
    // candidate), and a drop on childB's row is cross-level: no-op.
    const fromBox = await taskBlock(childA.id).getByTestId("sortable-task-handle").boundingBox();
    if (!fromBox) throw new Error("drag source handle has no bounding box");
    await testPage.mouse.move(fromBox.x + fromBox.width / 2, fromBox.y + fromBox.height / 2);
    await testPage.mouse.down();
    await testPage.mouse.move(fromBox.x + fromBox.width / 2 + 24, fromBox.y + fromBox.height / 2, {
      steps: 3,
    });
    const zoneFor = (taskId: string) =>
      session.sidebar.locator(`[data-testid="nest-drop-zone"][data-task-id="${taskId}"]`);
    await expect(zoneFor(childB.id)).toHaveCount(0);
    await expect(zoneFor(parentB.id)).toHaveCount(1);
    const toBox = await taskBlock(childB.id).boundingBox();
    if (!toBox) throw new Error("target row has no bounding box");
    await testPage.mouse.move(toBox.x + toBox.width / 2, toBox.y + toBox.height / 2, { steps: 12 });
    await testPage.mouse.up();

    await expect.poll(async () => (await apiClient.getTask(childA.id)).parent_id).toBe(parentA.id);
  });

  test("sibling reorder still works inside a parent (regression)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const placement = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const parent = await apiClient.createTask(seedData.workspaceId, "Reorder parent", placement);
    const first = await apiClient.createTask(seedData.workspaceId, "Reorder first child", {
      ...placement,
      parent_id: parent.id,
    });
    const second = await apiClient.createTask(seedData.workspaceId, "Reorder second child", {
      ...placement,
      parent_id: parent.id,
    });

    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const taskBlock = (taskId: string) =>
      session.sidebar.locator(`[data-testid="sortable-task-block"][data-task-id="${taskId}"]`);

    // Children render newest-first by default, so resolve the rendered order
    // and drag the lower sibling onto the upper one's row (subtasks are not
    // nest targets, so only the reorder path can fire).
    const firstY = (await taskBlock(first.id).boundingBox())?.y ?? 0;
    const secondY = (await taskBlock(second.id).boundingBox())?.y ?? 0;
    const [upperId, lowerId] = firstY < secondY ? [first.id, second.id] : [second.id, first.id];
    const upperBox = await taskBlock(upperId).boundingBox();
    if (!upperBox) throw new Error("rows have no bounding boxes");

    const fromBox = await taskBlock(lowerId).getByTestId("sortable-task-handle").boundingBox();
    if (!fromBox) throw new Error("drag source handle has no bounding box");
    await testPage.mouse.move(fromBox.x + fromBox.width / 2, fromBox.y + fromBox.height / 2);
    await testPage.mouse.down();
    await testPage.mouse.move(fromBox.x + fromBox.width / 2 + 24, fromBox.y + fromBox.height / 2, {
      steps: 3,
    });
    await expect(session.sidebar.getByTestId("nest-drop-zone")).toHaveCount(0);
    await testPage.mouse.move(upperBox.x + upperBox.width / 2, upperBox.y + upperBox.height / 2, {
      steps: 12,
    });
    await testPage.mouse.up();

    await expect
      .poll(async () => {
        const upperY = (await taskBlock(upperId).boundingBox())?.y ?? 0;
        const lowerY = (await taskBlock(lowerId).boundingBox())?.y ?? 0;
        return lowerY < upperY;
      })
      .toBe(true);
  });

  test("starting a drag inside the row context menu does not drag the row", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const placement = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const parent = await apiClient.createTask(seedData.workspaceId, "Menu drag parent", placement);
    const child = await apiClient.createTask(seedData.workspaceId, "Menu drag child", placement);

    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const taskBlock = (taskId: string) =>
      session.sidebar.locator(`[data-testid="sortable-task-block"][data-task-id="${taskId}"]`);
    const before = {
      parentY: (await taskBlock(parent.id).boundingBox())?.y ?? 0,
      childY: (await taskBlock(child.id).boundingBox())?.y ?? 0,
    };

    // Right-click opens the context menu; the MouseSensor ignores right
    // buttons, so opening the menu alone must not start a drag.
    await taskBlock(child.id).getByTestId("sortable-task-handle").click({ button: "right" });
    const colorItem = testPage.getByRole("menuitem", { name: /color/i });
    await expect(colorItem).toBeVisible();

    // Press and move inside the menu. The menu's pointer events must not
    // reach the drag handle's sensor: a live drag would dim the source row
    // and render nest drop zones on candidate rows.
    const colorBox = await colorItem.boundingBox();
    if (!colorBox) throw new Error("context menu color item has no bounding box");
    await testPage.mouse.move(colorBox.x + colorBox.width / 2, colorBox.y + colorBox.height / 2);
    await testPage.mouse.down();
    await testPage.mouse.move(
      colorBox.x + colorBox.width / 2 + 24,
      colorBox.y + colorBox.height / 2,
      {
        steps: 3,
      },
    );
    await expect(session.sidebar.getByTestId("nest-drop-zone")).toHaveCount(0);
    await expect(taskBlock(child.id)).not.toHaveCSS("opacity", "0.5");
    await testPage.mouse.up();

    // The row never moved: both rows keep their vertical order.
    await expect
      .poll(async () => {
        return {
          parentY: (await taskBlock(parent.id).boundingBox())?.y ?? 0,
          childY: (await taskBlock(child.id).boundingBox())?.y ?? 0,
        };
      })
      .toEqual(before);
  });
});
