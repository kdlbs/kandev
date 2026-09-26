import { test, expect } from "../../fixtures/test-base";

/**
 * Phone variant of `agent-host-cli.spec.ts`: same discovery note and custom
 * model entry, through the viewport-contained popover the shared selector
 * already uses on narrow viewports.
 */
test.describe("Agent host CLI model discovery on mobile", () => {
  test("shows the discovery note and accepts a typed custom model ID", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((a) => a.name === "mock-agent") ?? agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Mobile host CLI test", {
      model: "mock-fast",
    });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);

      const trigger = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(trigger).toBeVisible({ timeout: 15_000 });

      await testPage.getByTestId("profile-refresh-capabilities").click();
      await expect(testPage.getByTestId("model-discovery-note")).toBeVisible({ timeout: 15_000 });

      await trigger.click();
      const customModelId = "claude-opus-5-5";
      await testPage.getByPlaceholder("Filter models...").fill(customModelId);
      const customRow = testPage.getByTestId("model-config-custom-row");
      await expect(customRow).toBeVisible();
      await customRow.click();

      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      const stored = await apiClient.getAgentProfile(profile.id);
      expect(stored.model).toBe(customModelId);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });
});
