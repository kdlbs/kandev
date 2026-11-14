import type { ApiClient } from "./api-client";

export async function seededAgentProfile(apiClient: ApiClient, profileId: string) {
  const { agents } = await apiClient.listAgents();
  const matches = agents.flatMap((agent) =>
    agent.profiles
      .filter((profile) => profile.id === profileId)
      .map((profile) => ({ agent, profile })),
  );
  if (matches.length !== 1) {
    throw new Error(`Seed agent profile ${profileId} must have exactly one owning agent`);
  }
  return matches[0];
}
