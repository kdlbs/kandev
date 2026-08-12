import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

// Mobile companion to no-silent-model-fallback.spec.ts: the task-create
// picker's fallback explanation must be discoverable on touch — visible
// secondary text, not a hover-only tooltip (icon title).
const GONE_MODEL = "claude-gone";

useRegularMode();

test.describe("No silent model fallback on mobile", () => {
  test("profile fallback settings stack and open help in a touch drawer", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent) throw new Error("no agents available");
    const profile = await apiClient.createAgentProfile(agent.id, "Mobile Fallback Settings", {
      model: "mock-fast",
      fallback_model: "mock-smart",
    });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);

      const fallbackTrigger = testPage.getByTestId("profile-fallback-settings-trigger");
      await expect(fallbackTrigger).toBeVisible({ timeout: 15_000 });
      await expect(fallbackTrigger).toHaveAttribute("aria-expanded", "false");
      await expect(testPage.getByTestId("profile-fallback-settings-summary")).toContainText(
        "Explicit fallback: mock-smart",
      );

      await fallbackTrigger.tap();
      const autoOption = testPage.getByTestId("profile-auto-fallback-option");
      const explicitOption = testPage.getByTestId("profile-explicit-fallback-option");
      await expect(autoOption).toBeVisible();
      await expect(explicitOption).toBeVisible();

      const autoBox = await autoOption.boundingBox();
      const explicitBox = await explicitOption.boundingBox();
      expect(autoBox).not.toBeNull();
      expect(explicitBox).not.toBeNull();
      if (!autoBox || !explicitBox) throw new Error("fallback option cards are not laid out");
      expect(explicitBox.y).toBeGreaterThan(autoBox.y + 8);

      const helpButton = testPage.getByTestId("profile-automatic-fallback-help");
      const helpBox = await helpButton.boundingBox();
      expect(helpBox).not.toBeNull();
      if (!helpBox) throw new Error("automatic fallback help button is not laid out");
      expect(helpBox.width).toBeGreaterThanOrEqual(44);
      expect(helpBox.height).toBeGreaterThanOrEqual(44);

      // The disclosure finishes a height transition when its children mount;
      // keep the interaction touch-native while bypassing that transient
      // stability window.
      await helpButton.tap({ force: true });
      const helpDrawer = testPage.getByRole("dialog");
      await expect(helpDrawer).toContainText("Automatic fallback");
      await expect(helpDrawer).toContainText(/remaining advertised models/);
      await testPage.keyboard.press("Escape");
      await expect(helpDrawer).toBeHidden();

      const advancedOptions = testPage.getByTestId("profile-advanced-options");
      const advancedTrigger = testPage.getByTestId("profile-advanced-options-trigger");
      await expect(advancedTrigger).toBeVisible({ timeout: 10_000 });
      await expect(testPage.getByTestId("command-prefix-input")).toBeHidden();
      const fallbackBox = await testPage.getByTestId("profile-fallback-settings").boundingBox();
      const advancedBox = await advancedOptions.boundingBox();
      expect(fallbackBox).not.toBeNull();
      expect(advancedBox).not.toBeNull();
      if (!fallbackBox || !advancedBox) throw new Error("profile disclosures are not laid out");
      expect(advancedBox.y - (fallbackBox.y + fallbackBox.height)).toBeLessThanOrEqual(8);

      await advancedTrigger.tap();
      await expect(testPage.getByTestId("command-prefix-input")).toBeVisible();

      const overflow = await testPage.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth,
        clientWidth: document.documentElement.clientWidth,
      }));
      expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });

  test("task-create picker shows the fallback note as visible text on touch", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(90_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent) throw new Error("no agents available");
    const fallbackProfile = await apiClient.createAgentProfile(agent.id, "Gone Fallback Mobile", {
      model: GONE_MODEL,
      fallback_model: "mock-fast",
    });

    try {
      // Mobile kanban opens the create-task dialog from the floating action
      // button, not the desktop sidebar button.
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      const fab = testPage.getByTestId("mobile-fab");
      await expect(fab).toBeVisible({ timeout: 15_000 });
      await fab.tap();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();

      await testPage.getByTestId("task-title-input").fill("Gone model picker mobile test");
      await testPage.getByTestId("task-description-input").fill("verify visible fallback note");

      const selector = dialog.getByTestId("agent-profile-selector");
      await selector.tap();

      const fallbackOption = testPage.getByRole("option", { name: /Gone Fallback Mobile/ });
      await expect(fallbackOption).toBeVisible({ timeout: 15_000 });
      await expect(fallbackOption).not.toHaveAttribute("aria-disabled", "true");

      // The fallback explanation must be readable without hover: the
      // picker renders it as visible secondary text inside the option.
      await expect(fallbackOption).toContainText(/mock-fast/);
      await expect(fallbackOption).toContainText(/no longer available/);
    } finally {
      await apiClient.deleteAgentProfile(fallbackProfile.id, true);
    }
  });
});
