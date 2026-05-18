import { test, expect } from "../../fixtures/test-base";

test.describe("System Status page", () => {
  test("renders health card, version summary, and the page title", async ({ testPage }) => {
    await testPage.goto("/settings/system/status");

    await expect(testPage.getByTestId("system-page-title")).toHaveText("Status");
    await expect(testPage.getByTestId("system-health-card")).toBeVisible();
    await expect(testPage.getByTestId("system-version-summary-card")).toBeVisible();
    await expect(testPage.getByTestId("system-disk-usage-card")).toBeVisible();
  });
});
