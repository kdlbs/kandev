import type { ApiClient } from "./api-client";

/**
 * Seed a concrete Copilot profile for plugin selection tests. The Copilot agent
 * row is built in; the spec skips when an E2E runtime does not register it.
 */
export async function createCopilotPluginFixture(apiClient: ApiClient) {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((candidate) => candidate.name === "copilot-acp");
  if (!agent) return null;
  const suffix = Date.now().toString(36);
  const profile = await apiClient.createAgentProfile(agent.id, `Copilot plugins ${suffix}`, {
    model: "auto",
    copilot_selected_plugins: [],
  });
  return {
    agentId: agent.id,
    agentName: agent.name,
    profileId: profile.id,
    profileName: profile.name,
    async dispose() {
      await apiClient.deleteAgentProfile(profile.id, true);
    },
  };
}

export function copilotPluginDiscovery(agentId: string, profileId: string) {
  return {
    agent_id: agentId,
    profile_id: profileId,
    provider_id: "copilot-acp",
    status: "ready",
    runtime_version: "1.0.88",
    mount_permitted: false,
    plugins: [
      {
        id: "installed:copilot-plugins/spark",
        name: "spark",
        marketplace: "copilot-plugins",
        version: "1.0.0",
        native_enabled: false,
        availability: "available",
      },
      {
        id: "live:fixture-market/harness",
        name: "harness",
        marketplace: "fixture-market",
        version: "1.2.0",
        native_enabled: true,
        availability: "available",
      },
      {
        id: "builtin:/browser-use",
        name: "browser-use",
        version: "0.0.4",
        native_enabled: false,
        availability: "unavailable",
        reason_code: "builtin_not_mountable",
      },
    ],
  };
}
