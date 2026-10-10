import type { StoreApi } from "zustand/vanilla";
import type { AppState } from "./app-state-types";

type AgentOwnerStore = Pick<StoreApi<AppState>, "getState" | "subscribe">;

const removedOwnersByStore = new WeakMap<AgentOwnerStore, Set<string>>();

// Agent identities are not reused. Observed removals remain visible to pending
// publishers for the store's lifetime, independently of editor subscriptions.
export function observeSettingsAgentRemovals(store: AgentOwnerStore) {
  if (removedOwnersByStore.has(store)) return;
  const removedOwners = new Set<string>();
  removedOwnersByStore.set(store, removedOwners);
  store.subscribe((state, previous) => {
    if (state.settingsAgents.items === previous.settingsAgents.items) return;
    const currentIds = new Set(state.settingsAgents.items.map((agent) => agent.id));
    for (const agent of previous.settingsAgents.items) {
      if (!currentIds.has(agent.id)) removedOwners.add(agent.id);
    }
  });
}

export function wasSettingsAgentRemoved(store: AgentOwnerStore, agentId: string) {
  return removedOwnersByStore.get(store)?.has(agentId) === true;
}
