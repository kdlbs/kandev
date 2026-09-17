import { fetchJson } from "../client";
import { createAgentProfile, listAgentProfiles } from "./office-api";
import { getWorkspaceRouting } from "./office-routing-api";
import type { AgentRole } from "@/lib/types/agent-profile";

const base = (id: string) => `/api/v1/office/workspaces/${encodeURIComponent(id)}`;
export const getWorkspaceChief = (id: string) =>
  fetchJson<{ agent_id: string }>(`${base(id)}/chief`);
export const setWorkspaceChief = (id: string, agentId: string) =>
  fetchJson(`${base(id)}/chief`, {
    init: { method: "PUT", body: JSON.stringify({ agent_id: agentId }) },
  });
export const enableWorkspaceAgents = (id: string) =>
  fetchJson<{ enabled: boolean }>(`${base(id)}/agents/enable`, {
    init: { method: "POST" },
  });
export const openAgentConversation = (workspaceId: string, agentId: string) =>
  fetchJson<{ channel: { task_id: string } }>(
    `${base(workspaceId)}/agents/${encodeURIComponent(agentId)}/conversation`,
    { init: { method: "POST" } },
  );
export async function loadWorkspaceAgents(id: string) {
  const [agents, chief] = await Promise.all([listAgentProfiles(id), getWorkspaceChief(id)]);
  return { agents: agents.agents, chiefId: chief.agent_id };
}
export { getWorkspaceRouting };
export async function connectWorkspaceAgent(
  workspaceId: string,
  input: {
    delegationContext: string;
    name: string;
    role: AgentRole;
    profileId: string;
    executorId: string;
    executorType: string;
  },
) {
  return createAgentProfile(workspaceId, {
    name: input.name,
    delegationContext: input.delegationContext,
    role: input.role,
    executionProfileId: input.profileId,
    maxConcurrentSessions: 1,
    executorPreference: { type: input.executorType, executor_profile_id: input.executorId },
  });
}
