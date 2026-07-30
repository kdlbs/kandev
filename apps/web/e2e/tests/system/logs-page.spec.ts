import { test, expect } from "../../fixtures/test-base";

test.describe("System Logs page", () => {
  test("downloads one frontend and backend diagnostic ZIP", async ({ testPage, prCapture }) => {
    test.setTimeout(45_000);
    await testPage.goto("/settings/system/logs");

    await expect(testPage.getByTestId("system-page-title")).toHaveText("Logs");
    await expect(
      testPage.getByText("Create a diagnostic ZIP with frontend and backend logs."),
    ).toBeVisible();
    await expect(testPage.getByText("Review before sharing")).toBeVisible();
    await expect(testPage.getByTestId("system-log-tail-card")).toHaveCount(0);
    await prCapture.screenshot("desktop-combined-diagnostic-logs", {
      caption: "System Logs clearly discloses the combined frontend and backend ZIP.",
      fullPage: true,
    });

    const downloadPromise = testPage.waitForEvent("download");
    await testPage.getByTestId("download-diagnostic-bundle").click();
    await expect(testPage.getByTestId("download-diagnostic-bundle")).toContainText(
      /Collecting|Preparing/,
    );
    const download = await downloadPromise;
    expect(download.suggestedFilename()).toBe("kandev-diagnostic-logs.zip");
  });
});
