import { expect, test } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("Office agent execution profile on mobile", () => {
  test("binds a dynamic profile from the configuration page", async ({
    testPage,
    backend,
    apiClient,
    officeApi,
    officeSeed,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING: "true",
    });
    let dynamicProfileId: string | undefined;

    try {
      await expect
        .poll(
          async () => {
            const { agents } = await apiClient.listAgents();
            return agents.some((agent) =>
              agent.profiles?.some((profile) => profile.id === seedData.agentProfileId),
            );
          },
          { timeout: 15_000 },
        )
        .toBe(true);

      const response = await apiClient.rawRequest("POST", "/api/v1/agents/dynamic/profiles", {
        name: "Mobile Office execution route",
        dynamic: {
          version: 1,
          candidates: [
            {
              position: 0,
              execution_profile_id: seedData.agentProfileId,
              enabled: true,
            },
          ],
        },
      });
      if (!response.ok) {
        throw new Error(
          `Dynamic profile creation failed (${response.status}): ${await response.text()}`,
        );
      }
      dynamicProfileId = ((await response.json()) as { id: string }).id;

      await testPage.goto(`/office/agents/${officeSeed.agentId}/configuration`);
      const picker = testPage.getByTestId("agent-execution-profile-selector");
      await expect(picker).toBeVisible({ timeout: 15_000 });
      const pickerBox = await picker.boundingBox();
      expect(pickerBox).not.toBeNull();
      expect(pickerBox!.height).toBeGreaterThanOrEqual(44);

      await picker.tap();
      const option = testPage.locator(`[data-value="${dynamicProfileId}"]`);
      await expect(option).toBeVisible();
      await option.tap();

      const saved = waitForHttp(
        testPage,
        "PATCH",
        new RegExp(`/api/v1/office/agents/${officeSeed.agentId}$`),
      );
      await testPage.getByRole("button", { name: "Save Configuration" }).tap();
      await saved;

      const agent = await officeApi.getAgent(officeSeed.agentId);
      expect(agent.execution_agent_profile_id).toBe(dynamicProfileId);
    } finally {
      try {
        await officeApi.updateAgent(officeSeed.agentId, {
          agent_profile_id: seedData.agentProfileId,
        });
      } finally {
        try {
          if (dynamicProfileId) await apiClient.deleteAgentProfile(dynamicProfileId, true);
        } finally {
          await releaseFeature();
        }
      }
    }
  });
});
