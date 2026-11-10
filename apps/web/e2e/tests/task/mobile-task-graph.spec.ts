// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { expect, test } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";

async function expectTouchTarget(locator: import("@playwright/test").Locator, label: string) {
  const box = await locator.boundingBox();
  expect(box, `${label} must have geometry`).not.toBeNull();
  expect(
    Math.min(box?.width ?? 0, box?.height ?? 0),
    `${label} touch target`,
  ).toBeGreaterThanOrEqual(44);
}

async function expectContainedBy(
  control: import("@playwright/test").Locator,
  group: import("@playwright/test").Locator,
  label: string,
) {
  const [controlBox, groupBox] = await Promise.all([control.boundingBox(), group.boundingBox()]);
  expect(controlBox, `${label}: control has geometry`).not.toBeNull();
  expect(groupBox, `${label}: group has geometry`).not.toBeNull();
  if (!controlBox || !groupBox) return;
  expect(controlBox.x, `${label}: left edge`).toBeGreaterThanOrEqual(groupBox.x - 1);
  expect(controlBox.x + controlBox.width, `${label}: right edge`).toBeLessThanOrEqual(
    groupBox.x + groupBox.width + 1,
  );
  expect(controlBox.y, `${label}: top edge`).toBeGreaterThanOrEqual(groupBox.y - 1);
  expect(controlBox.y + controlBox.height, `${label}: bottom edge`).toBeLessThanOrEqual(
    groupBox.y + groupBox.height + 1,
  );
}

test("mobile task graph opens full screen with touch-sized controls", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const base = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  };
  const parent = await apiClient.createTask(seedData.workspaceId, "Mobile graph parent", base);
  const a = await apiClient.createTask(seedData.workspaceId, "Mobile graph A", {
    ...base,
    parent_id: parent.id,
  });
  const b = await apiClient.createTask(seedData.workspaceId, "Mobile graph B", {
    ...base,
    parent_id: parent.id,
    blocked_by: [a.id],
    start_when_unblocked: false,
  });
  const loner = await apiClient.createTask(seedData.workspaceId, "Mobile graph loner", {
    ...base,
    parent_id: parent.id,
  });

  await testPage.goto(`/t/${a.id}`);
  const bar = testPage.getByTestId("chat-status-bar");
  const controls = bar.getByTestId("chat-status-bar-right-controls");
  const button = controls.getByTestId("task-graph-button");
  await expect(button).toHaveCount(1);
  await expect(button).toHaveAccessibleName("Task graph");
  await expect(controls).toHaveCSS("flex-wrap", "wrap");
  await expect(bar).toHaveCSS("flex-wrap", "wrap");
  await expectContainedBy(button, controls, "phone graph action");
  await expectTouchTarget(button, "graph button");
  await assertNoDocumentHorizontalOverflow(testPage, "graph entry");
  await button.tap();

  const dialog = testPage.getByTestId("task-graph-dialog");
  await expect(dialog).toBeVisible();
  const viewport = testPage.viewportSize();
  // The dialog zooms in from 95%; poll until the open animation settles.
  await expect
    .poll(async () => Math.round((await dialog.boundingBox())?.width ?? 0))
    .toBe(viewport?.width ?? 0);
  await assertLocatorWithinViewportX(dialog, "graph sheet");
  await assertNoDocumentHorizontalOverflow(testPage, "graph sheet");

  await expectTouchTarget(dialog.getByTestId("task-graph-close"), "close");
  await expectTouchTarget(dialog.getByTestId("task-graph-mode-subtasks"), "subtasks mode");
  await expectTouchTarget(dialog.getByTestId("task-graph-mode-chain"), "chain mode");
  await expectTouchTarget(dialog.locator(".react-flow__controls-fitview"), "fit view");
  await expect(dialog.locator(".react-flow__minimap")).toHaveCount(0);

  // Long press emphasizes the chain without navigating.
  const aNode = testPage.getByTestId(`task-graph-node-${a.id}`);
  await aNode.dispatchEvent("pointerdown", { pointerType: "touch", isPrimary: true });
  await dwell(testPage, 600, "product-timer", "the 500 ms long-press timer publishes no event");
  await aNode.dispatchEvent("pointerup", { pointerType: "touch", isPrimary: true });
  await aNode.dispatchEvent("click");
  await expect(
    testPage.getByTestId(`task-graph-node-${loner.id}`).getByTestId("task-graph-node-card"),
  ).toHaveAttribute("data-dimmed", "true");
  await expect(testPage).toHaveURL(new RegExp(`/t/${a.id}$`));

  // The phone canvas is centred on the focused task; zoom out to reach its dependent.
  const zoomOut = dialog.locator(".react-flow__controls-zoomout");
  await zoomOut.tap();
  await zoomOut.tap();
  await testPage.getByTestId(`task-graph-node-${b.id}`).tap();
  await expect(testPage).toHaveURL(new RegExp(`/t/${b.id}$`));
  await expect(dialog).toHaveCount(0);
  await assertNoDocumentHorizontalOverflow(testPage, "after graph navigation");
});

test("narrow touch layout keeps the graph sheet full width and returns focus", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 700, height: 900 });
  const base = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  };
  const parent = await apiClient.createTask(seedData.workspaceId, "Narrow graph parent", base);
  const child = await apiClient.createTask(seedData.workspaceId, "Narrow graph child", {
    ...base,
    parent_id: parent.id,
  });

  await testPage.goto(`/t/${child.id}`);
  const controls = testPage.getByTestId("chat-status-bar-right-controls");
  const button = controls.getByTestId("task-graph-button");
  await expect(button).toHaveCount(1);
  await expectContainedBy(button, controls, "narrow graph action");
  await button.focus();
  await button.tap();

  const dialog = testPage.getByTestId("task-graph-dialog");
  await expect(dialog).toBeVisible();
  await expect.poll(async () => Math.round((await dialog.boundingBox())?.width ?? 0)).toBe(700);
  await dialog.getByTestId("task-graph-close").tap();
  await expect(dialog).toHaveCount(0);
  await expect(button).toBeFocused();
});

test.describe("coarse-pointer tablet", () => {
  test.use({ hasTouch: true, viewport: { width: 900, height: 1000 } });

  test("keeps the graph action touch-sized in the right utility group", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const base = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    };
    const parent = await apiClient.createTask(seedData.workspaceId, "Tablet graph parent", base);
    const child = await apiClient.createTask(seedData.workspaceId, "Tablet graph child", {
      ...base,
      parent_id: parent.id,
    });

    await testPage.goto(`/t/${child.id}`);
    const controls = testPage.getByTestId("chat-status-bar-right-controls");
    const button = controls.getByTestId("task-graph-button");
    await expect(button).toHaveCount(1);
    await expect(button).toHaveAccessibleName("Task graph");
    await expectContainedBy(button, controls, "tablet graph action");
    await expectTouchTarget(button, "tablet graph button");
    await assertNoDocumentHorizontalOverflow(testPage, "tablet graph entry");

    await button.tap();
    await expect(testPage.getByTestId("task-graph-dialog")).toBeVisible();
  });
});
