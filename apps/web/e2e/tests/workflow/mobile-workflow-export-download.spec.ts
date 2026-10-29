import { expect, test } from "../../fixtures/test-base";
import { checkDownload, checkGeometry, openExport } from "./workflow-export-download-helpers";

test("downloads single and all workflow exports on a phone", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await apiClient.createWorkflow(seedData.workspaceId, "Café 雪 workflow", "standard");
  for (const single of [true, false]) {
    const dialog = await openExport(testPage, seedData.workspaceId, single, true);
    await checkGeometry(testPage, true);
    if (single)
      await prCapture.screenshot("workflow-export-phone", {
        caption: "Download a workflow export on a phone.",
      });
    await checkDownload(testPage, true);
    await dialog
      .locator('[data-slot="dialog-footer"]')
      .getByRole("button", { name: "Close" })
      .tap();
    await expect(dialog).not.toBeVisible();
  }
  await testPage.setViewportSize({ width: 1024, height: 900 });
  await openExport(testPage, seedData.workspaceId, false, true);
  await checkGeometry(testPage, true);
  await checkDownload(testPage, true);
  for (const height of [600, 568]) {
    await testPage.setViewportSize({ width: 375, height });
    for (const single of [true, false]) {
      const dialog = await openExport(testPage, seedData.workspaceId, single, true);
      await checkGeometry(testPage, true);
      if (height === 568 && single) {
        await prCapture.screenshot("workflow-export-short-phone", {
          caption: "Workflow export actions on a short phone.",
        });
      }
      await checkDownload(testPage, true);
      await dialog
        .locator('[data-slot="dialog-footer"]')
        .getByRole("button", { name: "Close" })
        .tap();
      await expect(dialog).not.toBeVisible();
    }
  }
});
