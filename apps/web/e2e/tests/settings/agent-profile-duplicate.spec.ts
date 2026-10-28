import { test, expect } from "../../fixtures/test-base";
import { seededAgentProfile } from "../../helpers/seeded-agent-profile";

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

      // Full desktop exposes the row actions directly, without the overflow menu.
      const row = testPage.getByTestId("agent-profile-row").filter({ hasText: profile.name });
      await expect(row).toBeVisible({ timeout: 15_000 });
      await expect(row.getByTestId(`profile-actions-inline-${profile.id}`)).toBeVisible();
      await expect(row.getByTestId(`profile-actions-menu-${profile.id}`)).toHaveCount(0);
      const duplicate = row.getByTestId(`duplicate-profile-inline-${profile.id}`);
      const deleteButton = row.getByTestId(`delete-profile-inline-${profile.id}`);
      await expect(duplicate).toBeVisible({ timeout: 15_000 });
      await expect(duplicate).toHaveAccessibleName("Duplicate");
      await expect(duplicate).toHaveText("");
      await expect(duplicate).toHaveAttribute("data-size", "icon");
      await expect(deleteButton).toHaveAccessibleName("Delete");
      await expect(deleteButton).toHaveText("");
      await expect(deleteButton).toHaveAttribute("data-size", "icon");
      await duplicate.hover();
      await expect(testPage.getByRole("tooltip", { name: "Duplicate" })).toBeVisible();
      await deleteButton.hover();
      await expect(testPage.getByRole("tooltip", { name: "Delete" })).toBeVisible();
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
    seedData,
  }) => {
    const { profile } = await seededAgentProfile(apiClient, seedData.agentProfileId);

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

  test("keeps the profile menu open when agent discovery completes", async ({
    tabletTestPage,
    apiClient,
    seedData,
  }) => {
    const { agent, profile } = await seededAgentProfile(apiClient, seedData.agentProfileId);
    await tabletTestPage.goto("/settings/agents");
    await tabletTestPage.waitForFunction((agentName) => {
      const discovery = Reflect.get(window, "__KANDEV_E2E_STORE__")?.getState().agentDiscovery;
      return (
        discovery?.loaded &&
        discovery.items.some((item: { name: string }) => item.name === agentName)
      );
    }, agent.name);
    await tabletTestPage.evaluate(() => {
      const store = Reflect.get(window, "__KANDEV_E2E_STORE__");
      Reflect.set(window, "__profileDiscoverySnapshot", store.getState().agentDiscovery.items);
      store.getState().setAgentDiscovery([]);
    });
    const trigger = tabletTestPage.getByTestId(`profile-actions-menu-${profile.id}`);
    await trigger.tap();
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    await tabletTestPage.evaluate(() => {
      const store = Reflect.get(window, "__KANDEV_E2E_STORE__");
      store.getState().setAgentDiscovery(Reflect.get(window, "__profileDiscoverySnapshot"));
      Reflect.deleteProperty(window, "__profileDiscoverySnapshot");
    });
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    await expect(tabletTestPage.getByTestId(`duplicate-profile-${profile.id}`)).toBeVisible();
  });

  test("keeps the profile menu open when another profile is added or removed", async ({
    tabletTestPage,
    apiClient,
    seedData,
  }) => {
    const { agent, profile } = await seededAgentProfile(apiClient, seedData.agentProfileId);
    const extraName = "Temporary profile for menu refresh";
    let extraId: string | undefined;
    try {
      await tabletTestPage.goto("/settings/agents");
      await tabletTestPage.waitForFunction(
        () =>
          Reflect.get(window, "__KANDEV_E2E_STORE__")?.getState().connection.status === "connected",
      );
      const row = tabletTestPage.getByTestId("agent-profile-row").filter({ hasText: profile.name });
      const trigger = row.getByTestId(`profile-actions-menu-${profile.id}`);
      await trigger.tap();
      await expect(trigger).toHaveAttribute("aria-expanded", "true");
      await expect(tabletTestPage.getByTestId(`duplicate-profile-${profile.id}`)).toBeVisible();

      const extra = await apiClient.createAgentProfile(agent.id, extraName, {
        model: profile.model || "mock-fast",
      });
      extraId = extra.id;
      const extraRow = tabletTestPage
        .getByTestId("agent-profile-row")
        .filter({ hasText: extraName });
      await expect(extraRow).toHaveCount(1);
      await expect(trigger).toHaveAttribute("aria-expanded", "true");
      await apiClient.deleteAgentProfile(extraId, true);
      extraId = undefined;
      await expect(extraRow).toHaveCount(0);
      await expect(trigger).toHaveAttribute("aria-expanded", "true");
      await expect(tabletTestPage.getByTestId(`delete-profile-${profile.id}`)).toBeVisible();
    } finally {
      if (extraId) await apiClient.deleteAgentProfile(extraId, true);
    }
  });

  test("duplicates from the profile page header and navigates to the copy", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { agent, profile } = await seededAgentProfile(apiClient, seedData.agentProfileId);
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
