import { expect, test } from "../../fixtures/test-base";
import {
  approvePendingCanvas,
  canvasHref,
  enableCanvasFeature,
  removeCanvas,
  seedTaskCanvas,
} from "./canvas-fixture";

test("renames a task canvas without replacing its running frame", async ({
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
    const active = seeded.canvas.pending_release
      ? await approvePendingCanvas(apiClient, seeded.canvas)
      : seeded.canvas;
    await testPage.goto(canvasHref(active.id));
    const frame = testPage.getByTestId("web-app-frame").locator("iframe");
    await expect(frame).toBeVisible();
    await frame.evaluate((element) => {
      element.setAttribute("data-rename-marker", "same-frame");
    });
    expect(
      (await testPage.getByTestId("canvas-rename-action").boundingBox())?.height,
    ).toBeLessThanOrEqual(32);
    await testPage.getByTestId("canvas-rename-action").click();
    const dialog = testPage.getByRole("dialog", { name: "Rename canvas" });
    await expect(dialog.getByLabel("Canvas name")).toBeFocused();
    await dialog.getByLabel("Canvas name").fill("Updated workflow canvas");
    await dialog.getByRole("button", { name: "Save" }).click();
    await expect(testPage.getByText("Updated workflow canvas").first()).toBeVisible();
    await expect(frame).toHaveAttribute("data-rename-marker", "same-frame");
    await testPage.reload();
    await expect(testPage.getByText("Updated workflow canvas").first()).toBeVisible();
  } finally {
    if (canvasId) await removeCanvas(apiClient, canvasId);
    await releaseFeature();
  }
});
