import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile general settings", () => {
  test("opens a dedicated General settings page from the overview", async ({ testPage }) => {
    await testPage.goto("/settings/general");

    await expect(
      testPage.getByRole("heading", { name: "General Settings", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    await testPage.getByRole("link", { name: /Terminal/ }).click();

    await expect(testPage).toHaveURL(/\/settings\/general\/terminal$/);
    await expect(testPage.locator("h2", { hasText: "Terminal" })).toBeVisible();
    await expect(testPage.getByTestId("terminal-font-select")).toBeVisible();
    await expect(testPage.getByTestId("terminal-font-size-input")).toBeVisible();
  });
});
