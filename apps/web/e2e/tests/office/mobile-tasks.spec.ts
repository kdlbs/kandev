import { test, expect } from "../../fixtures/office-fixture";
import type { OfficeApiClient } from "../../helpers/office-api-client";

const ROUTING_PROFILE_WAIT_MS = 20_000;

test.describe("Tasks on mobile", () => {
  test("cleans only the stale profile from workspace routing", async ({
    apiClient,
    backend,
    officeApi,
    officeSeed,
    seedData,
  }) => {
    await backend.restart({ KANDEV_MOCK_PROVIDERS: "claude-acp,codex-acp" });
    const { agents } = await apiClient.listAgents();
    const claudeAgent = agents.find((agent) => agent.name === "claude-acp");
    const codexAgent = agents.find((agent) => agent.name === "codex-acp");
    const claudeProfile = claudeAgent?.profiles[0];
    const codexProfile = codexAgent?.profiles[0];
    if (!claudeAgent || !claudeProfile || !codexAgent || !codexProfile) {
      throw new Error("E2E seed has no Claude and Codex profiles");
    }

    const profile = await apiClient.createAgentProfile(claudeAgent.id, "Routing cleanup profile", {
      model: claudeProfile.model,
    });
    const preservedProfile = await apiClient.createAgentProfile(
      codexAgent.id,
      "Preserved routing profile",
      {
        model: codexProfile.model,
      },
    );
    const routingProfile = await waitForRoutingProfile(
      officeApi,
      officeSeed.workspaceId,
      profile.id,
    );
    const preservedRoutingProfile = await waitForRoutingProfile(
      officeApi,
      officeSeed.workspaceId,
      preservedProfile.id,
    );
    await officeApi.updateRouting(officeSeed.workspaceId, {
      enabled: true,
      provider_order: ["claude-acp", "codex-acp"],
      default_tier: "balanced",
      provider_profiles: {
        "claude-acp": {
          tier_map: { balanced: routingProfile.model },
          execution_profile_ids: { balanced: routingProfile.id },
        },
        "codex-acp": {
          tier_map: { balanced: preservedRoutingProfile.model },
          execution_profile_ids: { balanced: preservedRoutingProfile.id },
        },
      },
    });

    await expect(apiClient.deleteAgentProfile(profile.id, true)).rejects.toThrow("routing_tiers");

    await apiClient.cleanupTestProfiles([seedData.agentProfileId, preservedProfile.id]);

    const remainingProfiles = (await apiClient.listAgents()).agents.flatMap(
      (agent) => agent.profiles ?? [],
    );
    expect(remainingProfiles.map((item) => item.id)).not.toContain(profile.id);

    const routing = await officeApi.getRouting(officeSeed.workspaceId);
    expect(routing.config.enabled).toBe(false);
    expect(routing.config.provider_order).toEqual(["claude-acp", "codex-acp"]);
    expect(routing.config.provider_profiles["codex-acp"]).toEqual({
      tier_map: { balanced: preservedRoutingProfile.model },
      execution_profile_ids: { balanced: preservedRoutingProfile.id },
    });
  });

  test("does not mutate routing for malformed or unrelated profile conflicts", async ({
    apiClient,
  }) => {
    const client = apiClient as unknown as TestableApiClient;
    const rawRequest = client.rawRequest;
    const request = client.request;
    let putRequests = 0;
    try {
      client.rawRequest = async () => new Response("not JSON", { status: 409 });
      await expect(client.deleteTestProfile("target-profile")).rejects.toThrow("not JSON");

      client.rawRequest = async () =>
        new Response(JSON.stringify({ routing_tiers: [{ workspace_id: "workspace-1" }] }), {
          status: 409,
        });
      client.request = async (method) => {
        if (method === "PUT") putRequests += 1;
        return {
          config: {
            enabled: true,
            provider_order: ["claude-acp"],
            default_tier: "balanced",
            provider_profiles: {
              "claude-acp": {
                execution_profile_ids: { balanced: "different-profile" },
              },
            },
          },
        };
      };
      await expect(client.deleteTestProfile("target-profile")).rejects.toThrow("routing_tiers");
      expect(putRequests).toBe(0);
    } finally {
      client.rawRequest = rawRequest;
      client.request = request;
    }
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("shows subtasks expanded by default", async ({ testPage, apiClient, officeSeed }) => {
    const parentTitle = "Mobile Expanded Parent";
    const childTitle = "Mobile Expanded Child";
    const parent = await apiClient.createTask(officeSeed.workspaceId, parentTitle, {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.createTask(officeSeed.workspaceId, childTitle, {
      workflow_id: officeSeed.workflowId,
      parent_id: parent.id,
    });

    await testPage.goto("/office/tasks");

    await expect(testPage.getByText(parentTitle)).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText(childTitle)).toBeVisible();
  });
});

async function waitForRoutingProfile(
  officeApi: OfficeApiClient,
  workspaceId: string,
  profileId: string,
): Promise<{ id: string; model: string }> {
  const deadline = Date.now() + ROUTING_PROFILE_WAIT_MS;
  while (Date.now() < deadline) {
    const routing = await officeApi.getRouting(workspaceId);
    const profile = routing.execution_profiles.find((candidate) => candidate.id === profileId);
    if (profile) return profile;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(
    `Routing profile ${profileId} did not appear within ${ROUTING_PROFILE_WAIT_MS}ms`,
  );
}

type TestableApiClient = {
  deleteTestProfile(profileId: string): Promise<void>;
  rawRequest(method: string, path: string): Promise<Response>;
  request(method: string, path: string, body?: unknown): Promise<unknown>;
};
