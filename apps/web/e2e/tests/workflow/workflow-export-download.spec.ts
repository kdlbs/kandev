import { expect, test } from "../../fixtures/test-base";
import { checkDownload, checkGeometry, openExport } from "./workflow-export-download-helpers";

test("downloads single and all workflow exports with keyboard on desktop", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await apiClient.createWorkflow(seedData.workspaceId, "Café 雪 workflow", "standard");
  for (const single of [true, false]) {
    const dialog = await openExport(testPage, seedData.workspaceId, single, false);
    await checkGeometry(testPage, false);
    if (single)
      await prCapture.screenshot("workflow-export-desktop", {
        caption: "Download a workflow export on desktop.",
      });
    await checkDownload(testPage, false);
    await dialog
      .locator('[data-slot="dialog-footer"]')
      .getByRole("button", { name: "Close" })
      .click();
    await expect(dialog).not.toBeVisible();
  }
});

test("keeps download contained and touch-sized on a narrow fine-pointer phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 375, height: 812 });
  await apiClient.createWorkflow(seedData.workspaceId, "Café 雪 workflow", "standard");
  await openExport(testPage, seedData.workspaceId, false, false);
  await checkGeometry(testPage, true);
  await checkDownload(testPage, false);
});

test("keeps export actions reachable on short fine-pointer phones", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.createWorkflow(seedData.workspaceId, "Café 雪 workflow", "standard");
  for (const height of [600, 568]) {
    await testPage.setViewportSize({ width: 375, height });
    for (const single of [true, false]) {
      const dialog = await openExport(testPage, seedData.workspaceId, single, false);
      await checkGeometry(testPage, true);
      await checkDownload(testPage, false);
      await dialog
        .locator('[data-slot="dialog-footer"]')
        .getByRole("button", { name: "Close" })
        .click();
      await expect(dialog).not.toBeVisible();
    }
  }
});
