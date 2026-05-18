import { test, expect } from "../../fixtures/test-base";

test.describe("System Logs page", () => {
  test("renders the tail viewer card and refresh works without error", async ({ testPage }) => {
    test.setTimeout(30_000);

    await testPage.goto("/settings/system/logs");

    await expect(testPage.getByTestId("system-page-title")).toHaveText("Logs");
    await expect(testPage.getByTestId("system-log-tail-card")).toBeVisible();

    const refreshButton = testPage.getByTestId("system-log-tail-refresh");
    await expect(refreshButton).toBeVisible();

    const tailRequest = testPage.waitForRequest(
      (req) => req.url().includes("/api/v1/system/logs/tail") && req.method() === "GET",
      { timeout: 10_000 },
    );
    await refreshButton.click();
    await tailRequest;

    // Either the tail content or the empty-tail message is rendered; both are valid.
    const content = testPage.getByTestId("system-log-tail-content");
    const empty = testPage.getByTestId("system-log-tail-empty");
    await expect(content.or(empty)).toBeVisible({ timeout: 10_000 });
  });
});
