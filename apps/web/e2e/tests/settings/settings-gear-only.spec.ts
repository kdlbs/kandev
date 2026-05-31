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

  test("navigating off settings (Kandev brand → Home) closes the takeover", async ({
    testPage,
  }) => {
    await testPage.goto("/settings");

    const gear = testPage.getByTestId("sidebar-settings-gear");
    const takeover = testPage.getByTestId("app-sidebar-settings-mode");
    const sidebar = testPage.getByTestId("app-sidebar");

    await gear.click();
    await expect(takeover).toBeVisible();

    // The Kandev brand navigates Home — leaving the settings surface must close
    // the takeover (the bug left the tree up after going Home).
    await sidebar.locator('[aria-label="Kandev home"]').first().click();
    await expect(testPage).not.toHaveURL(/\/settings/);
    await expect(takeover).toHaveCount(0);
  });

  test("the gear expands a collapsed sidebar and reveals the tree", async ({ testPage }) => {
    await testPage.goto("/");

    const sidebar = testPage.getByTestId("app-sidebar");
    const takeover = testPage.getByTestId("app-sidebar-settings-mode");

    // Collapse the rail first.
    await sidebar.getByRole("button", { name: "Collapse sidebar" }).click();
    await expect(sidebar).toHaveAttribute("data-collapsed", "true");

    // Clicking the gear while collapsed must expand the rail AND show the tree —
    // a collapsed rail can't host the settings tree.
    await testPage.getByTestId("sidebar-settings-gear").click();
    await expect(sidebar).toHaveAttribute("data-collapsed", "false");
    await expect(takeover).toBeVisible();
  });
});
