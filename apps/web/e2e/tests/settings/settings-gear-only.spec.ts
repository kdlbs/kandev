import { test, expect } from "../../fixtures/test-base";

// Settings has no nav "Settings" section; direct settings routes keep the
// gear-gated tree open so a hard refresh preserves settings navigation context.
test.describe("Settings sidebar takeover", () => {
  test("direct settings routes open the tree, and the gear can close it", async ({ testPage }) => {
    await testPage.goto("/settings");

    const gear = testPage.getByTestId("sidebar-settings-gear");
    const takeover = testPage.getByTestId("app-sidebar-settings-mode");

    // Direct settings routes preserve sidebar context after a hard refresh.
    await expect(gear).toBeVisible();
    await expect(takeover).toBeVisible();

    // Gear closes the takeover even though we're sitting on a settings page.
    await gear.click();
    await expect(takeover).toHaveCount(0);

    // Gear opens the takeover again.
    await gear.click();
    await expect(takeover).toBeVisible();
    await expect(takeover.getByText("Active", { exact: true })).toBeVisible();
    await expect(takeover.getByRole("link", { name: "Repositories" })).toBeVisible();
    await expect(takeover.getByRole("link", { name: "Integrations" })).toHaveAttribute(
      "href",
      /\/settings\/workspace\/[^/]+\/integrations$/,
    );
    await expect(takeover.locator('a[href="/settings/integrations"]')).toHaveCount(0);

    // Enter a section: navigates to a settings sub-page; takeover stays open.
    await takeover.locator('a[href="/settings/agents"]').first().click();
    await expect(testPage).toHaveURL(/\/settings\/agents/);
    await expect(takeover).toBeVisible();

    // Clicking the gear again must close the tree even though we're still on a
    // settings page (the previous bug left it open).
    await gear.click();
    await expect(takeover).toHaveCount(0);
  });

  test("navigating off settings (Kandev brand → Home) closes the takeover", async ({
    testPage,
  }) => {
    await testPage.goto("/settings");

    const takeover = testPage.getByTestId("app-sidebar-settings-mode");
    const sidebar = testPage.getByTestId("app-sidebar");

    await expect(takeover).toBeVisible();

    // The Kandev brand navigates Home; leaving the settings surface must close
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

    // Clicking the gear while collapsed must expand the rail AND show the tree:
    // a collapsed rail can't host the settings tree.
    await testPage.getByTestId("sidebar-settings-gear").click();
    await expect(sidebar).toHaveAttribute("data-collapsed", "false");
    await expect(takeover).toBeVisible();
  });

  test("a route change cannot undo the first gear click", async ({ testPage }) => {
    await testPage.goto("/");

    const takeover = testPage.getByTestId("app-sidebar-settings-mode");
    await expect(testPage.getByTestId("sidebar-settings-gear")).toBeVisible();

    // Keep both actions in one browser task so the route-sync effect is still
    // pending when the user action arrives. The click must win.
    await testPage.evaluate(() => {
      window.history.pushState({}, "", "/tasks");
      window.dispatchEvent(new Event("kandev:navigation"));
      const gear = document.querySelector<HTMLElement>('[data-testid="sidebar-settings-gear"]');
      if (!gear) throw new Error("sidebar-settings-gear not found");
      gear.click();
    });

    await expect(takeover).toBeVisible();
  });
});
