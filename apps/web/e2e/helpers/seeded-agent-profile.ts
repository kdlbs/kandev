import type { ApiClient } from "./api-client";

export async function seededAgentProfile(apiClient: ApiClient, profileId: string) {
  const { agents } = await apiClient.listAgents();
  for (const agent of agents) {
    const profile = agent.profiles.find((candidate) => candidate.id === profileId);
    if (profile) return { agent, profile };
  }
  throw new Error(`Seed agent profile ${profileId} was not found`);
}
