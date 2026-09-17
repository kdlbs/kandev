import type { Locator } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

type Box = { x: number; y: number; width: number; height: number };

function requireBox(box: Box | null, name: string): Box {
  if (!box) throw new Error(`${name} must be rendered`);
  return box;
}

async function readStaticPaintBounds(locator: Locator) {
  return locator.evaluate((element) => {
    const previousAnimation = element.style.animation;
    element.style.animation = "none";
    const rect = element.getBoundingClientRect();
    const computed = getComputedStyle(element);
    element.style.animation = previousAnimation;
    return {
      width: rect.width,
      height: rect.height,
      cssWidth: computed.width,
      cssHeight: computed.height,
    };
  });
}

function expectStablePosition(before: Box, after: Box) {
  expect(after.x).toBeCloseTo(before.x, 0);
  expect(after.y).toBeCloseTo(before.y, 0);
}

function adjacentStep(
  steps: Array<{ id: string; name: string; position: number }>,
  currentStepId: string,
): { id: string; name: string; position: number } {
  const sorted = [...steps].sort((left, right) => left.position - right.position);
  const currentIndex = sorted.findIndex((step) => step.id === currentStepId);
  const target = sorted[currentIndex + 1] ?? sorted[currentIndex - 1];
  if (!target) throw new Error("workflow step progress test requires an adjacent target");
  return target;
}

test.describe("Workflow step progress", () => {
  test("shows the pending marker and status while a move request is unresolved", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.seedTask(seedData.workspaceId, "Workflow step progress", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const targetStep = adjacentStep(seedData.steps, seedData.startStepId);

    await testPage.setViewportSize({ width: 900, height: 800 });
    await testPage.goto(`/t/${task.task_id}`);
    await new SessionPage(testPage).waitForLoad();

    const trigger = testPage.getByTestId("workflow-stepper-minimal");
    await expect(trigger).toBeVisible();
    await trigger.hover();

    const disclosure = testPage.getByTestId("workflow-step-disclosure");
    const targetRow = disclosure.getByTestId(`workflow-step-disclosure-row-${targetStep.id}`);
    const marker = targetRow.locator("[data-marker-state]");
    await expect(marker).toBeVisible();
    const before = await marker.boundingBox();
    expect(before).not.toBeNull();

    let releaseMove!: () => void;
    const moveGate = new Promise<void>((resolve) => {
      releaseMove = resolve;
    });
    let resolveMoveRequest!: () => void;
    const moveRequestStarted = new Promise<void>((resolve) => {
      resolveMoveRequest = resolve;
    });
    const moveRoute = `**/api/v1/tasks/${task.task_id}/move`;
    await testPage.route(moveRoute, async (route) => {
      resolveMoveRequest();
      await moveGate;
      try {
        await route.continue();
      } catch {
        // The test timeout may close the page while the held request is still waiting.
      }
    });

    try {
      await targetRow.getByTestId(`workflow-step-disclosure-move-${targetStep.id}`).click();
      await moveRequestStarted;

      await expect(marker).toHaveAttribute("data-marker-state", "pending");
      await expect(disclosure.getByTestId(`workflow-step-progress-${targetStep.id}`)).toContainText(
        "Moving to this step",
      );
      const during = await marker.boundingBox();
      expect(during).not.toBeNull();
      if (!before || !during) return;
      expect(during.width).toBeCloseTo(before.width, 0);
      expect(during.height).toBeCloseTo(before.height, 0);

      releaseMove();
      await expect
        .poll(async () => (await apiClient.getTask(task.task_id)).workflow_step_id, {
          timeout: 15_000,
        })
        .toBe(targetStep.id);
      await expect(disclosure).toBeHidden();
    } finally {
      releaseMove();
      await testPage.unroute(moveRoute);
    }
  });

  test("keeps the full hover card useful after the destination becomes current", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.seedTask(seedData.workspaceId, "Full workflow step progress", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const targetStep = adjacentStep(seedData.steps, seedData.startStepId);

    await testPage.setViewportSize({ width: 1600, height: 900 });
    await testPage.goto(`/t/${task.task_id}`);
    await new SessionPage(testPage).waitForLoad();

    const stepper = testPage.locator('[data-testid="workflow-stepper"]:visible').first();
    const targetTrigger = stepper.getByTestId(`workflow-step-${targetStep.name}`);
    await expect(targetTrigger).toBeVisible();
    await expect(stepper.getByTestId("workflow-stepper-minimal")).toHaveCount(0);

    const marker = targetTrigger.locator("[data-marker-state]");
    const markerPaint = marker.locator("span").first();
    const label = targetTrigger.getByText(targetStep.name, { exact: true });
    const connector = stepper.getByTestId(`workflow-step-connector-${targetStep.id}`);
    const [beforeMarker, beforePaint, beforeLabel, beforeConnector] = await Promise.all([
      marker.boundingBox(),
      markerPaint.boundingBox(),
      label.boundingBox(),
      connector.boundingBox(),
    ]);
    const beforeMarkerBox = requireBox(beforeMarker, "the destination marker");
    const beforePaintBox = requireBox(beforePaint, "the destination paint");
    const beforeLabelBox = requireBox(beforeLabel, "the destination label");
    const beforeConnectorBox = requireBox(beforeConnector, "the destination connector");
    expect(beforePaintBox.width).toBeCloseTo(8, 0);
    expect(beforePaintBox.height).toBeCloseTo(8, 0);

    await targetTrigger.hover();
    const popover = testPage.getByTestId("workflow-step-popover");
    await expect(popover).toBeVisible();

    let releaseMove!: () => void;
    const moveGate = new Promise<void>((resolve) => {
      releaseMove = resolve;
    });
    let resolveMoveRequest!: () => void;
    const moveRequestStarted = new Promise<void>((resolve) => {
      resolveMoveRequest = resolve;
    });
    const moveRoute = `**/api/v1/tasks/${task.task_id}/move`;
    await testPage.route(moveRoute, async (route) => {
      resolveMoveRequest();
      await moveGate;
      try {
        await route.continue();
      } catch {
        // The test timeout may close the page while the held request is still waiting.
      }
    });

    try {
      await popover.getByTestId("workflow-step-move-here").click();
      await moveRequestStarted;
      await expect(marker).toHaveAttribute("data-marker-state", "pending");
      const pendingPaint = marker.locator("svg");
      await expect(pendingPaint).toBeVisible();
      const [duringMarker, duringPaint, duringLabel, duringConnector] = await Promise.all([
        marker.boundingBox(),
        pendingPaint.boundingBox(),
        label.boundingBox(),
        connector.boundingBox(),
      ]);
      const duringMarkerBox = requireBox(duringMarker, "the pending marker");
      const duringPaintBox = requireBox(duringPaint, "the pending paint");
      const duringLabelBox = requireBox(duringLabel, "the pending label");
      const duringConnectorBox = requireBox(duringConnector, "the pending connector");
      expect(duringPaintBox.width).toBeGreaterThan(0);
      expect(duringPaintBox.height).toBeGreaterThan(0);
      const staticPaint = await readStaticPaintBounds(pendingPaint);
      expect(staticPaint.width).toBeCloseTo(8, 0);
      expect(staticPaint.height).toBeCloseTo(8, 0);
      expect(staticPaint.cssWidth).toBe("8px");
      expect(staticPaint.cssHeight).toBe("8px");
      expect(duringMarkerBox.width).toBeCloseTo(beforeMarkerBox.width, 0);
      expect(duringMarkerBox.height).toBeCloseTo(beforeMarkerBox.height, 0);
      expectStablePosition(beforeLabelBox, duringLabelBox);
      expectStablePosition(beforeConnectorBox, duringConnectorBox);

      releaseMove();
      await expect
        .poll(async () => (await apiClient.getTask(task.task_id)).workflow_step_id, {
          timeout: 15_000,
        })
        .toBe(targetStep.id);

      await expect(targetTrigger).toHaveAttribute("aria-current", "step");
      await targetTrigger.hover();
      await expect(popover).toBeVisible();
      await expect(popover).toContainText("Current step");
      await expect(popover.getByTestId(`workflow-step-progress-${targetStep.id}`)).toContainText(
        "Not started",
      );
    } finally {
      releaseMove();
      await testPage.unroute(moveRoute);
    }
  });
});
