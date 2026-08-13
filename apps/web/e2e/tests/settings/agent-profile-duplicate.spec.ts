import { test, expect } from "../../fixtures/test-base";

/**
 * Verifies the agent profile duplicate flow:
 *
 * - A full-desktop /settings/agents profile row shows inline Duplicate/Delete
 *   buttons.
 * - Clicking it creates a "<source> Copy" profile that appears in the list
 *   without a page reload, copying the source's model.
 * - The copy is an independent profile with its own settings page.
 *
 * Uses the seeded default profile (like agent-profile-disable.spec.ts)
 * rather than creating one: the profile list is hydrated on the server, so a
 * freshly POSTed profile race-conditions with SSR.
 */
test.describe("Agent profile duplicate", () => {
  test("duplicates the seeded profile from the settings list", async ({ testPage, apiClient }) => {
    test.setTimeout(90_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = agent.profiles[0];
    const copyName = `${profile.name} Copy`;

    try {
      await testPage.goto("/settings/agents");

      // Full desktop exposes the row actions directly, without the overflow menu.
      const row = testPage.getByTestId("agent-profile-row").filter({ hasText: profile.name });
      await expect(row).toBeVisible({ timeout: 15_000 });
      await expect(row.getByTestId(`profile-actions-inline-${profile.id}`)).toBeVisible();
      await expect(row.getByTestId(`profile-actions-menu-${profile.id}`)).toHaveCount(0);
      const duplicate = row.getByTestId(`duplicate-profile-inline-${profile.id}`);
      await expect(duplicate).toBeVisible({ timeout: 15_000 });
      await expect(duplicate).toHaveAccessibleName("Duplicate");
      await expect(row.getByTestId(`delete-profile-inline-${profile.id}`)).toHaveAccessibleName(
        "Delete",
      );
      await duplicate.click();

      // Backend state: the copy is a new row with the source's model. The
      // duplicate POST is synchronous, so listAgents reflects it immediately.
      let copy: { id: string; model: string } | undefined;
      await expect
        .poll(
          async () => {
            const { agents: after } = await apiClient.listAgents();
            const afterAgent = after.find((a) => a.id === agent.id);
            copy = afterAgent?.profiles.find((p) => p.name === copyName);
            return copy?.id;
          },
          { timeout: 15_000 },
        )
        .toBeTruthy();
      expect(copy?.id).not.toBe(profile.id);
      expect(copy?.model).toBe(profile.model);

      // The copy row appears in the list without a reload. Its row link is
      // the unambiguous handle (the sidebar shows a sibling link with a
      // different accessible name).
      await expect(testPage.getByRole("link", { name: copyName, exact: true })).toBeVisible({
        timeout: 15_000,
      });

      // The copy has its own settings page showing the copied name.
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${copy?.id}`);
      await expect(
        testPage.getByRole("heading", {
          name: new RegExp(copyName.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")),
        }),
      ).toBeVisible({ timeout: 15_000 });
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

  test("keeps profile actions in the overflow menu at tablet width", async ({
    tabletTestPage,
    apiClient,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = agent.profiles[0];

    await tabletTestPage.goto("/settings/agents");

    await expect
      .poll(
        async () =>
          tabletTestPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        { timeout: 15_000 },
      )
      .toBe(true);

    const row = tabletTestPage.getByTestId("agent-profile-row").filter({ hasText: profile.name });
    await expect(row).toBeVisible({ timeout: 15_000 });
    await expect(row.getByTestId(`profile-actions-inline-${profile.id}`)).toHaveCount(0);
    const trigger = row.getByTestId(`profile-actions-menu-${profile.id}`);
    await expect(trigger).toBeVisible();
    await trigger.tap();
    await expect(tabletTestPage.getByTestId(`duplicate-profile-${profile.id}`)).toBeVisible();
    await expect(tabletTestPage.getByTestId(`delete-profile-${profile.id}`)).toBeVisible();
  });

  test("duplicates from the profile page header and navigates to the copy", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(90_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = agent.profiles[0];
    const copyName = `${profile.name} Copy`;

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      const headerDuplicate = testPage.getByTestId("duplicate-profile-header");
      await expect(headerDuplicate).toBeVisible({ timeout: 15_000 });
      await headerDuplicate.click();

      // The copy exists server-side and the SPA navigates to its page (no
      // full reload: the URL changes to the copy's profile route).
      let copy: { id: string } | undefined;
      await expect
        .poll(
          async () => {
            const { agents: after } = await apiClient.listAgents();
            const afterAgent = after.find((a) => a.id === agent.id);
            copy = afterAgent?.profiles.find((p) => p.name === copyName);
            return copy?.id;
          },
          { timeout: 15_000 },
        )
        .toBeTruthy();
      expect(copy?.id).not.toBe(profile.id);
      await expect(testPage).toHaveURL(new RegExp(`/profiles/${copy?.id}$`), {
        timeout: 15_000,
      });

      // The copy page renders the copy's name in its header.
      await expect(
        testPage.getByRole("heading", {
          name: new RegExp(copyName.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")),
        }),
      ).toBeVisible({ timeout: 15_000 });
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
