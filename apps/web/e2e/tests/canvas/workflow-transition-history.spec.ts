import { expect, test } from "../../fixtures/test-base";
import {
  approvePendingCanvas,
  canvasHref,
  enableCanvasFeature,
  promoteCanvas,
  removeCanvas,
  seedTaskCanvas,
} from "./canvas-fixture";

test("workspace transition groups remain available before and after task canvas promotion", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  test.setTimeout(180_000);
  const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
  let canvasId: string | undefined;
  try {
    const seeded = await seedTaskCanvas(testPage, apiClient, seedData, false, {
      minimalPermissions: true,
    });
    canvasId = seeded.canvas.id;
    const approved = seeded.canvas.pending_release
      ? await approvePendingCanvas(apiClient, seeded.canvas)
      : seeded.canvas;
    const nextStep = seedData.steps.find((step) => step.id !== seedData.startStepId);
    expect(nextStep).toBeDefined();
    await apiClient.moveTask(seeded.taskId, seedData.workflowId, nextStep!.id);
    await apiClient.moveTask(seeded.taskId, seedData.workflowId, seedData.startStepId);
    await testPage.goto(canvasHref(approved.id));
    const body = testPage.frameLocator('[data-testid="web-app-frame"] iframe').locator("body");
    await expect(body).toBeVisible();
    const taskRead = await body.evaluate(async (_, taskId) => {
      const response = await fetch(
        `./_kandev/v1/data/tasks/${encodeURIComponent(taskId)}/step-transitions?limit=200`,
      );
      return { status: response.status, body: await response.json() };
    }, seeded.taskId);
    expect(taskRead.status).toBe(200);
    expect(
      taskRead.body.items.some(
        (item: { to_workflow_step_id: string }) =>
          item.to_workflow_step_id === seedData.startStepId,
      ),
    ).toBe(true);
    const prePromotionGroups = await body.evaluate(async (_, flowId) => {
      const response = await fetch(
        `./_kandev/v1/data/workflows/${encodeURIComponent(flowId)}/transition-groups?limit=200`,
      );
      return { status: response.status, body: await response.json() };
    }, seedData.workflowId);
    expect(prePromotionGroups.status).toBe(200);
    expect(prePromotionGroups.body.items.some((item: { count: number }) => item.count > 0)).toBe(
      true,
    );
    const workspace = await promoteCanvas(apiClient, approved);
    await testPage.goto(canvasHref(workspace.id));
    const groups = await testPage
      .frameLocator('[data-testid="web-app-frame"] iframe')
      .locator("body")
      .evaluate(async (_, flowId) => {
        const response = await fetch(
          `./_kandev/v1/data/workflows/${encodeURIComponent(flowId)}/transition-groups?limit=200`,
        );
        return { status: response.status, body: await response.json() };
      }, seedData.workflowId);
    expect(groups.status).toBe(200);
    expect(groups.body.items.some((item: { count: number }) => item.count > 0)).toBe(true);
  } finally {
    if (canvasId) await removeCanvas(apiClient, canvasId);
    await releaseFeature();
  }
});
