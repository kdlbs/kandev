import type { Agent } from "@/lib/types/http";
import type { AgentProfileOption, ProfileOrderSync } from "@/lib/state/slices/settings/types";

export type ProfileOrderState = Record<string, ProfileOrderSync>;

export function acceptAgentOrdersFromSnapshot(
  state: ProfileOrderState,
  agents: Agent[],
): ProfileOrderState {
  let next = state;
  for (const agent of agents) {
    if (agent.profile_order_revision === undefined && !state[agent.id]) continue;
    next = acceptServerOrder(
      next,
      agent.id,
      agent.profiles.map((profile) => profile.id),
      agent.profile_order_revision ?? state[agent.id]?.revision ?? 0,
    );
    const current = next[agent.id];
    const incoming = agent.profiles.map((profile) => profile.id);
    const liveIds = new Set<string>(incoming);
    const knownIds = new Set(current.order ?? []);
    // Membership has its own freshness fence; equal order revisions can add or remove rows.
    const order = [
      ...incoming.filter((id) => !knownIds.has(id)),
      ...(current.order ?? []).filter((id) => liveIds.has(id)),
    ];
    if (
      order.length !== current.order?.length ||
      order.some((id, index) => id !== current.order?.[index])
    ) {
      next = { ...next, [agent.id]: { ...current, order } };
    }
  }
  return next;
}

export function reorderIds(ids: string[], activeId: string, overId: string): string[] {
  const from = ids.indexOf(activeId);
  const to = ids.indexOf(overId);
  if (from < 0 || to < 0 || from === to) return ids;
  const next = ids.slice();
  next.splice(from, 1);
  next.splice(to, 0, activeId);
  return next;
}

export function acceptServerOrder(
  state: ProfileOrderState,
  agentId: string,
  ids: string[],
  revision: number,
): ProfileOrderState {
  const current = state[agentId];
  if (current?.order !== null && current?.order !== undefined && revision <= current.revision)
    return state;
  return {
    ...state,
    [agentId]: {
      revision,
      order: [...(current?.order ?? []).filter((id) => !ids.includes(id)), ...ids],
      inFlight: current?.inFlight ?? null,
      queued: current?.queued ?? null,
    },
  };
}

function reorderProfileGroup<T extends { id: string }>(items: T[], order: string[]): T[] {
  const rank = new Map(order.map((id, index) => [id, index]));
  return items
    .map((item, index) => ({ item, index }))
    .sort((left, right) => {
      const leftRank = rank.get(left.item.id);
      const rightRank = rank.get(right.item.id);
      if (leftRank === undefined && rightRank === undefined) return left.index - right.index;
      if (leftRank === undefined) return -1;
      if (rightRank === undefined) return 1;
      return leftRank - rightRank || left.index - right.index;
    })
    .map(({ item }) => item);
}

export function reconcileAgentOrders(agents: Agent[], sync: ProfileOrderState): Agent[] {
  let changed = false;
  const next = agents.map((agent) => {
    const state = sync[agent.id];
    if (!state) return agent;
    const overlay = state.queued ?? state.inFlight;
    const order = overlay ?? state.order;
    if (!order) return agent;
    const profiles = reorderProfileGroup(agent.profiles, order);
    if (profiles.every((profile, index) => profile === agent.profiles[index])) return agent;
    changed = true;
    return { ...agent, profiles };
  });
  return changed ? next : agents;
}

export function insertFirstInAgentGroup(
  options: AgentProfileOption[],
  agentId: string,
  option: AgentProfileOption,
): AgentProfileOption[] {
  const withoutDuplicate = options.filter((existing) => existing.id !== option.id);
  const firstIndex = withoutDuplicate.findIndex((existing) => existing.agent_id === agentId);
  withoutDuplicate.splice(firstIndex < 0 ? withoutDuplicate.length : firstIndex, 0, option);
  return withoutDuplicate;
}
