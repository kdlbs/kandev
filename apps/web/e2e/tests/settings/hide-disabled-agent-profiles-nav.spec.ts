import { test, expect } from "../../fixtures/test-base";
import { getMockAgent } from "../../helpers/agent-fixtures";

// Covers docs/specs/agents/requirements/hide-disabled-profiles-nav.md's nav-visibility
// scenarios: with "Hide disabled profiles from navigation"
// off (the default), a disabled profile still shows in the Settings left
// panel's Agents tree; turning the setting on hides it; re-enabling the
// profile reveals it again — all without a reload.
test.describe("hide disabled agent profiles from left panel navigation", () => {
  test("off by default keeps a disabled profile visible; on hides it; re-enabling reveals it", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);

    const { agents } = await apiClient.listAgents();
    const agent = getMockAgent(agents);
    const profile = await apiClient.createAgentProfile(agent.id, "Navigation visibility profile", {
      model: "mock-fast",
    });
    // The Settings tree's profile leaf is labelled with the profile name and
    // appends the "Disabled" badge while the profile is disabled — an
    // unanchored regex matches both states.
    const profileLink = new RegExp(escapeRegExp(profile.name));

    try {
      // Disable this test's profile via the API. The profile editor toggle
      // itself is covered by agent-profile-disable.spec.ts.
      await apiClient.updateAgentProfile(profile.id, { enabled: false });

      // The Settings sidebar defaults to Accordion tree. Seed the mode before
      // every navigation so this test remains independent of a remembered
      // device preference.
      await testPage.addInitScript(() => {
        // getLocalStorage JSON-parses the value, so the mode must be stored
        // the way setLocalStorage would write it.
        window.localStorage.setItem("kandev.settings.menuMode", JSON.stringify("accordion"));
      });

      await testPage.goto("/settings/agents");
      const settingsTree = testPage.getByTestId("app-sidebar-settings-mode");
      // Accordion opens the route-owned Agents row; the agent node under it
      // (Mock) has no page of its own, so expand it to expose the profile
      // leaves.
      await settingsTree.getByRole("button", { name: "Expand Mock" }).click();
      const disabledLink = settingsTree.getByRole("link", { name: profileLink });
      await expect(disabledLink).toBeVisible({ timeout: 15_000 });

      // The setting lives inside the Agent options surface and is off by default.
      await testPage.getByRole("button", { name: "Options", exact: true }).click();
      const hideDisabledSwitch = testPage.locator("#hide-disabled-agent-profiles-in-nav");
      await expect(hideDisabledSwitch).toHaveAttribute("aria-checked", "false");

      // Turn "hide disabled" on — the tree entry disappears immediately,
      // while the Agents row stays.
      await hideDisabledSwitch.click();
      await expect(hideDisabledSwitch).toHaveAttribute("aria-checked", "true");
      await testPage.getByRole("button", { name: "Done", exact: true }).click();
      await expect(disabledLink).not.toBeVisible();
      await expect(settingsTree.getByRole("link", { name: /^Agents/ })).toBeVisible();

      // Re-enable from the profile editor (the settings list no longer holds
      // the toggle after the PageShell restructure) and confirm the tree
      // entry reappears with the setting still on (no reload).
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      const headerToggle = testPage.getByTestId("profile-enabled-toggle");
      await expect(headerToggle).toBeVisible({ timeout: 15_000 });
      await expect(headerToggle).toHaveAttribute("data-state", "unchecked");
      await headerToggle.click();
      await expect(headerToggle).toHaveAttribute("data-state", "checked");
      await testPage.getByRole("button", { name: /^Save( changes)?$/i }).click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      await testPage.goto("/settings/agents");
      // Fresh page load collapses the agent node again; re-expand it before
      // asserting the revealed profile.
      await settingsTree.getByRole("button", { name: "Expand Mock" }).click();
      await expect(disabledLink).toBeVisible({ timeout: 15_000 });
    } finally {
      await apiClient.deleteAgentProfile(profile.id);
    }
  });
});

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
