import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile agent profile config selector", () => {
  test("changes a dynamic profile config option", async ({ testPage, apiClient, backend }) => {
    test.setTimeout(60_000);

    await expect
      .poll(
        async () => {
          const resp = await testPage.request.get(`${backend.baseUrl}/api/v1/agents/available`);
          if (!resp.ok()) return false;
          const data = (await resp.json()) as {
            agents?: {
              name: string;
              model_config?: { config_options?: { id: string }[] };
            }[];
          };
          const mock = data.agents?.find((a) => a.name === "mock-agent");
          return Boolean(
            mock?.model_config?.config_options?.some((option) => option.id === "effort"),
          );
        },
        { timeout: 20_000, intervals: [250, 500, 1000] },
      )
      .toBe(true);

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Mobile Config Option Profile", {
      model: "mock-fast",
      config_options: { effort: "medium" },
    });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      const selector = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(selector).toBeVisible({ timeout: 15_000 });
      await selector.click();
      const effortTrigger = testPage.getByTestId("config-option-trigger-effort");
      await expect(effortTrigger).toBeVisible();
      await effortTrigger.click();
      await testPage.getByRole("button", { name: "High", exact: true }).click();
      await expect(selector).toContainText("High");
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });

  test("sets and saves a command prefix on the agent profile page", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(90_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Mobile Command Prefix Profile", {
      model: "mock-fast",
    });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);

      const prefixInput = testPage.getByTestId("command-prefix-input");
      await expect(prefixInput).toBeVisible({ timeout: 15_000 });
      await prefixInput.fill("greywall --");

      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      // Reload — the saved prefix must still be there.
      await testPage.reload();
      await expect(testPage.getByTestId("command-prefix-input")).toHaveValue("greywall --", {
        timeout: 15_000,
      });

      // API-path verification confirms the normalized persisted value.
      const stored = await apiClient.getAgentProfile(profile.id);
      expect(stored.commandPrefix).toBe("greywall --");

      // Clearing a previously-saved prefix must persist an empty value.
      await testPage.getByTestId("command-prefix-input").fill("");
      const saveButtonAfterClear = testPage
        .getByRole("button", { name: /^Save( changes)?$/i })
        .first();
      await expect(saveButtonAfterClear).toBeEnabled({ timeout: 10_000 });
      await saveButtonAfterClear.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      await testPage.reload();
      await expect(testPage.getByTestId("command-prefix-input")).toHaveValue("", {
        timeout: 15_000,
      });
      const storedAfterClear = await apiClient.getAgentProfile(profile.id);
      expect(storedAfterClear.commandPrefix ?? "").toBe("");
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });
});
