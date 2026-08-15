import type { AgentProfile } from "../../../lib/types/http-agents";
import type { ApiClient } from "../../helpers/api-client";

export const UNADVERTISED_MODEL = "host-only-model";

export async function createMismatchedProfile(
  apiClient: ApiClient,
  name: string,
): Promise<AgentProfile> {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
  if (!agent) throw new Error("The E2E fixture must provide a mock agent");
  return apiClient.createAgentProfile(agent.id, name, { model: UNADVERTISED_MODEL });
}

export async function readModelSelectionWarnings(
  apiClient: ApiClient,
  sessionId: string,
): Promise<
  Array<{
    content: string;
    metadata?: Record<string, unknown>;
  }>
> {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  return messages.filter((message) => message.metadata?.kind === "model_selection_warning");
}
