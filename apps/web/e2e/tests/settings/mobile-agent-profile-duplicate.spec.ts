import { test, expect } from "../../fixtures/test-base";
import { seededAgentProfile } from "../../helpers/seeded-agent-profile";
import { waitForFiniteAnimations } from "../../helpers/animations";

/**
 * Mobile parity for the agent profile duplicate flow: the row-level
 * Duplicate control must be reachable by touch on a phone viewport (the
 * 44px hitbox), complete the same copy, and keep the document free of
 * horizontal overflow. Uses the seeded default profile like the desktop
 * spec: the /settings/agents list is server-hydrated, so a freshly POSTed
 * profile would race with SSR.
 */
test.describe("Mobile agent profile duplicate", () => {
  test("duplicates the seeded profile from the settings list", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { agent, profile } = await seededAgentProfile(apiClient, seedData.agentProfileId);
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
      await expect(trigger).toHaveAttribute("aria-expanded", "true");
      const menu = testPage.locator('[data-slot="dropdown-menu-content"]:visible');
      await expect(menu).toBeVisible();
      await waitForFiniteAnimations(menu);
      await menu.getByTestId(`duplicate-profile-${profile.id}`).tap();

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

  test("keeps an open profile menu usable when another profile is removed", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { agent, profile } = await seededAgentProfile(apiClient, seedData.agentProfileId);
    const extraName = "Temporary mobile profile";
    const copyName = `${profile.name} Copy`;
    const extra = await apiClient.createAgentProfile(agent.id, extraName, {
      model: profile.model || "mock-fast",
    });
    try {
      await testPage.goto("/settings/agents");
      const extraRow = testPage.getByTestId("agent-profile-row").filter({ hasText: extraName });
      await expect(extraRow).toHaveCount(1);
      await testPage.waitForFunction(
        () =>
          Reflect.get(window, "__KANDEV_E2E_STORE__")?.getState().connection.status === "connected",
      );
      const row = testPage.getByTestId("agent-profile-row").filter({ hasText: profile.name });
      const trigger = row.getByTestId(`profile-actions-menu-${profile.id}`);
      await trigger.tap();
      await expect(trigger).toHaveAttribute("aria-expanded", "true");
      const menu = testPage.locator('[data-slot="dropdown-menu-content"]:visible');
      await expect(menu).toBeVisible();

      await apiClient.deleteAgentProfile(extra.id, true);
      await expect(extraRow).toHaveCount(0);
      await expect(trigger).toHaveAttribute("aria-expanded", "true");
      await waitForFiniteAnimations(menu);
      await menu.getByTestId(`duplicate-profile-${profile.id}`).tap();
      await expect(testPage.getByRole("link", { name: copyName, exact: true })).toBeVisible();
    } finally {
      const { agents: currentAgents } = await apiClient.listAgents();
      const currentAgent = currentAgents.find((candidate) => candidate.id === agent.id);
      for (const candidate of currentAgent?.profiles ?? []) {
        if (candidate.id === extra.id || candidate.name === copyName) {
          await apiClient.deleteAgentProfile(candidate.id, true);
        }
      }
    }
  });
});
