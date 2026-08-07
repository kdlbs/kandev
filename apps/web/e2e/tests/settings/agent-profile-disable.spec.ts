import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

/**
 * Verifies the profile enable/disable flow:
 *
 * - The profile settings header toggle persists across reload.
 * - A disabled profile is hidden from the new-task dialog agent selector.
 * - The /settings/agents profile list shows the toggle and re-enabling there
 *   restores the profile to task creation.
 *
 * Uses the seeded default profile (like agent-profile-acp.spec.ts) rather
 * than creating one — the profile editor page reads from the agents list
 * that is hydrated on the server, and a freshly POSTed profile
 * race-conditions with SSR.
 */
test.describe("Agent profile — enable/disable", () => {
  test("header toggle persists and disabled profile is hidden from task creation", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = agent.profiles[0];
    const otherProfiles = agents
      .flatMap((a) => a.profiles ?? [])
      .filter((p) => p.id !== profile.id && !p.workspaceId && p.enabled !== false);

    try {
      // 1. Disable via the profile settings header toggle and save.
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      const headerToggle = testPage.getByTestId("profile-enabled-toggle");
      await expect(headerToggle).toBeVisible({ timeout: 15_000 });
      await expect(headerToggle).toHaveAttribute("data-state", "checked");
      await headerToggle.click();
      await expect(headerToggle).toHaveAttribute("data-state", "unchecked");

      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i });
      await expect(saveButton).toHaveCount(1);
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      // 2. Reload — the toggle must reflect the persisted disabled state.
      await testPage.reload();
      await expect(testPage.getByTestId("profile-enabled-toggle")).toHaveAttribute(
        "data-state",
        "unchecked",
        { timeout: 15_000 },
      );

      // 3. The disabled profile is not offered in the new-task dialog.
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await testPage.reload({ waitUntil: "networkidle" });
      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible({ timeout: 15_000 });
      await testPage.getByTestId("task-title-input").fill("Disable profile e2e");
      await testPage
        .getByTestId("task-description-input")
        .fill("verifies disabled profile is hidden");

      // The disabled profile must not be offered as a choice. With the
      // seeded profile the only profile, the dialog renders the
      // no-compatible-agent state instead of a selector; with other enabled
      // profiles the selector lists them but never the disabled one.
      const agentSelector = testPage.getByTestId("agent-profile-selector");
      const emptyState = testPage.getByTestId("agent-profile-empty-state");
      if (otherProfiles.length === 0) {
        await expect(emptyState).toBeVisible({ timeout: 15_000 });
        await expect(emptyState).toContainText("No compatible agent profiles");
        await expect(agentSelector).toHaveCount(0);
      } else {
        await expect(agentSelector).toBeVisible({ timeout: 15_000 });
        await agentSelector.click();
        const listbox = testPage.getByRole("listbox");
        await expect(listbox.getByRole("option", { name: profile.name, exact: false })).toHaveCount(
          0,
        );
        const other = otherProfiles[0];
        await expect(listbox.getByRole("option", { name: other.name, exact: false })).toHaveCount(
          1,
        );
      }
    } finally {
      // Always restore so worker-scoped seedData stays valid for later tests.
      await apiClient.updateAgentProfile(profile.id, { enabled: true }).catch(() => {});
    }
  });

  test("settings list toggle disables and re-enables the profile", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(90_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = agent.profiles[0];

    try {
      await testPage.goto("/settings/agents");
      const rowToggle = testPage.getByTestId(`profile-enabled-toggle-${profile.id}`);
      await expect(rowToggle).toBeVisible({ timeout: 15_000 });
      await expect(rowToggle).toHaveAttribute("data-state", "checked");

      // Disabling from the list saves immediately (no save button) and must
      // not navigate away from the page.
      await rowToggle.click();
      await expect(rowToggle).toHaveAttribute("data-state", "unchecked");
      await expect(testPage).toHaveURL(/\/settings\/agents$/);

      // Persisted across reload.
      await testPage.reload();
      await expect(testPage.getByTestId(`profile-enabled-toggle-${profile.id}`)).toHaveAttribute(
        "data-state",
        "unchecked",
        { timeout: 15_000 },
      );

      // Re-enable from the list and confirm the state flips back.
      await testPage.getByTestId(`profile-enabled-toggle-${profile.id}`).click();
      await expect(testPage.getByTestId(`profile-enabled-toggle-${profile.id}`)).toHaveAttribute(
        "data-state",
        "checked",
      );

      // Re-enabling restores the profile to task creation. With a single
      // enabled profile the dialog auto-selects it and hides the selector;
      // with multiple profiles it appears as an option in the open listbox.
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await testPage.reload({ waitUntil: "networkidle" });
      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible({ timeout: 15_000 });
      const selector = testPage.getByTestId("agent-profile-selector");
      if (await selector.count()) {
        await selector.click();
        await expect(
          testPage.getByRole("listbox").getByRole("option", { name: profile.name, exact: false }),
        ).toHaveCount(1);
      } else {
        await expect(testPage.getByTestId("agent-profile-empty-state")).toHaveCount(0);
      }
    } finally {
      await apiClient.updateAgentProfile(profile.id, { enabled: true }).catch(() => {});
    }
  });
});
