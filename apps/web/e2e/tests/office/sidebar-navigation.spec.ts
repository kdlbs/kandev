import { test, expect } from "../../fixtures/office-fixture";

test.describe("Sidebar navigation", () => {
  test("sidebar shows CEO agent link", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    // The sidebar renders the agent name + an adjacent "Open <name>" icon link;
    // scope to the aside and take the first /CEO/ match. The sidebar
    // agent list hydrates from a client-side fetch after first paint —
    // 10s gives that hydration headroom on a heavily-loaded run without
    // affecting the happy path (it resolves in <1s in isolation).
    await expect(testPage.locator("aside").getByRole("link", { name: /CEO/i }).first()).toBeVisible(
      { timeout: 10_000 },
    );
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

  test("navigate to tasks page via sidebar", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    await testPage.locator("aside").getByRole("link", { name: /Tasks/i }).first().click();
    await expect(testPage.getByRole("heading", { name: /Tasks/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });
});
