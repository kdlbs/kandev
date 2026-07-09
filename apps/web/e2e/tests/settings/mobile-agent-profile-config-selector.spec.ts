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
});
