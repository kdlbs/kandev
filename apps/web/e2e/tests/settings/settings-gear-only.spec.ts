import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

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

    // Closing leaves the settings surface too: the sidebar and the main panel
    // must not disagree (kanban navigation beside an open settings page).
    await gear.click();
    await expect(takeover).toHaveCount(0);
    await expect(testPage).not.toHaveURL(/\/settings/);

    // Gear opens the takeover again.
    await gear.click();
    await expect(takeover).toBeVisible();

    // The menu is static: the Workspaces row is a page, and workspace data
    // (rows, sections) lives on that page rather than in the menu.
    await takeover.getByRole("link", { name: "Workspaces" }).click();
    await expect(testPage).toHaveURL(/\/settings\/workspaces$/);
    await expect(takeover.getByRole("link", { name: "Repositories" })).toHaveCount(0);
    await expect(takeover.locator('a[href="/settings/integrations"]')).toHaveCount(0);
    // Each workspace row links to its overview as a whole (overlay link) and
    // still offers per-section quick links with counts.
    const workspaceRow = testPage.getByTestId("workspace-list-item").first();
    await expect(workspaceRow.getByTestId("workspace-overview-link")).toHaveAttribute(
      "href",
      /\/settings\/workspaces\/[^/]+$/,
    );
    await expect(workspaceRow.getByRole("link", { name: /Repositories/ })).toBeVisible();
    await expect(workspaceRow.getByRole("link", { name: /Integrations/ })).toHaveAttribute(
      "href",
      /\/settings\/workspaces\/[^/]+\/integrations$/,
    );

    // Enter a section: navigates to a settings sub-page; takeover stays open.
    await takeover.locator('a[href="/settings/agents"]').first().click();
    await expect(testPage).toHaveURL(/\/settings\/agents/);
    await expect(takeover).toBeVisible();

    // Clicking the gear again closes the tree and leaves the settings route,
    // from a nested page too.
    await gear.click();
    await expect(takeover).toHaveCount(0);
    await expect(testPage).not.toHaveURL(/\/settings/);
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

  test("the gear navigates to Settings in one click from a task session view", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Regression: from a session view the gear used to only swap the
    // sidebar's own tree — the main panel kept showing the task, so the
    // click looked like a no-op. A second click (on a tree leaf) was
    // needed to actually reach Settings. The gear must now behave like
    // the sibling Stats/Office footer buttons and navigate in one click.
    const task = await apiClient.createTask(seedData.workspaceId, "Gear Nav Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const takeover = testPage.getByTestId("app-sidebar-settings-mode");
    await expect(takeover).toHaveCount(0);

    await testPage.getByTestId("sidebar-settings-gear").click();

    // The gear pushes `/settings`, which on desktop hands off to the settings
    // page this device was last on — Appearance in a fresh profile.
    await expect(testPage).toHaveURL(/\/settings\/general\/appearance$/);
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
