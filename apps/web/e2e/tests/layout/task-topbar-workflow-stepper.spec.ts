import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
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
    const disclosureSurface = testPage.getByRole("dialog", { name: "Move to" });
    await expect(disclosure).toBeVisible();
    await expect(disclosureSurface).toBeVisible();
    await testPage.mouse.move(0, 0);
    await expect(disclosureSurface).toBeHidden();
    await trigger.focus();
    await expect(disclosureSurface).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(disclosureSurface).toBeHidden();
    await expect(trigger).toBeFocused();
    await testPage.keyboard.press("Tab");
    await trigger.focus();
    await expect(disclosureSurface).toBeVisible();
    await expect(disclosure.locator('[data-testid^="workflow-step-disclosure-row-"]')).toHaveCount(
      seedData.steps.length,
    );

    const moveButton = testPage.getByTestId(`workflow-step-disclosure-move-${targetStep.id}`);
    await expect(moveButton).toBeVisible();
    const moveButtonBox = await moveButton.boundingBox();
    expect(moveButtonBox).not.toBeNull();
    if (!moveButtonBox) return;
    expect(moveButtonBox.height).toBeLessThan(40);

    let moveButtonFocused = false;
    for (let tabCount = 0; tabCount < seedData.steps.length + 2; tabCount += 1) {
      if (await moveButton.evaluate((element) => element === document.activeElement)) {
        moveButtonFocused = true;
        break;
      }
      await testPage.keyboard.press("Tab");
    }
    expect(moveButtonFocused).toBe(true);
    await testPage.keyboard.press("Enter");

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
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox).not.toBeNull();
    if (!triggerBox) return;
    expect(triggerBox.height).toBeGreaterThanOrEqual(44);
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

    const targetButton = tabletTestPage.getByTestId(
      `workflow-step-disclosure-move-${targetStep.id}`,
    );
    const targetButtonBox = await targetButton.boundingBox();
    expect(targetButtonBox).not.toBeNull();
    if (!targetButtonBox) return;
    expect(targetButtonBox.height).toBeGreaterThanOrEqual(44);
    await targetButton.tap();
    await expect
      .poll(async () => (await apiClient.getTask(task.task_id)).workflow_step_id, {
        timeout: 15_000,
      })
      .toBe(targetStep.id);
  });
});
