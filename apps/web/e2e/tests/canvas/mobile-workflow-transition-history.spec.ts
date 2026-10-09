import { expect, test } from "../../fixtures/test-base";
import {
  approvePendingCanvas,
  canvasHref,
  enableCanvasFeature,
  removeCanvas,
  seedTaskCanvas,
} from "./canvas-fixture";

test("phone canvas can read its scoped task trail without page overflow", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  test.setTimeout(180_000);
  const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
  let canvasId: string | undefined;
  try {
    const seeded = await seedTaskCanvas(testPage, apiClient, seedData, true, {
      minimalPermissions: true,
    });
    canvasId = seeded.canvas.id;
    const approved = seeded.canvas.pending_release
      ? await approvePendingCanvas(apiClient, seeded.canvas)
      : seeded.canvas;
    await testPage.goto(canvasHref(approved.id));
    const body = testPage.frameLocator('[data-testid="web-app-frame"] iframe').locator("body");
    await expect(body).toBeVisible();
    const result = await body.evaluate(async (_, taskId) => {
      const response = await fetch(
        `./_kandev/v1/data/tasks/${encodeURIComponent(taskId)}/step-transitions?limit=200`,
      );
      return { status: response.status, body: await response.json() };
    }, seeded.taskId);
    expect(result.status).toBe(200);
    expect(Array.isArray(result.body.items)).toBe(true);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  } finally {
    if (canvasId) await removeCanvas(apiClient, canvasId);
    await releaseFeature();
  }
});
