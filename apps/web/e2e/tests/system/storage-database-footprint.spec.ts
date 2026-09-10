import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";

test.describe("System storage database footprint", () => {
  test("measures the SQLite database and refreshes sibling backup usage", async ({
    testPage,
    backend,
    prCapture,
  }) => {
    const databasePath = path.join(backend.tmpDir, "kandev.db");
    const backupDirectory = path.join(backend.tmpDir, "backups");
    const firstBackup = path.join(backupDirectory, "e2e-manual.db");
    const secondBackup = path.join(backupDirectory, "e2e-second.db");
    fs.mkdirSync(backupDirectory, { recursive: true });
    fs.writeFileSync(firstBackup, "first database snapshot");

    await testPage.goto("/settings/system/storage");
    await testPage.getByTestId("storage-analyze").click();
    await expect(testPage.getByTestId("storage-analyze")).toHaveAttribute(
      "data-job-state",
      "succeeded",
      { timeout: 30_000 },
    );

    const firstOverview = await testPage.evaluate(async () => {
      const response = await fetch("/api/v1/system/storage");
      return response.json();
    });
    expect(firstOverview.summary.database).toMatchObject({
      status: "measured",
      path: databasePath,
      included_in_total: true,
    });
    expect(firstOverview.summary.database.size_bytes).toBeGreaterThan(0);
    expect(firstOverview.summary.database_backups).toMatchObject({
      status: "measured",
      path: backupDirectory,
      included_in_total: true,
      size_bytes: fs.statSync(firstBackup).size,
    });

    await testPage.getByTestId("storage-resource-database-trigger").click();
    await testPage.getByTestId("storage-resource-database-backups-trigger").click();
    await expect(testPage.getByTestId("storage-resource-database")).toContainText(databasePath);
    await expect(testPage.getByTestId("storage-resource-database-backups")).toContainText(
      backupDirectory,
    );
    await expect(testPage.getByTestId("toast-message")).toHaveCount(0, { timeout: 10_000 });
    await testPage.evaluate(() => {
      window.scrollTo(0, 0);
      let primaryScroller: HTMLElement | null = null;
      let primaryScrollRange = 0;
      for (const element of document.querySelectorAll<HTMLElement>("*")) {
        const style = getComputedStyle(element);
        if (
          (style.overflowY === "auto" || style.overflowY === "scroll") &&
          element.scrollHeight > element.clientHeight
        ) {
          element.scrollTop = 0;
          const scrollRange = element.scrollHeight - element.clientHeight;
          if (scrollRange > primaryScrollRange) {
            primaryScroller = element;
            primaryScrollRange = scrollRange;
          }
        }
      }
      primaryScroller?.scrollTo(0, Math.min(180, primaryScrollRange));
    });
    await testPage.mouse.move(20, 20);
    await prCapture.screenshot("database-footprint", {
      caption: "Desktop storage shows the SQLite database and sibling backup footprint",
      fullPage: true,
    });

    fs.writeFileSync(secondBackup, "second database snapshot with more bytes");
    await testPage.getByTestId("storage-analyze").click();
    await expect(testPage.getByTestId("storage-analyze")).toHaveAttribute(
      "data-job-state",
      "succeeded",
      { timeout: 30_000 },
    );
    const refreshedBackupBytes = fs.statSync(firstBackup).size + fs.statSync(secondBackup).size;
    await expect
      .poll(
        async () =>
          testPage.evaluate(async () => {
            const response = await fetch("/api/v1/system/storage");
            const overview = await response.json();
            return overview.summary.database_backups.size_bytes;
          }),
        { timeout: 30_000 },
      )
      .toBe(refreshedBackupBytes);
  });
});
