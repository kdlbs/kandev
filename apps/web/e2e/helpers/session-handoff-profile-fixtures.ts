import type { ApiClient } from "./api-client";
import { getMockAgent } from "./agent-fixtures";

export async function createHandoffProfiles(apiClient: ApiClient) {
  const { agents } = await apiClient.listAgents();
  const agent = getMockAgent(agents);
  const agentId = agent.id;
  const profileA = await apiClient.createAgentProfile(agentId, "Handoff Filter Profile A", {
    model: "mock-fast",
  });
  const profileB = await apiClient.createAgentProfile(agentId, "Handoff Filter Profile B", {
    model: "mock-slow",
  });
  return { profileA, profileB };
}
