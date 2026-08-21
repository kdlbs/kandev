import { test, expect } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";
import { AppSidebarPage } from "../../pages/app-sidebar-page";

test.describe("Sidebar navigation", () => {
  test("sidebar shows CEO agent link", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    // Each agent row is a single `<Link href="/office/agents/<id>">` whose
    // accessible name is the agent name (the avatar is aria-hidden). The
    // sidebar agent list hydrates from a client-side fetch after first paint.
    const sidebar = new AppSidebarPage(testPage);
    await expect(sidebar.root.getByRole("link", { name: /CEO/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });

  test("sidebar shows tasks link", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    const sidebar = new AppSidebarPage(testPage);
    await expect(sidebar.root.getByRole("link", { name: /Tasks/i })).toBeVisible();
    await expect(sidebar.root.getByText("No tasks yet.")).toHaveCount(0);
    await expect(sidebar.root.getByRole("button", { name: "Integrations" })).toHaveCount(0);
  });

  test("sidebar shows office workspace pages", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });

    const sidebar = new AppSidebarPage(testPage);
    await expect(sidebar.root.getByRole("link", { name: "Preferences" })).toHaveAttribute(
      "href",
      "/office/workspace/settings",
    );
    await expect(sidebar.root.getByRole("link", { name: "Skills" })).toHaveAttribute(
      "href",
      "/office/workspace/skills",
    );
    await expect(sidebar.root.getByRole("link", { name: "Agent topology" })).toHaveAttribute(
      "href",
      "/office/workspace/org",
    );
    await expect(sidebar.root.getByRole("link", { name: "Costs" })).toHaveAttribute(
      "href",
      "/office/workspace/costs",
    );
  });

  test("navigate to agents page via sidebar", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    // Click the "Agents Enabled" metric card link on the dashboard to navigate to agents
    await testPage.getByRole("link", { name: /Agents Enabled/i }).click();
    await expect(officeTopbarTitle(testPage)).toHaveText(/Agents/i, {
      timeout: 10_000,
    });
  });

  test("navigate to tasks page via dashboard card", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    // Keep dashboard-card navigation covered separately from the sidebar Tasks
    // link asserted above; both are intentional entry points to /office/tasks.
    await testPage.getByRole("link", { name: /Tasks In Progress/i }).click();
    // Assert on the topbar's title crumb rather than an unscoped role=heading/
    // text match: the unified AppSidebar's collapsible "Tasks" section header
    // also exposes the accessible text "Tasks" against the global rail.
    await expect(officeTopbarTitle(testPage)).toHaveText(/Tasks/i, {
      timeout: 10_000,
    });
  });
});

// The sidebar "Home" item is office-aware: while on any /office/* route it
// targets the office dashboard (/office), and on a regular Kanban route it
// targets the board (/). Active-highlight stays exact-match in both modes.
test.describe("Sidebar Home destination", () => {
  test("Home goes to the office dashboard from an office route", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.goto("/office/inbox");
    const home = testPage.getByRole("link", { name: "Home", exact: true });
    await expect(home).toBeVisible({ timeout: 15_000 });
    await home.click();
    // Carries the workspace id: Home resolves through the same rule as the
    // sidebar brand link, so the two are byte-identical rather than one
    // naming the workspace and the other not.
    await expect(testPage).toHaveURL(/\/office\?workspaceId=.+$/);
    // "Agents Enabled" is the stable dashboard metric marker.
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
  });

  test("Home follows the workspace, not the route, from a shared surface", async ({
    testPage,
    officeSeed: _,
  }) => {
    // This spec used to assert the opposite: from /stats, Home went to the
    // kanban board regardless of which workspace was active. That was the
    // pathname rule — /stats is not an /office route — and it is exactly what
    // made "go Home" switch an Office user's workspace as a side effect.
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 15_000 });

    await testPage.goto("/stats");
    const home = testPage.getByRole("link", { name: "Home", exact: true });
    await expect(home).toBeVisible({ timeout: 15_000 });
    // Still an office workspace, so Home still points at its Office home even
    // though /stats is a shared, non-office route.
    await expect(home).toHaveAttribute("href", /^\/office\?workspaceId=.+$/);

    await home.click();
    await expect(testPage).toHaveURL(/\/office(\?|$)/);
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 15_000 });
  });

  test("the kanban board redirects to Office when an office workspace is active", async ({
    testPage,
    officeSeed: _,
  }) => {
    // The other half of the same rule: the workspace decides, so asking for
    // the kanban board while an Office workspace is active moves the URL to
    // that workspace's Office home. It used to do the reverse — quietly
    // activate some other workspace whose board it could render.
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 15_000 });

    await testPage.goto("/");

    await expect(testPage).toHaveURL(/\/office(\?|$)/, { timeout: 15_000 });
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 15_000 });
  });
});
