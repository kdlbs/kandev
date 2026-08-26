import { type Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const COMPACT_TASK_TITLE = `Compact workflow navigation ${"W".repeat(90)}`;

function adjacentStep(
  steps: Array<{ id: string; position: number }>,
  currentStepId: string,
): { id: string; position: number } {
  const sorted = [...steps].sort((left, right) => left.position - right.position);
  const currentIndex = sorted.findIndex((step) => step.id === currentStepId);
  const target = sorted[currentIndex + 1] ?? sorted[currentIndex - 1];
  if (!target) throw new Error("compact workflow step test requires an adjacent target");
  return target;
}

async function waitForFiniteAnimations(locator: Locator): Promise<void> {
  await locator.evaluate(async (element) => {
    const animations = element.getAnimations().filter((animation) => {
      return animation.effect?.getComputedTiming().iterations !== Infinity;
    });
    await Promise.all(animations.map((animation) => animation.finished.catch(() => undefined)));
  });
}

test.describe("Compact task topbar workflow stepper", () => {
  test("opens ordered steps on hover and moves the task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.seedTask(seedData.workspaceId, COMPACT_TASK_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const targetStep = adjacentStep(seedData.steps, seedData.startStepId);

    await testPage.setViewportSize({ width: 900, height: 800 });
    await testPage.goto(`/t/${task.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const trigger = testPage.getByTestId("workflow-stepper-minimal");
    await expect(trigger).toBeVisible();
    await expect(trigger).toHaveAttribute("aria-haspopup", "dialog");
    await trigger.hover();

    const disclosure = testPage.getByTestId("workflow-step-disclosure");
    await expect(disclosure).toBeVisible();
    await trigger.focus();
    await expect(trigger).toBeFocused();
    await expect(disclosure).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(disclosure).toBeHidden();
    await expect(trigger).toBeFocused();
    await testPage.mouse.move(0, 0);
    await trigger.hover();
    await expect(disclosure).toBeVisible();
    await expect(disclosure.locator('[data-testid^="workflow-step-disclosure-row-"]')).toHaveCount(
      seedData.steps.length,
    );

    const moveButton = testPage.getByTestId(`workflow-step-disclosure-move-${targetStep.id}`);
    await expect(moveButton).toBeVisible();
    await moveButton.click();

    await expect
      .poll(async () => (await apiClient.getTask(task.task_id)).workflow_step_id, {
        timeout: 15_000,
      })
      .toBe(targetStep.id);
  });

  test("opens the same steps in a contained tablet touch drawer", async ({
    tabletTestPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.seedTask(seedData.workspaceId, COMPACT_TASK_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const targetStep = adjacentStep(seedData.steps, seedData.startStepId);

    await tabletTestPage.goto(`/t/${task.task_id}`);
    const session = new SessionPage(tabletTestPage);
    await session.waitForLoad();

    const trigger = tabletTestPage.getByTestId("workflow-stepper-minimal");
    await expect(trigger).toBeVisible();
    await trigger.tap();

    const drawer = tabletTestPage.getByRole("dialog", { name: "Move to" });
    await expect(drawer).toBeVisible();
    await waitForFiniteAnimations(drawer);
    const drawerBox = await drawer.boundingBox();
    expect(drawerBox).not.toBeNull();
    if (!drawerBox) return;
    const viewport = await tabletTestPage.evaluate(() => ({
      height: innerHeight,
      width: innerWidth,
    }));
    expect(drawerBox.x).toBeGreaterThanOrEqual(0);
    expect(drawerBox.y).toBeGreaterThanOrEqual(0);
    expect(drawerBox.x + drawerBox.width).toBeLessThanOrEqual(viewport.width);
    expect(drawerBox.y + drawerBox.height).toBeLessThanOrEqual(viewport.height);
    expect(
      await tabletTestPage.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(await tabletTestPage.evaluate(() => window.innerWidth));

    const targetRow = tabletTestPage.getByTestId(`workflow-step-disclosure-row-${targetStep.id}`);
    const targetRowBox = await targetRow.boundingBox();
    expect(targetRowBox).not.toBeNull();
    if (!targetRowBox) return;
    expect(targetRowBox.height).toBeGreaterThanOrEqual(44);

    await tabletTestPage.getByTestId(`workflow-step-disclosure-move-${targetStep.id}`).tap();
    await expect
      .poll(async () => (await apiClient.getTask(task.task_id)).workflow_step_id, {
        timeout: 15_000,
      })
      .toBe(targetStep.id);
  });
});
