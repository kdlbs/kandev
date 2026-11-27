import type { AgentProfileOption } from "@/lib/state/slices/settings/types";

export type TaskPair = { agent: string; executor: string };
export type TaskPairTouched = { agent: boolean; executor: boolean };

export type PrefillTaskPairInput = {
  /** `undefined` while loading, `null` once loaded with no default. */
  workspaceDefaultAgentProfileId: string | null | undefined;
  /** `undefined` while the profiles have not loaded. */
  agentProfiles: readonly AgentProfileOption[] | undefined;
  ownAgent: string;
  ownExecutor: string;
  touched: TaskPairTouched;
  current: TaskPair;
};

function computeAgent(input: PrefillTaskPairInput): string {
  const { workspaceDefaultAgentProfileId, agentProfiles, ownAgent } = input;
  if (agentProfiles === undefined || workspaceDefaultAgentProfileId === undefined) return "";
  const fallback = workspaceDefaultAgentProfileId
    ? agentProfiles.find((p) => p.id === workspaceDefaultAgentProfileId)
    : undefined;
  if (fallback && !fallback.cli_passthrough) return fallback.id;
  return ownAgent;
}

/**
 * Pre-fill of the Agent for created tasks pair while a coordinator is being
 * added: a touched field keeps the manager's value, an untouched one follows
 * the workspace default and the coordinator's own pair.
 */
export function prefillTaskPair(input: PrefillTaskPairInput): TaskPair {
  return {
    agent: input.touched.agent ? input.current.agent : computeAgent(input),
    executor: input.touched.executor ? input.current.executor : input.ownExecutor,
  };
}
