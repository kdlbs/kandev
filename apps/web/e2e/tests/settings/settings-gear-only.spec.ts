import { test, expect } from "../../fixtures/test-base";

// Settings is reachable ONLY via the footer gear (no nav "Settings" section),
// and the gear closes the settings-tree takeover even while on a settings page.
test.describe("Settings sidebar takeover", () => {
  test("gear opens the tree, and closes it even on a settings page", async ({ testPage }) => {
    await testPage.goto("/settings");

    const gear = testPage.getByTestId("sidebar-settings-gear");
    const takeover = testPage.getByTestId("app-sidebar-settings-mode");

    // No standalone "Settings" nav section: the tree is not shown until the gear
    // is toggled, even though we're sitting on a settings page.
    await expect(gear).toBeVisible();
    await expect(takeover).toHaveCount(0);

    // Gear opens the takeover.
    await gear.click();
    await expect(takeover).toBeVisible();

    // Enter a section — navigates to a settings sub-page; takeover stays open.
    await takeover.locator('a[href="/settings/agents"]').first().click();
    await expect(testPage).toHaveURL(/\/settings\/agents/);
    await expect(takeover).toBeVisible();

    // Clicking the gear again must close the tree — even though we're still on a
    // settings page (the previous bug left it open).
    await gear.click();
    await expect(takeover).toHaveCount(0);
  });
});
