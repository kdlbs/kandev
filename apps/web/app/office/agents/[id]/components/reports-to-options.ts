import type { AgentProfile } from "@/lib/state/slices/office/types";

/**
 * Every agent transitively managed by `rootId`, walking the `reportsTo` edges
 * forward. Re-parenting `rootId` under one of its own descendants would form
 * a cycle: `buildForest` (org-tree-layout.ts) treats a cyclic agent's parent
 * as unresolved and drops the whole cycle from the org chart with no error.
 */
export function collectDescendantIds(agents: AgentProfile[], rootId: string): Set<string> {
  const childrenByParent = new Map<string, string[]>();
  for (const agent of agents) {
    if (!agent.reportsTo) continue;
    const siblings = childrenByParent.get(agent.reportsTo) ?? [];
    siblings.push(agent.id);
    childrenByParent.set(agent.reportsTo, siblings);
  }

  const descendants = new Set<string>();
  const queue = [...(childrenByParent.get(rootId) ?? [])];
  while (queue.length > 0) {
    const id = queue.pop()!;
    if (descendants.has(id)) continue;
    descendants.add(id);
    queue.push(...(childrenByParent.get(id) ?? []));
  }
  return descendants;
}

/** Agents eligible as a manager for `agentId`: everyone except itself and its own descendants. */
export function reportsToOptions(
  agents: AgentProfile[],
  agentId: string,
): Array<{ id: string; name: string }> {
  const excluded = collectDescendantIds(agents, agentId);
  excluded.add(agentId);
  return agents.filter((a) => !excluded.has(a.id)).map((a) => ({ id: a.id, name: a.name }));
}
