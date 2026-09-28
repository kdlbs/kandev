import type { ApiClient } from "./api-client";

export async function createCursorMcpAuthFixture(apiClient: ApiClient) {
  const suffix = Date.now().toString(36);
  const agent = await apiClient.createCustomTUIAgent({
    display_name: `Cursor MCP Auth ${suffix}`,
    command: "echo",
    model: "mock-fast",
    mcp_strategy: "cursor",
  });
  const profile = agent.profiles[0];
  if (!profile) throw new Error("Cursor MCP auth fixture has no seeded profile");

  return {
    agentName: agent.name,
    profileId: profile.id,
    async dispose() {
      await apiClient.deleteCustomAgentByName(agent.name);
    },
  };
}
