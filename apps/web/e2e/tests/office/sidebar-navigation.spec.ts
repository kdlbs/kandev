import { test, expect } from "../../fixtures/office-fixture";

test.describe("Sidebar navigation", () => {
  test("sidebar shows CEO agent link", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    // Post-overhaul: the office Agents section lives in the unified AppSidebar
    // (`<aside data-testid="app-sidebar">`). Each agent row is a single
    // `<Link href="/office/agents/<id>">` whose accessible name is the agent
    // name (the avatar is aria-hidden). The sidebar agent list hydrates from a
    // client-side fetch after first paint — 10s gives that hydration headroom
    // on a heavily-loaded run without affecting the happy path (<1s in isolation).
    await expect(
      testPage.getByTestId("app-sidebar").getByRole("link", { name: /CEO/i }).first(),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("sidebar shows tasks link", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByRole("link", { name: /Tasks/i }).first()).toBeVisible();
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
    await expect(testPage.getByRole("heading", { name: /Tasks/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });
});
