import { expect, test } from "../../fixtures/test-base";
import {
  approvePendingCanvas,
  enableCanvasFeature,
  getCanvas,
  seedTaskCanvas,
} from "./canvas-fixture";
import { startCanvasOriginFixture } from "./canvas-origin-fixture";

test.describe("Canvas runtime host origins", () => {
  test("executes through same-origin HTTPS aliases and rejects foreign ancestors", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
    const originFixture = await startCanvasOriginFixture(backend.baseUrl);
    try {
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData);
      const published = seeded.canvas.pending_release
        ? await approvePendingCanvas(apiClient, seeded.canvas)
        : seeded.canvas;
      const canvas = (await getCanvas(apiClient, published.id)) ?? published;
      const runtimeResponse = await apiClient.rawRequest(
        "GET",
        `/api/v1/canvases/${encodeURIComponent(canvas.id)}/runtime`,
      );
      expect(runtimeResponse.ok).toBe(true);
      const runtime = (await runtimeResponse.json()) as { runtime_url?: string };
      expect(runtime.runtime_url).toMatch(/^\/api\/v1\/plugins\/web-apps\/runtime\//);
      const runtimePath = runtime.runtime_url as string;

      await originFixture.install(testPage);
      for (const alias of [originFixture.aliases.primary, originFixture.aliases.secondary]) {
        await testPage.goto(
          `${alias.origin}/__kandev_canvas_origin_test__?src=${encodeURIComponent(`${alias.origin}${runtimePath}`)}`,
          { waitUntil: "domcontentloaded" },
        );
        const runtimeFrame = testPage.frameLocator("#runtime");
        await expect(runtimeFrame.getByTestId("canvas-fixture-script")).toHaveText("inline-ready");
        await expect(runtimeFrame.getByTestId("canvas-fixture-content")).toBeVisible();
      }
      await testPage.setExtraHTTPHeaders({
        "X-Forwarded-Host": originFixture.aliases.primary.host,
      });
      const foreignRuntimeURL = `${originFixture.aliases.primary.origin}${runtimePath}`;
      const foreignRuntimeRequest = originFixture.waitForRuntimeRequest(
        runtimePath,
        originFixture.aliases.foreign.origin,
      );
      await testPage.goto(
        `${originFixture.aliases.foreign.origin}/__kandev_canvas_origin_test__?src=${encodeURIComponent(foreignRuntimeURL)}`,
        { waitUntil: "domcontentloaded" },
      );
      await foreignRuntimeRequest;
      await expect.poll(() => runtimeFrameContentCount(testPage)).toBe(0);

      const nestedForeignURL = `${originFixture.aliases.nestedForeign.origin}${
        "/__kandev_canvas_origin_test__?src=" + encodeURIComponent(foreignRuntimeURL)
      }`;
      const nestedRuntimeRequest = originFixture.waitForRuntimeRequest(
        runtimePath,
        originFixture.aliases.nestedForeign.origin,
      );
      await testPage.goto(
        `${originFixture.aliases.foreign.origin}/__kandev_canvas_origin_test__?src=${encodeURIComponent(nestedForeignURL)}`,
        { waitUntil: "domcontentloaded" },
      );
      await nestedRuntimeRequest;
      await expect.poll(() => runtimeFrameContentCount(testPage)).toBe(0);
    } finally {
      await originFixture.close();
      await releaseFeature();
    }
  });
});

async function runtimeFrameContentCount(page: import("@playwright/test").Page): Promise<number> {
  let count = 0;
  for (const frame of page.frames()) {
    if (!frame.url().includes("/api/v1/plugins/web-apps/runtime/")) continue;
    count += await frame.getByTestId("canvas-fixture-content").count();
  }
  return count;
}
