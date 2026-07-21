import { test, expect } from "../../fixtures/test-base";

test.describe("Changelog redirect", () => {
  test("/settings/changelog redirects to /settings/system/updates", async ({ testPage }) => {
    await testPage.goto("/settings/changelog");
    await expect(testPage).toHaveURL(/\/settings\/system\/updates$/, { timeout: 10_000 });
    await expect(testPage.getByTestId("system-page-title")).toHaveText("Updates");
  });
});
