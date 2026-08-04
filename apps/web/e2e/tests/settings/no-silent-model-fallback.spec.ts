import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

// The mock agent advertises `mock-fast` / `mock-smart`; any other model id
// is "gone" (configured but no longer advertised) — the scenario this
// feature makes explicit instead of silently falling back.
const GONE_MODEL = "claude-gone";

useRegularMode();

test.describe("No silent model fallback", () => {
  test("profile editor keeps a gone start model, shows it red, and toggles the fallback row", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent) throw new Error("no agents available");
    const profile = await apiClient.createAgentProfile(agent.id, "Gone Start Model", {
      model: GONE_MODEL,
    });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);

      // The start-model trigger keeps the gone model visible and red.
      const trigger = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(trigger).toBeVisible({ timeout: 15_000 });
      await expect(trigger).toContainText(GONE_MODEL);
      await expect(trigger).toHaveClass(/text-destructive/);

      // Strict mode (no toggle): the explicit fallback row is visible.
      const fallbackField = testPage.getByTestId("profile-fallback-model-field");
      await expect(fallbackField).toBeVisible({ timeout: 10_000 });

      // Enabling "Fallback automatically to next model" hides the fallback row.
      const toggle = testPage.getByTestId("profile-auto-fallback-field").getByRole("switch");
      await toggle.click();
      await expect(fallbackField).toBeHidden({ timeout: 10_000 });

      // The gone model survives a save (no auto-heal on save).
      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });

  test("task-create picker blocks a strict-gone profile and warns for a fallback profile", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(90_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent) throw new Error("no agents available");
    const strictProfile = await apiClient.createAgentProfile(agent.id, "Gone Strict", {
      model: GONE_MODEL,
    });
    const fallbackProfile = await apiClient.createAgentProfile(agent.id, "Gone Fallback", {
      model: GONE_MODEL,
      fallback_model: "mock-fast",
    });

    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();

      await testPage.getByTestId("task-title-input").fill("Gone model picker test");
      await testPage.getByTestId("task-description-input").fill("verify gating");

      // Open the agent profile dropdown.
      const selector = dialog.getByTestId("agent-profile-selector");
      await selector.click();

      // Strict-gone profile: greyed out and unselectable.
      const strictOption = testPage.getByRole("option", { name: /Gone Strict/ });
      await expect(strictOption).toBeVisible({ timeout: 15_000 });
      await expect(strictOption).toHaveAttribute("aria-disabled", "true");

      // Fallback profile: selectable and carries the "fallback will be used" warning.
      const fallbackOption = testPage.getByRole("option", { name: /Gone Fallback/ });
      await expect(fallbackOption).toBeVisible({ timeout: 15_000 });
      await expect(fallbackOption).not.toHaveAttribute("aria-disabled", "true");
      await expect(fallbackOption.getByRole("img", { name: /mock-fast/ })).toBeVisible();
    } finally {
      await apiClient.deleteAgentProfile(strictProfile.id, true);
      await apiClient.deleteAgentProfile(fallbackProfile.id, true);
    }
  });
});
