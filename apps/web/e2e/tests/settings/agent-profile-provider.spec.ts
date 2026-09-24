import { test, expect } from "../../fixtures/test-base";

/**
 * OpenAI-compatible provider section of the agent profile editor.
 *
 * - The section renders only for a provider-supported agent (mock-agent
 *   advertises the capability).
 * - Switching to "OpenAI-compatible provider" reveals the base URL + key
 *   fields; an empty/relative base URL blocks save with an inline error.
 * - A valid base URL saves and survives reload (asserted via the API).
 * - Switching back to "Native" hides the fields and the saved payload clears
 *   the base URL.
 */
test.describe("Agent profile — OpenAI-compatible provider", () => {
  test("configure, validate, persist, and clear the provider section", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((a) => a.name === "mock-agent") ?? agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Provider section test", {
      model: "mock-fast",
    });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);

      const kindSelect = testPage.getByTestId("provider-kind-select");
      await expect(kindSelect).toBeVisible({ timeout: 15_000 });

      // Native by default: no base URL field.
      await expect(testPage.getByTestId("provider-base-url-input")).toBeHidden();

      // Switch to OpenAI-compatible.
      await kindSelect.click();
      await testPage.getByRole("option", { name: /OpenAI-compatible provider/i }).click();

      const baseUrl = testPage.getByTestId("provider-base-url-input");
      await expect(baseUrl).toBeVisible();

      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();

      // Relative URL blocks save with an inline error.
      await baseUrl.fill("/v1");
      await expect(testPage.getByTestId("provider-base-url-error")).toBeVisible();
      await expect(saveButton).toBeDisabled();

      // Valid absolute URL clears the block.
      await baseUrl.fill("http://localhost:20128/v1");
      await expect(testPage.getByTestId("provider-base-url-error")).toBeHidden();
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      await testPage.reload();
      await expect(testPage.getByTestId("provider-base-url-input")).toHaveValue(
        "http://localhost:20128/v1",
        { timeout: 15_000 },
      );

      let stored = await apiClient.getAgentProfile(profile.id);
      expect(stored.providerKind).toBe("openai_compatible");
      expect(stored.providerBaseUrl).toBe("http://localhost:20128/v1");

      // Switch back to Native: fields disappear and the payload clears.
      await testPage.getByTestId("provider-kind-select").click();
      await testPage.getByRole("option", { name: /Native/i }).click();
      await expect(testPage.getByTestId("provider-base-url-input")).toBeHidden();

      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      stored = await apiClient.getAgentProfile(profile.id);
      expect(stored.providerKind ?? "").toBe("");
      expect(stored.providerBaseUrl ?? "").toBe("");
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });
});
