import { test, expect } from "../../fixtures/office-fixture";

test.describe("Workspace settings UI", () => {
  test("settings page shows danger zone", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/workspace/settings");
    await expect(testPage.getByText(/danger zone/i)).toBeVisible({ timeout: 10_000 });
  });
});
