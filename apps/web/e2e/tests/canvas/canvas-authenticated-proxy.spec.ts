import { expect, test } from "../../fixtures/test-base";
import type { Browser, BrowserContext, Page } from "@playwright/test";
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

async function openAuthenticatedCanvas(
  browser: Browser,
  proxy: CanvasAuthenticatedProxy,
  canvasId: string,
  mobile = false,
): Promise<{ context: BrowserContext; page: Page }> {
  const context = await browser.newContext({
    baseURL: proxy.origin,
    ignoreHTTPSErrors: true,
    ...(mobile ? { viewport: { width: 393, height: 852 }, isMobile: true, hasTouch: true } : {}),
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

test.describe("authenticated same-origin canvas runtime", () => {
  test("loads a retained canvas through cookie authentication", async ({
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
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData);
      canvasId = seeded.canvas.id;
      proxy = await startCanvasAuthenticatedProxy(backend.baseUrl, { injectRuntimeHtml: true });
      const opened = await openAuthenticatedCanvas(browser, proxy, canvasId);
      context = opened.context;
      const page = opened.page;

      await expect(page.getByTestId("web-app-frame")).toHaveAttribute("data-frame-state", "ready", {
        timeout: 30_000,
      });
      await expectCanvasFrameFillsHost(page);
      const frame = page.frameLocator('iframe[title="E2E Plugin Canvas"]');
      await expect(frame.getByTestId("canvas-fixture-script")).toHaveText("inline-ready");
      await expect(frame.getByTestId("canvas-fixture-context")).toHaveText(seeded.taskId);
      await expect(frame.getByTestId("canvas-fixture-task-count")).toHaveText("1");
      await expect(frame.getByTestId("canvas-fixture-sse-status")).toHaveText("connected", {
        timeout: 20_000,
      });
      await expect.poll(() => proxy?.count("runtime-entry", true) ?? 0).toBeGreaterThan(0);
      await expect.poll(() => proxy?.count("runtime-asset", true) ?? 0).toBeGreaterThan(0);
      await expect.poll(() => proxy?.count("context", true) ?? 0).toBeGreaterThan(0);
      await expect.poll(() => proxy?.count("data", true) ?? 0).toBeGreaterThan(0);
      await expect.poll(() => proxy?.count("events", true) ?? 0).toBeGreaterThan(0);
      expect(proxy?.injectedRuntimeDocuments()).toBe(0);
      expect(proxy?.observations().every((observation) => observation.host === proxy?.host)).toBe(
        true,
      );

      await frame.getByTestId("canvas-fixture-continue").dispatchEvent("click");
      await expect(frame.getByTestId("canvas-fixture-message-status")).toHaveText("accepted");
      await expect
        .poll(async () =>
          Number(await frame.getByTestId("canvas-fixture-sse-events").textContent()),
        )
        .toBeGreaterThan(0);
      await expect.poll(() => proxy?.count("write", true) ?? 0).toBeGreaterThan(0);
    } finally {
      await context?.close();
      await proxy?.close();
      if (canvasId) await removeCanvas(apiClient, canvasId);
      await releaseFeature();
    }
  });

  test("keeps the existing recoverable failure when a proxy ignores no-transform", async ({
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
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData);
      canvasId = seeded.canvas.id;
      proxy = await startCanvasAuthenticatedProxy(backend.baseUrl, {
        injectRuntimeHtml: true,
        stripNoTransform: true,
      });
      const opened = await openAuthenticatedCanvas(browser, proxy, canvasId);
      context = opened.context;

      await expect(opened.page.getByTestId("canvas-host-state")).toHaveText("Canvas unavailable", {
        timeout: 30_000,
      });
      await expect(opened.page.getByTestId("web-app-frame")).toHaveCount(0);
      await expect(
        opened.page.getByRole("button", { name: "Try again", exact: true }),
      ).toBeVisible();
      expect(proxy.injectedRuntimeDocuments()).toBeGreaterThan(0);
    } finally {
      await context?.close();
      await proxy?.close();
      if (canvasId) await removeCanvas(apiClient, canvasId);
      await releaseFeature();
    }
  });

  test("releases a disconnected event stream before reconnecting", async ({
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
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData);
      canvasId = seeded.canvas.id;
      proxy = await startCanvasAuthenticatedProxy(backend.baseUrl);
      const opened = await openAuthenticatedCanvas(browser, proxy, canvasId);
      context = opened.context;
      await expect(opened.page.getByTestId("web-app-frame")).toHaveAttribute(
        "data-frame-state",
        "ready",
        { timeout: 30_000 },
      );
      const frame = opened.page.frameLocator('iframe[title="E2E Plugin Canvas"]');
      await expect(frame.getByTestId("canvas-fixture-sse-status")).toHaveText("connected", {
        timeout: 20_000,
      });
      await expect.poll(() => proxy?.activeUpstreamRequests() ?? 0).toBeGreaterThan(0);

      await opened.page.close();
      await expect
        .poll(() => proxy?.activeUpstreamRequests() ?? 0, {
          timeout: 10_000,
          message: "The proxy kept the disconnected canvas event stream open",
        })
        .toBe(0);

      const reconnectedPage = await context.newPage();
      await reconnectedPage.goto(canvasHref(canvasId));
      await expect(reconnectedPage.getByTestId("web-app-frame")).toHaveAttribute(
        "data-frame-state",
        "ready",
        { timeout: 30_000 },
      );
      const reconnectedFrame = reconnectedPage.frameLocator('iframe[title="E2E Plugin Canvas"]');
      await expect(reconnectedFrame.getByTestId("canvas-fixture-sse-status")).toHaveText(
        "connected",
        { timeout: 20_000 },
      );
      await expect.poll(() => proxy?.count("events", true) ?? 0).toBeGreaterThan(1);
    } finally {
      await context?.close();
      await proxy?.close();
      if (canvasId) await removeCanvas(apiClient, canvasId);
      await releaseFeature();
    }
  });

  test("recovers after proxy authentication expires", async ({
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
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData);
      canvasId = seeded.canvas.id;
      const releaseBefore = (await getCanvas(apiClient, canvasId))?.active_release_id;
      proxy = await startCanvasAuthenticatedProxy(backend.baseUrl);
      const opened = await openAuthenticatedCanvas(browser, proxy, canvasId);
      context = opened.context;
      const page = opened.page;
      await expect(page.getByTestId("web-app-frame")).toHaveAttribute("data-frame-state", "ready", {
        timeout: 30_000,
      });

      await proxy.clearAuthentication(context);
      await page.reload();
      await expect(page.getByTestId("canvas-host-state")).toHaveText(
        "Canvas runtime failed to start",
        { timeout: 30_000 },
      );
      await expect(page.getByTestId("web-app-frame")).toHaveCount(0);
      await expect(page.getByRole("button", { name: "Try again", exact: true })).toBeVisible();

      await proxy.setAuthentication(context);
      await page.getByRole("button", { name: "Try again", exact: true }).click();
      await expect(page.getByTestId("web-app-frame")).toHaveAttribute("data-frame-state", "ready", {
        timeout: 30_000,
      });
      await expectCanvasFrameFillsHost(page);
      expect((await getCanvas(apiClient, canvasId))?.active_release_id).toBe(releaseBefore);
      await expect.poll(() => proxy?.count("runtime-entry", false) ?? 0).toBeGreaterThan(0);
      await expect.poll(() => proxy?.count("runtime-entry", true) ?? 0).toBeGreaterThan(1);
    } finally {
      await context?.close();
      await proxy?.close();
      if (canvasId) await removeCanvas(apiClient, canvasId);
      await releaseFeature();
    }
  });

  test("preserves capability checks with cookies", async ({
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
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData);
      canvasId = seeded.canvas.id;
      proxy = await startCanvasAuthenticatedProxy(backend.baseUrl);
      const opened = await openAuthenticatedCanvas(browser, proxy, canvasId);
      context = opened.context;
      const page = opened.page;
      await expect(page.getByTestId("web-app-frame")).toHaveAttribute("data-frame-state", "ready", {
        timeout: 30_000,
      });

      const checks = await page.evaluate(async () => {
        const frame = document.querySelector<HTMLIFrameElement>(
          'iframe[title="E2E Plugin Canvas"]',
        );
        if (!frame?.src) throw new Error("canvas runtime URL was not mounted");
        const runtimeURL = new URL(frame.src);
        const marker = "/api/v1/plugins/web-apps/runtime/";
        const tokenStart = runtimeURL.pathname.indexOf(marker) + marker.length;
        const tokenEnd = runtimeURL.pathname.indexOf("/", tokenStart);
        if (tokenStart < marker.length || tokenEnd < 0) {
          throw new Error("canvas runtime URL did not contain a capability segment");
        }
        runtimeURL.pathname = `${runtimeURL.pathname.slice(0, tokenStart)}invalid-capability/${runtimeURL.pathname.slice(tokenEnd + 1)}`;
        const invalidToken = await fetch(runtimeURL, { credentials: "same-origin" });
        const deniedWrite = await fetch(`${frame.src}_kandev/v1/actions/not-declared`, {
          method: "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body: "{}",
        });
        return {
          deniedBody: await deniedWrite.text(),
          deniedStatus: deniedWrite.status,
          invalidBody: await invalidToken.text(),
          invalidStatus: invalidToken.status,
        };
      });

      expect(checks.invalidStatus).toBe(404);
      expect(checks.invalidBody).toContain("runtime_token_invalid");
      expect(checks.deniedStatus).toBe(403);
      expect(checks.deniedBody).toContain("plugin_permission_denied");
    } finally {
      await context?.close();
      await proxy?.close();
      if (canvasId) await removeCanvas(apiClient, canvasId);
      await releaseFeature();
    }
  });
});
