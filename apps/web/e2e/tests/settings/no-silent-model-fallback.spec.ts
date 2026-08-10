import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

// The mock agent advertises `mock-fast` / `mock-smart`; any other model id
// is "gone" (configured but no longer advertised) — the scenario this
// feature makes explicit instead of silently falling back.
const GONE_MODEL = "claude-gone";

useRegularMode();

test.describe("No silent model fallback", () => {
  test("profile editor explains and persists the fallback settings disclosure", async ({
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

      const fallbackSettings = testPage.getByTestId("profile-fallback-settings");
      const fallbackTrigger = testPage.getByTestId("profile-fallback-settings-trigger");
      await expect(fallbackTrigger).toBeVisible({ timeout: 10_000 });
      await expect(fallbackTrigger).toHaveAttribute("aria-expanded", "false");
      await expect(testPage.getByTestId("profile-fallback-settings-summary")).toContainText(
        "No fallback configured",
      );

      // The fallback options are intentionally hidden until the user opens
      // the disclosure.
      await expect(testPage.getByTestId("profile-fallback-model-field")).toBeHidden();
      await fallbackTrigger.click();
      await expect(fallbackSettings).toHaveAttribute("data-state", "open");

      const fallbackField = testPage.getByTestId("profile-fallback-model-field");
      await expect(fallbackField).toBeVisible({ timeout: 10_000 });

      // Desktop keeps the two options on one row.
      const autoOption = testPage.getByTestId("profile-auto-fallback-option");
      const explicitOption = testPage.getByTestId("profile-explicit-fallback-option");
      const autoBox = await autoOption.boundingBox();
      const explicitBox = await explicitOption.boundingBox();
      expect(autoBox).not.toBeNull();
      expect(explicitBox).not.toBeNull();
      if (!autoBox || !explicitBox) throw new Error("fallback option cards are not laid out");
      expect(Math.abs(autoBox.y - explicitBox.y)).toBeLessThan(8);
      expect(explicitBox.x).toBeGreaterThan(autoBox.x);

      // Fine-pointer users can focus or hover the info buttons for the
      // detailed explanation.
      const automaticHelp = testPage.getByTestId("profile-automatic-fallback-help");
      await automaticHelp.focus();
      await expect(testPage.getByRole("tooltip")).toContainText(/remaining advertised models/);
      await testPage.keyboard.press("Escape");

      // The fallback model is optional: without a value its selector stays
      // hidden behind the attached switch, and opting in reveals it.
      const fallbackSelector = testPage.getByRole("button", {
        name: "Agent fallback model settings",
      });
      await expect(fallbackSelector).toBeHidden({ timeout: 10_000 });
      await fallbackField.getByRole("switch", { name: "Agent fallback" }).click();
      await expect(fallbackSelector).toBeVisible({ timeout: 10_000 });
      await fallbackField.getByRole("switch", { name: "Agent fallback" }).click();
      await expect(fallbackSelector).toBeHidden({ timeout: 10_000 });

      // Enabling automatic fallback keeps the explicit value visible but
      // disables its controls so it cannot be edited accidentally.
      await fallbackField.getByRole("switch", { name: "Agent fallback" }).click();
      await fallbackSelector.click();
      await testPage.getByRole("option", { name: /Mock Fast/ }).click();
      await expect(fallbackSelector).toContainText("Mock Fast");
      const toggle = testPage.getByTestId("profile-auto-fallback-field").getByRole("switch");
      await toggle.click();
      await expect(fallbackField).toBeVisible({ timeout: 10_000 });
      await expect(fallbackField.getByRole("switch", { name: "Agent fallback" })).toBeDisabled();
      await expect(
        fallbackField.getByRole("button", { name: "Agent fallback model settings" }),
      ).toBeDisabled();
      await expect(testPage.getByTestId("profile-fallback-settings-summary")).toContainText(
        "Automatic fallback enabled",
      );

      // The gone model survives a save (no auto-heal on save).
      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      const savedProfile = await apiClient.getAgentProfile(profile.id);
      expect(savedProfile.fallbackModel).toBe("mock-fast");
      expect(savedProfile.autoFallback).toBe(true);

      await testPage.reload();
      await expect(testPage.getByTestId("profile-fallback-settings-trigger")).toHaveAttribute(
        "aria-expanded",
        "false",
      );
      await expect(testPage.getByTestId("profile-fallback-settings-summary")).toContainText(
        "Automatic fallback enabled",
      );
      await testPage.getByTestId("profile-fallback-settings-trigger").click();
      await expect(
        testPage
          .getByTestId("profile-fallback-model-field")
          .getByRole("button", { name: "Agent fallback model settings" }),
      ).toBeDisabled();
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

      // Fallback profile: selectable and carries the "fallback will be used"
      // warning as visible secondary text (touch-discoverable, not just a
      // hover tooltip) plus the alert icon.
      const fallbackOption = testPage.getByRole("option", { name: /Gone Fallback/ });
      await expect(fallbackOption).toBeVisible({ timeout: 15_000 });
      await expect(fallbackOption).not.toHaveAttribute("aria-disabled", "true");
      await expect(fallbackOption.getByRole("img", { name: /mock-fast/ })).toBeVisible();
      await expect(fallbackOption).toContainText(/no longer available/);
    } finally {
      await apiClient.deleteAgentProfile(strictProfile.id, true);
      await apiClient.deleteAgentProfile(fallbackProfile.id, true);
    }
  });
});
