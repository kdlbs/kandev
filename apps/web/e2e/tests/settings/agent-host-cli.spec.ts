import { test, expect } from "../../fixtures/test-base";

/**
 * Host CLI model discovery and version display (REQ-AGENTS-HOST-CLI-001,
 * REQ-AGENTS-HOST-CLI-002, REQ-AGENTS-HOST-CLI-004).
 *
 * The mock agent declares a host CLI (`cmd/mock-agent/host_cli.go`) so this
 * exercises the same `--version` and `codex app-server`-shaped `model/list`
 * exchange a real Claude Code or Codex CLI install would answer, without
 * depending on either being installed on the runner.
 */
test.describe("Agent host CLI version and model discovery", () => {
  test("shows the CLI version, the discovery note, and persists a typed custom model", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);

    await testPage.goto("/settings/agents");
    const versionLine = testPage.getByTestId("cli-version-mock-agent");
    await expect(versionLine).toBeVisible({ timeout: 15_000 });
    await expect(versionLine).toContainText("9.9.9");

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((a) => a.name === "mock-agent") ?? agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Host CLI discovery test", {
      model: "mock-fast",
    });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);

      const trigger = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(trigger).toBeVisible({ timeout: 15_000 });

      // A manual refresh forces synchronous discovery so the note below does
      // not race the background warmup.
      await testPage.getByTestId("profile-refresh-capabilities").click();

      const note = testPage.getByTestId("model-discovery-note");
      await expect(note).toBeVisible({ timeout: 15_000 });
      await expect(note).toContainText("9.9.9");

      await trigger.click();
      const customModelId = "claude-opus-5-5";
      await testPage.getByPlaceholder("Filter models...").fill(customModelId);
      const customRow = testPage.getByTestId("model-config-custom-row");
      await expect(customRow).toBeVisible();
      await expect(customRow).toContainText(customModelId);
      await customRow.click();

      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      const stored = await apiClient.getAgentProfile(profile.id);
      expect(stored.model).toBe(customModelId);

      await testPage.reload();
      const reloadedTrigger = testPage.getByRole("button", {
        name: "Profile start model settings",
      });
      await expect(reloadedTrigger).toBeVisible({ timeout: 15_000 });
      await expect(reloadedTrigger).toContainText(customModelId);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });
});
