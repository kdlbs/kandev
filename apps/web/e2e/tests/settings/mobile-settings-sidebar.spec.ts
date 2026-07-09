import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile settings navigation", () => {
  test("keeps the desktop settings sidebar hidden without leaking the old active text", async ({
    testPage,
  }) => {
    await testPage.goto("/settings");

    await expect(testPage.getByRole("link", { name: /Appearance/ })).toBeVisible();
    await expect(testPage.getByRole("link", { name: /Terminal/ })).toBeVisible();
    await expect(testPage.getByText("[active]")).toHaveCount(0);
    await expect(testPage.getByTestId("app-sidebar")).toBeHidden();

    const takeover = testPage.getByTestId("app-sidebar-settings-mode");

    await expect(takeover).toBeHidden();
  });
});
