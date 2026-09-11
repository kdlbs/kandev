import { test, expect } from "../../fixtures/test-base";
import { mockDatabaseFootprintOverview } from "../../helpers/storage-maintenance";

test.describe("Mobile storage database footprint", () => {
  test("wraps long database paths and keeps footprint rows touch reachable", async ({
    testPage,
    prCapture,
  }) => {
    const databasePath = `/var/lib/kandev/${"long-database-directory/".repeat(8)}kandev.db`;
    const backupPath = `/var/lib/kandev/${"long-backup-directory/".repeat(8)}backups`;
    await mockDatabaseFootprintOverview(testPage, { databasePath, backupPath });

    await testPage.goto("/settings/system/storage");
    for (const resourceId of ["database", "database-backups"]) {
      const trigger = testPage.getByTestId(`storage-resource-${resourceId}-trigger`);
      await expect(trigger).toBeVisible();
      const box = await trigger.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      await trigger.tap();
    }

    await expect(testPage.getByTestId("storage-resource-database")).toContainText(databasePath);
    await expect(testPage.getByTestId("storage-resource-database-backups")).toContainText(
      backupPath,
    );
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await prCapture.screenshot("database-footprint", {
      caption: "Mobile storage wraps database paths without horizontal overflow",
      fullPage: true,
    });
  });
});
