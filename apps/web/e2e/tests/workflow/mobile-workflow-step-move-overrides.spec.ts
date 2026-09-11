import { expect, test } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { dwell } from "../../helpers/causal-waits";
import {
  expectMoveInstructionsDelivered,
  fillMoveOverrides,
  MOVE_INSTRUCTIONS,
  seedMoveOverrideFixture,
  TARGET_STEP_MESSAGE,
  waitForMoveRequest,
} from "./workflow-step-move-overrides-helpers";

async function longPress(page: Page, target: Locator): Promise<void> {
  await target.scrollIntoViewIfNeeded();
  const bounds = await target.boundingBox();
  if (!bounds) throw new Error("next-step button has no bounding box");

  const cdp = await page.context().newCDPSession(page);
  const x = bounds.x + bounds.width / 2;
  const y = bounds.y + bounds.height / 2;
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x, y }],
  });
  await dwell(page, 600, "library-timer", "workflow move long-press threshold settles");
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
}

async function expectMoveOptionsDrawer(page: Page): Promise<Locator> {
  const form = page.getByTestId("workflow-move-options");
  await expect(form).toBeVisible();

  const drawer = page.locator('[data-slot="drawer-content"]:visible');
  await expect(drawer).toBeVisible();
  await expect(drawer).toHaveCSS("transform", "none");
  const bounds = await drawer.boundingBox();
  const viewport = page.viewportSize();
  expect(bounds).not.toBeNull();
  expect(viewport).not.toBeNull();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewport!.width);
  expect(bounds!.y).toBeGreaterThanOrEqual(0);
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(viewport!.height);
  await expect(page.getByTestId("workflow-move-submit")).toHaveCSS("min-height", "44px");
  await assertNoDocumentHorizontalOverflow(page);
  return form;
}

async function expectTargetStep(
  apiClient: Parameters<typeof seedMoveOverrideFixture>[1],
  taskId: string,
  targetStepId: string,
): Promise<void> {
  await expect
    .poll(async () => (await apiClient.getTask(taskId)).workflow_step_id, { timeout: 15_000 })
    .toBe(targetStepId);
}

test("short-taps the existing mobile next-step button for a direct move", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Mobile Direct Move",
  );
  const nextStepButton = testPage.getByTestId("proceed-next-step");
  await expect(nextStepButton).toBeVisible({ timeout: 15_000 });

  const moveRequest = waitForMoveRequest(testPage, fixture.taskId);
  await nextStepButton.tap();

  expect((await moveRequest).postDataJSON()).toEqual({
    workflow_id: expect.any(String),
    workflow_step_id: fixture.targetStepId,
    position: 0,
  });
  await expectTargetStep(apiClient, fixture.taskId, fixture.targetStepId);
  await expect(
    fixture.session.chat.getByText(TARGET_STEP_MESSAGE, { exact: false }).first(),
  ).toBeVisible({ timeout: 30_000 });
});

test("long-presses the existing mobile next-step button for move options", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Mobile Long Press Move",
  );
  const nextStepButton = testPage.getByTestId("proceed-next-step");
  await expect(nextStepButton).toBeVisible({ timeout: 15_000 });
  await longPress(testPage, nextStepButton);

  await expectMoveOptionsDrawer(testPage);
  await fillMoveOverrides(testPage);
  const moveRequest = waitForMoveRequest(testPage, fixture.taskId);
  await testPage.getByTestId("workflow-move-submit").tap();

  expect((await moveRequest).postDataJSON()).toEqual({
    workflow_id: expect.any(String),
    workflow_step_id: fixture.targetStepId,
    position: 0,
    entry_options: {
      reset_context: true,
      instructions: MOVE_INSTRUCTIONS,
    },
  });
  await expectTargetStep(apiClient, fixture.taskId, fixture.targetStepId);
  await expectMoveInstructionsDelivered(fixture.session);
});

test("opens move options from the mobile task-switcher context menu", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Mobile Task Switcher Move",
  );
  await testPage.getByTestId("mobile-session-menu").tap();

  const taskSheet = testPage.getByRole("dialog", { name: "Tasks" });
  const taskRow = taskSheet
    .getByTestId("sidebar-task-item")
    .filter({ hasText: "Mobile Task Switcher Move Task" });
  await expect(taskRow).toBeVisible();
  await taskRow.getByRole("button", { name: "Task actions" }).tap();

  await testPage.getByTestId("task-context-move-to").tap();

  const targetStep = testPage.getByTestId(`task-context-step-${fixture.targetStepId}`);
  await expect(targetStep).toBeVisible();
  const targetStepBox = await targetStep.boundingBox();
  if (!targetStepBox) throw new Error("target step menu item is not measurable");
  // The step label remains the direct move action on touch. Tap its trailing chevron to open
  // the nested options menu instead.
  await targetStep.tap({
    position: { x: Math.max(1, targetStepBox.width - 12), y: targetStepBox.height / 2 },
  });
  const moveWithOptions = testPage.getByTestId(`task-context-step-options-${fixture.targetStepId}`);
  await expect(moveWithOptions).toBeVisible();
  await moveWithOptions.tap();

  await expect(taskSheet).toBeHidden();
  await expectMoveOptionsDrawer(testPage);
  await fillMoveOverrides(testPage);
  const moveRequest = waitForMoveRequest(testPage, fixture.taskId);
  await testPage.getByTestId("workflow-move-submit").tap();

  expect((await moveRequest).postDataJSON()).toEqual({
    workflow_id: expect.any(String),
    workflow_step_id: fixture.targetStepId,
    position: 0,
    entry_options: {
      reset_context: true,
      instructions: MOVE_INSTRUCTIONS,
    },
  });
  await expectTargetStep(apiClient, fixture.taskId, fixture.targetStepId);
  // Submitting a move from the task-switcher clears the move-options request but
  // leaves the switcher drawer itself open on top of the chat, so it intercepts
  // pointer events on the delivered instruction message. Dismiss it before
  // asserting the chat received the one-shot instructions.
  await testPage.keyboard.press("Escape");
  await expect(taskSheet).toBeHidden();
  await expectMoveInstructionsDelivered(fixture.session);
});
