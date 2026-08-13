import { test, expect } from "../../fixtures/test-base";

/**
 * Mobile parity for the agent profile duplicate flow: the row-level
 * Duplicate control must be reachable by touch on a phone viewport (the
 * 44px hitbox), complete the same copy, and keep the document free of
 * horizontal overflow. Uses the seeded default profile like the desktop
 * spec: the /settings/agents list is server-hydrated, so a freshly POSTed
 * profile would race with SSR.
 */
test.describe("Mobile agent profile duplicate", () => {
  test("duplicates the seeded profile from the settings list", async ({ testPage, apiClient }) => {
    test.setTimeout(90_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = agent.profiles[0];
    const copyName = `${profile.name} Copy`;

    try {
      await testPage.goto("/settings/agents");

      // No document-level horizontal overflow on the list page.
      await expect
        .poll(
          async () =>
            testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
          { timeout: 15_000 },
        )
        .toBe(true);

      const row = testPage.getByTestId("agent-profile-row").filter({ hasText: profile.name });
      await expect(row).toBeVisible({ timeout: 15_000 });
      await expect(row.getByTestId(`profile-actions-inline-${profile.id}`)).toHaveCount(0);
      // The row's actions trigger exposes a touch-sized hitbox (>= 44px).
      // Poll for the box: layout can settle a frame after visibility.
      const trigger = row.getByTestId(`profile-actions-menu-${profile.id}`);
      await expect
        .poll(
          async () => {
            const box = await trigger.boundingBox();
            if (!box) return null;
            return Math.min(box.width, box.height);
          },
          { timeout: 10_000 },
        )
        .toBeGreaterThanOrEqual(44);
      await trigger.tap();
      const duplicate = testPage.getByTestId(`duplicate-profile-${profile.id}`);
      await expect(duplicate).toBeVisible({ timeout: 15_000 });
      await duplicate.tap();

      // The copy appears as its own row without a reload.
      await expect
        .poll(
          async () => {
            const { agents: after } = await apiClient.listAgents();
            const afterAgent = after.find((a) => a.id === agent.id);
            const copy = afterAgent?.profiles.find((p) => p.name === copyName);
            return copy?.id;
          },
          { timeout: 15_000 },
        )
        .toBeTruthy();

      await expect(testPage.getByRole("link", { name: copyName, exact: true })).toBeVisible({
        timeout: 15_000,
      });
    } finally {
      const { agents: cleanupAgents } = await apiClient.listAgents().catch(() => ({
        agents: [] as typeof agents,
      }));
      const cleanupAgent = cleanupAgents.find((a) => a.id === agent.id);
      const copy = cleanupAgent?.profiles.find((p) => p.name === copyName);
      if (copy) {
        await apiClient.deleteAgentProfile(copy.id, true).catch(() => {});
      }
    }
  });
});
