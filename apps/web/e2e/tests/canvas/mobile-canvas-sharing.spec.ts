import { expect, test } from "../../fixtures/test-base";
import { expectTouchControl } from "../../helpers/control-sizing";
import {
  approvePendingCanvas,
  canvasHref,
  enableCanvasFeature,
  removeCanvas,
  promoteCanvas,
  seedTaskCanvas,
} from "./canvas-fixture";

test.describe("Canvas sharing on mobile", () => {
  test("keeps sharing actions in a focused drawer with touch-sized downloads", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
    let canvasId: string | undefined;
    try {
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData, true);
      canvasId = seeded.canvas.id;
      const active = await promoteCanvas(
        apiClient,
        await approvePendingCanvas(apiClient, seeded.canvas),
      );

      await testPage.goto(canvasHref(active.id));
      await expect(testPage.getByTestId("canvas-host-route")).toBeVisible({ timeout: 30_000 });
      await testPage.getByTestId("canvas-mobile-actions").tap();
      const actions = testPage.getByTestId("canvas-mobile-actions-sheet");
      await expect(actions).toBeVisible();
      await actions.getByRole("button", { name: "Share canvas", exact: true }).tap();

      const drawer = testPage.getByRole("dialog").last();
      await expect(drawer).toBeVisible();
      await expectTouchControl(drawer.getByLabel("Package ID", { exact: true }));
      await expectTouchControl(drawer.getByRole("combobox", { name: "Source mode" }));
      const prepare = drawer.getByRole("button", { name: "Prepare downloads", exact: true });
      await expect(prepare).toBeVisible();
      expect((await prepare.boundingBox())?.height).toBeGreaterThanOrEqual(44);
      await prepare.tap();

      const review = drawer.getByTestId("canvas-export-review");
      await expect(review).toBeVisible({ timeout: 30_000 });
      expect(
        (await review.getByRole("button", { name: "Download source", exact: true }).boundingBox())
          ?.height,
      ).toBeGreaterThanOrEqual(44);
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
    } finally {
      if (canvasId) await removeCanvas(apiClient, canvasId);
      await releaseFeature();
    }
  });
});
