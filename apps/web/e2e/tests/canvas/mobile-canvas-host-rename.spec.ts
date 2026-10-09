import { expect, test } from "../../fixtures/test-base";
import {
  approvePendingCanvas,
  canvasHref,
  enableCanvasFeature,
  removeCanvas,
  seedTaskCanvas,
} from "./canvas-fixture";

test("renames from the phone action drawer", async ({ testPage, apiClient, backend, seedData }) => {
  test.setTimeout(180_000);
  const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
  let canvasId: string | undefined;
  try {
    const seeded = await seedTaskCanvas(testPage, apiClient, seedData, true);
    canvasId = seeded.canvas.id;
    const active = seeded.canvas.pending_release
      ? await approvePendingCanvas(apiClient, seeded.canvas)
      : seeded.canvas;
    await testPage.goto(canvasHref(active.id));
    await testPage.getByTestId("canvas-mobile-actions").tap();
    await testPage.getByTestId("canvas-mobile-rename").tap();
    const dialog = testPage.getByRole("dialog", { name: "Rename canvas" });
    await dialog.getByLabel("Canvas name").fill("Phone canvas title");
    await dialog.getByRole("button", { name: "Save" }).tap();
    await expect(testPage.getByText("Phone canvas title").first()).toBeVisible();
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  } finally {
    if (canvasId) await removeCanvas(apiClient, canvasId);
    await releaseFeature();
  }
});
