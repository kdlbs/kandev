import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";

async function updateFixtureConfig(
  apiClient: import("../../helpers/api-client").ApiClient,
  profileID: string,
): Promise<void> {
  const response = await apiClient.rawRequest("PATCH", `/api/plugins/${PLUGIN_ID}`, {
    config: {
      api_token: "utility-invocation-e2e-token",
      agent_profile: profileID,
    },
  });
  const body = await response.text();
  expect(response.ok, body).toBe(true);
}

async function invokeUtilityAction(
  apiClient: import("../../helpers/api-client").ApiClient,
  action: "utility-default" | "utility-preference",
  workspaceID: string,
): Promise<{ status: number; body: string }> {
  const response = await apiClient.rawRequest(
    "POST",
    `/api/plugins/${PLUGIN_ID}/actions/${action}`,
    { workspaceId: workspaceID },
  );
  return { status: response.status, body: await response.text() };
}

function utilityResponse(body: string): string {
  return (JSON.parse(body) as { response: string }).response;
}

test.describe("Packaged plugin utility invocation", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
    await apiClient
      .saveUserSettings({ default_utility_agent_profile_id: "" })
      .catch(() => undefined);
  });

  test("uses the default or the caller's explicit profile across the real plugin transport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const createdProfileIDs: string[] = [];

    try {
      await installFixturePlugin(testPage);

      const { agents } = await apiClient.listAgents();
      const owner =
        agents.find((agent) =>
          agent.profiles?.some((profile) => profile.id === seedData.agentProfileId),
        ) ?? agents.find((agent) => agent.id !== "dynamic");
      if (!owner) throw new Error("mock agent owner was not available for utility profiles");

      const profileFast = await apiClient.createAgentProfile(owner.id, "E2E utility fast", {
        model: "mock-fast",
      });
      const profileSmart = await apiClient.createAgentProfile(owner.id, "E2E utility smart", {
        model: "mock-smart",
      });
      const profileSlow = await apiClient.createAgentProfile(owner.id, "E2E utility slow", {
        model: "mock-slow",
      });
      createdProfileIDs.push(profileFast.id, profileSmart.id, profileSlow.id);

      await apiClient.saveUserSettings({
        default_utility_agent_profile_id: profileFast.id,
      });
      await updateFixtureConfig(apiClient, profileSmart.id);

      const defaultCall = await invokeUtilityAction(
        apiClient,
        "utility-default",
        seedData.workspaceId,
      );
      expect(defaultCall.status, defaultCall.body).toBe(200);
      expect(utilityResponse(defaultCall.body)).toBe("utility profile model: mock-fast");

      const explicitCall = await invokeUtilityAction(
        apiClient,
        "utility-preference",
        seedData.workspaceId,
      );
      expect(explicitCall.status, explicitCall.body).toBe(200);
      expect(utilityResponse(explicitCall.body)).toBe("utility profile model: mock-smart");

      await updateFixtureConfig(apiClient, "");
      const clearedPreferenceCall = await invokeUtilityAction(
        apiClient,
        "utility-preference",
        seedData.workspaceId,
      );
      expect(clearedPreferenceCall.status, clearedPreferenceCall.body).toBe(200);
      expect(utilityResponse(clearedPreferenceCall.body)).toBe("utility profile model: mock-fast");

      await apiClient.saveUserSettings({
        default_utility_agent_profile_id: profileSlow.id,
      });
      const changedDefaultCall = await invokeUtilityAction(
        apiClient,
        "utility-preference",
        seedData.workspaceId,
      );
      expect(changedDefaultCall.status, changedDefaultCall.body).toBe(200);
      expect(utilityResponse(changedDefaultCall.body)).toBe("utility profile model: mock-slow");

      await updateFixtureConfig(apiClient, profileSmart.id);
      await apiClient.saveUserSettings({ default_utility_agent_profile_id: "" });
      const explicitWithoutDefaultCall = await invokeUtilityAction(
        apiClient,
        "utility-preference",
        seedData.workspaceId,
      );
      expect(explicitWithoutDefaultCall.status, explicitWithoutDefaultCall.body).toBe(200);
      expect(utilityResponse(explicitWithoutDefaultCall.body)).toBe(
        "utility profile model: mock-smart",
      );

      await apiClient.updateAgentProfile(profileSmart.id, { enabled: false });
      const invalidExplicitCall = await invokeUtilityAction(
        apiClient,
        "utility-preference",
        seedData.workspaceId,
      );
      expect(invalidExplicitCall.status).toBeGreaterThanOrEqual(400);

      await apiClient.saveUserSettings({
        default_utility_agent_profile_id: profileSlow.id,
      });
      const defaultAfterInvalidOverride = await invokeUtilityAction(
        apiClient,
        "utility-default",
        seedData.workspaceId,
      );
      expect(defaultAfterInvalidOverride.status, defaultAfterInvalidOverride.body).toBe(200);
      expect(utilityResponse(defaultAfterInvalidOverride.body)).toBe(
        "utility profile model: mock-slow",
      );
    } finally {
      await apiClient
        .saveUserSettings({ default_utility_agent_profile_id: "" })
        .catch(() => undefined);
      for (const profileID of createdProfileIDs) {
        await apiClient.deleteAgentProfile(profileID, true).catch(() => undefined);
      }
    }
  });
});
