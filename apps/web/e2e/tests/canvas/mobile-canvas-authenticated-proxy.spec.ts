import { expect, test } from "../../fixtures/test-base";
import { devices, type Browser, type BrowserContext, type Page } from "@playwright/test";
import {
  canvasHref,
  enableCanvasFeature,
  expectCanvasFrameFillsHost,
  getCanvas,
  removeCanvas,
  seedTaskCanvas,
} from "./canvas-fixture";
import {
  startCanvasAuthenticatedProxy,
  type CanvasAuthenticatedProxy,
} from "./canvas-authenticated-proxy-fixture";

async function openMobileCanvas(
  browser: Browser,
  proxy: CanvasAuthenticatedProxy,
  canvasId: string,
): Promise<{ context: BrowserContext; page: Page }> {
  const context = await browser.newContext({
    ...devices["Pixel 5"],
    baseURL: proxy.origin,
    ignoreHTTPSErrors: true,
  });
  await context.addInitScript(() => {
    localStorage.setItem("kandev.onboarding.completed", "true");
    delete (window as unknown as { __KANDEV_API_PORT?: string }).__KANDEV_API_PORT;
  });
  await proxy.setAuthentication(context);
  const page = await context.newPage();
  await page.goto(canvasHref(canvasId));
  return { context, page };
}

test.describe("authenticated same-origin canvas runtime on mobile", () => {
  test("loads retained canvas data through cookie authentication on phone", async ({
    browser,
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
    let canvasId: string | undefined;
    let proxy: CanvasAuthenticatedProxy | undefined;
    let context: BrowserContext | undefined;
    try {
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData, true);
      canvasId = seeded.canvas.id;
      proxy = await startCanvasAuthenticatedProxy(backend.baseUrl);
      const opened = await openMobileCanvas(browser, proxy, canvasId);
      context = opened.context;
      const page = opened.page;

      await expect(page.getByTestId("web-app-frame")).toHaveAttribute("data-frame-state", "ready", {
        timeout: 30_000,
      });
      await expectCanvasFrameFillsHost(page);
      const frame = page.frameLocator('iframe[title="E2E Plugin Canvas"]');
      await expect(frame.getByTestId("canvas-fixture-context")).toHaveText(seeded.taskId);
      await expect(frame.getByTestId("canvas-fixture-task-count")).toHaveText("1");
      await expect(frame.getByTestId("canvas-fixture-sse-status")).toHaveText("connected");
      await expect.poll(() => proxy?.count("context", true) ?? 0).toBeGreaterThan(0);
      await expect.poll(() => proxy?.count("data", true) ?? 0).toBeGreaterThan(0);
      await expect.poll(() => proxy?.count("events", true) ?? 0).toBeGreaterThan(0);
    } finally {
      await context?.close();
      await proxy?.close();
      if (canvasId) await removeCanvas(apiClient, canvasId);
      await releaseFeature();
    }
  });

  test("recovers after proxy authentication expires on phone", async ({
    browser,
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
    let canvasId: string | undefined;
    let proxy: CanvasAuthenticatedProxy | undefined;
    let context: BrowserContext | undefined;
    try {
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData, true);
      canvasId = seeded.canvas.id;
      const releaseBefore = (await getCanvas(apiClient, canvasId))?.active_release_id;
      proxy = await startCanvasAuthenticatedProxy(backend.baseUrl);
      const opened = await openMobileCanvas(browser, proxy, canvasId);
      context = opened.context;
      const page = opened.page;
      await expect(page.getByTestId("web-app-frame")).toHaveAttribute("data-frame-state", "ready", {
        timeout: 30_000,
      });

      await proxy.clearAuthentication(context);
      await page.reload();
      await expect(page.getByTestId("canvas-host-state")).toHaveText("Canvas unavailable", {
        timeout: 30_000,
      });
      await expect(page.getByTestId("web-app-frame")).toHaveCount(0);
      await expect(page.getByRole("button", { name: "Try again", exact: true })).toBeVisible();

      await proxy.setAuthentication(context);
      await page.getByRole("button", { name: "Try again", exact: true }).tap();
      await expect(page.getByTestId("web-app-frame")).toHaveAttribute("data-frame-state", "ready", {
        timeout: 30_000,
      });
      expect((await getCanvas(apiClient, canvasId))?.active_release_id).toBe(releaseBefore);
    } finally {
      await context?.close();
      await proxy?.close();
      if (canvasId) await removeCanvas(apiClient, canvasId);
      await releaseFeature();
    }
  });
});
