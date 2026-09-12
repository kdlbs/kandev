import { expect, test } from "../../fixtures/test-base";
import {
  approvePendingCanvas,
  canvasHref,
  enableCanvasFeature,
  removeCanvas,
  promoteCanvas,
  seedTaskCanvas,
} from "./canvas-fixture";

test.describe("Canvas sharing", () => {
  test("prepares bundle and source downloads from the active release", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
    let canvasId: string | undefined;
    try {
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData);
      canvasId = seeded.canvas.id;
      const approved = await approvePendingCanvas(apiClient, seeded.canvas);

      await testPage.goto(canvasHref(approved.id));
      await expect(testPage.getByTestId("canvas-host-route")).toBeVisible({ timeout: 30_000 });
      await testPage.getByRole("button", { name: "Share canvas", exact: true }).click();
      const taskDialog = testPage.getByRole("dialog").last();
      await expect(taskDialog).toBeVisible();
      await taskDialog.getByRole("button", { name: "Prepare downloads", exact: true }).click();
      await expect(taskDialog.getByTestId("canvas-export-review")).toBeVisible({
        timeout: 30_000,
      });
      await taskDialog.getByRole("button", { name: "Cancel", exact: true }).click();

      const active = await promoteCanvas(apiClient, approved);

      await testPage.goto(canvasHref(active.id));
      await expect(testPage.getByTestId("canvas-host-route")).toBeVisible({ timeout: 30_000 });
      await expect(
        testPage.getByRole("button", { name: "Share canvas", exact: true }),
      ).toBeVisible();
      await testPage.getByRole("button", { name: "Share canvas", exact: true }).click();

      const dialog = testPage.getByRole("dialog").last();
      await expect(dialog).toBeVisible();
      await expect(
        dialog.getByText("Kandev creates bounded archives", { exact: false }),
      ).toBeVisible();
      await dialog.getByRole("button", { name: "Prepare downloads", exact: true }).click();

      const review = dialog.getByTestId("canvas-export-review");
      await expect(review).toBeVisible({ timeout: 30_000 });
      await expect(
        review.getByRole("button", { name: "Download bundle", exact: true }),
      ).toBeVisible();
      await expect(
        review.getByRole("button", { name: "Download source", exact: true }),
      ).toBeVisible();
      await expect(
        dialog.getByText("Check the archive for private content", { exact: false }),
      ).toBeVisible();
    } finally {
      if (canvasId) await removeCanvas(apiClient, canvasId);
      await releaseFeature();
    }
  });
});
