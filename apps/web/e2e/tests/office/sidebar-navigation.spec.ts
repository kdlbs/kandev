import { test, expect } from "../../fixtures/office-fixture";
import { AppSidebarPage } from "../../pages/app-sidebar-page";

test.describe("Sidebar navigation", () => {
  test("sidebar shows CEO agent link", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    // Post-overhaul: the office Agents section lives in the unified AppSidebar
    // (`<aside data-testid="app-sidebar">`) inside a COLLAPSIBLE section that
    // defaults to collapsed on the `/office` dashboard (SECTION_ROUTE_MAP only
    // auto-expands it on `/office/agents`). Expand it first, then assert the
    // row. Each agent row is a single `<Link href="/office/agents/<id>">` whose
    // accessible name is the agent name (the avatar is aria-hidden). The
    // sidebar agent list hydrates from a client-side fetch after first paint —
    // 10s gives that hydration headroom on a heavily-loaded run without
    // affecting the happy path (<1s in isolation).
    const sidebar = new AppSidebarPage(testPage);
    await sidebar.expandSection("Agents");
    await expect(sidebar.root.getByRole("link", { name: /CEO/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });

  test("sidebar shows tasks link", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    const sidebar = new AppSidebarPage(testPage);
    await expect(sidebar.root.getByRole("link", { name: /Tasks/i }).first()).toBeVisible();
    await expect(sidebar.root.getByText("No tasks yet.")).toHaveCount(0);
    await expect(sidebar.root.getByRole("button", { name: "Integrations" })).toHaveCount(0);
  });

  test("sidebar shows office workspace pages", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });

    const sidebar = new AppSidebarPage(testPage);
    await expect(sidebar.root.getByRole("link", { name: "Office Settings" })).toHaveAttribute(
      "href",
      "/office/workspace/settings",
    );
    await expect(sidebar.root.getByRole("link", { name: "Skills" })).toHaveAttribute(
      "href",
      "/office/workspace/skills",
    );
    await expect(sidebar.root.getByRole("link", { name: "Agent Topology" })).toHaveAttribute(
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
    await expect(testPage.getByRole("heading", { name: /Agents/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });

  test("navigate to tasks page via dashboard card", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    // Post-overhaul: the sidebar "Tasks" entry is a collapsible section header
    // (a toggle button), not a navigation link — there is no longer an
    // in-sidebar link to the /office/tasks page. Navigate via the dashboard
    // "Tasks In Progress" metric card link instead (mirrors the sibling
    // "navigate to agents page via sidebar" test, which uses "Agents Enabled").
    await testPage.getByRole("link", { name: /Tasks In Progress/i }).click();
    // Scope the heading assertion to the page content (`<main>` in the office
    // layout). The unified AppSidebar's collapsible "Tasks" section header also
    // exposes the accessible text "Tasks", so an unscoped role=heading/text
    // match could be ambiguous against the global rail.
    await expect(
      testPage.locator("main").getByRole("heading", { name: /Tasks/i }).first(),
    ).toBeVisible({
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
    await expect(testPage).toHaveURL(/\/office$/);
    // "Agents Enabled" is the stable dashboard metric marker.
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
  });

  test("Home goes to the Kanban board from a regular route", async ({
    testPage,
    officeSeed: _,
  }) => {
    // Start from a non-home regular route so the Home click is a real
    // navigation (not a no-op): from a non-office route `useInOffice()` is
    // false, so Home must land back on the board at `/`. Avoid settings routes
    // because they intentionally replace the primary nav with settings mode.
    await testPage.goto("/stats");
    const home = testPage.getByRole("link", { name: "Home", exact: true });
    await expect(home).toBeVisible({ timeout: 15_000 });
    await home.click();
    await expect(testPage).toHaveURL(/\/$/);
    await expect(testPage.getByTestId("kanban-board")).toBeVisible({ timeout: 15_000 });
  });
});
