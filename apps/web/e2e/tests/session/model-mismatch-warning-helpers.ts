import type { AgentProfile } from "../../../lib/types/http-agents";
import type { ApiClient } from "../../helpers/api-client";

export const UNADVERTISED_MODEL = "host-only-model";
export const MODEL_VARIATION_BASE = "opus";
export const UNIQUE_MODEL_VARIATION = "opus[1m]";
export const AMBIGUOUS_MODEL_VARIATIONS = ["opus[270k]", "opus[1m, fast]"] as const;
const MODEL_CATALOG_ENV = "MOCK_AGENT_MODEL_CATALOG";

export async function createMismatchedProfile(
  apiClient: ApiClient,
  name: string,
): Promise<AgentProfile> {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
  if (!agent) throw new Error("The E2E fixture must provide a mock agent");
  return apiClient.createAgentProfile(agent.id, name, {
    model: UNADVERTISED_MODEL,
    auto_fallback: true,
  });
}

export async function createStrictMismatchedProfile(
  apiClient: ApiClient,
  name: string,
): Promise<AgentProfile> {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
  if (!agent) throw new Error("The E2E fixture must provide a mock agent");
  return apiClient.createAgentProfile(agent.id, name, { model: UNADVERTISED_MODEL });
}

export async function createModelVariationProfile(
  apiClient: ApiClient,
  name: string,
  catalog: "unique" | "ambiguous",
): Promise<AgentProfile> {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
  if (!agent) throw new Error("The E2E fixture must provide a mock agent");
  return apiClient.createAgentProfile(agent.id, name, {
    model: MODEL_VARIATION_BASE,
    env_vars: [{ key: MODEL_CATALOG_ENV, value: catalog }],
  });
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

export async function createExecutorOnlyModelProfile(apiClient: ApiClient): Promise<AgentProfile> {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent");
  if (!agent) throw new Error("The E2E fixture must provide a mock agent");
  return apiClient.createAgentProfile(agent.id, "Opus High", {
    model: AMBIGUOUS_MODEL_VARIATIONS[0],
    config_options: { effort: "high" },
    fallback_model: "mock-smart",
    auto_fallback: false,
    env_vars: [{ key: MODEL_CATALOG_ENV, value: "ambiguous" }],
  });
}
