import { ApiError } from "@/lib/api/client";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { getAgentListResourceScope } from "@/hooks/domains/settings/agent-list-resource";
import { listAgents } from "@/lib/api";
import type { Agent } from "@/lib/types/http";
import { reorderAgentProfilesAction } from "@/app/actions/agents";

export type ProfileOrderQueueOptions = {
  save?: (
    agentId: string,
    profileIds: string[],
  ) => Promise<{ profile_ids: string[]; revision: number }>;
  read?: () => Promise<{ agents: Agent[] }>;
  onError?: (error: unknown) => void;
};

const queues = new WeakMap<StoreApi<AppState>, ProfileOrderQueue>();

export class ProfileOrderQueue {
  private readonly running = new Set<string>();
  private options: ProfileOrderQueueOptions;

  constructor(
    private readonly store: StoreApi<AppState>,
    options: ProfileOrderQueueOptions = {},
  ) {
    this.options = options;
  }

  setOptions(options: ProfileOrderQueueOptions): void {
    this.options = { ...this.options, ...options };
  }

  requestProfileOrder(agentId: string, profileIds: string[]): void {
    const state = this.store.getState();
    const current = state.agentProfiles.orderByAgent[agentId];
    const ids = [...profileIds];
    const nextInFlight = current?.inFlight ?? ids;
    const nextQueued = current?.inFlight ? ids : null;
    state.setAgentProfileOrder(agentId, ids);
    state.setAgentProfileOrderIntent(agentId, nextInFlight, nextQueued);
    if (!this.running.has(agentId)) void this.pump(agentId);
  }

  private async pump(agentId: string): Promise<void> {
    if (this.running.has(agentId)) return;
    this.running.add(agentId);
    try {
      while (await this.submitCurrent(agentId)) {}
    } finally {
      this.running.delete(agentId);
      if (this.store.getState().agentProfiles.orderByAgent[agentId]?.inFlight) {
        void this.pump(agentId);
      }
    }
  }

  private async submitCurrent(agentId: string): Promise<boolean> {
    const sync = this.store.getState().agentProfiles.orderByAgent[agentId];
    if (!sync?.inFlight) return false;
    try {
      const result = await (this.options.save ?? reorderAgentProfilesAction)(agentId, [
        ...sync.inFlight,
      ]);
      this.store.getState().acceptAgentProfileOrder(agentId, result.profile_ids, result.revision);
      const next = this.store.getState().agentProfiles.orderByAgent[agentId]?.queued ?? null;
      this.store.getState().setAgentProfileOrderIntent(agentId, next, null);
      return next !== null;
    } catch (error) {
      return this.handleRequestFailure(agentId, error);
    }
  }

  private async handleRequestFailure(agentId: string, error: unknown): Promise<boolean> {
    if (error instanceof ApiError && error.status === 409) {
      return this.handleConflict(agentId, error);
    }
    const queued = this.store.getState().agentProfiles.orderByAgent[agentId]?.queued;
    if (queued) {
      this.store.getState().setAgentProfileOrderIntent(agentId, queued, null);
      return true;
    }
    this.clearIntent(agentId);
    this.options.onError?.(error);
    return false;
  }

  private async handleConflict(agentId: string, error: ApiError): Promise<boolean> {
    try {
      await this.refetchAfterConflict();
      const queued = this.store.getState().agentProfiles.orderByAgent[agentId]?.queued;
      if (!queued) {
        this.clearIntent(agentId);
        this.options.onError?.(error);
        return false;
      }
      const replay = this.replayAgainstLiveProfiles(agentId, queued);
      if (replay.length < 2) {
        this.clearIntent(agentId);
        this.options.onError?.(error);
        return false;
      }
      this.store.getState().setAgentProfileOrder(agentId, replay);
      this.store.getState().setAgentProfileOrderIntent(agentId, replay, null);
      return true;
    } catch (refetchError) {
      this.clearIntent(agentId);
      this.options.onError?.(refetchError);
      return false;
    }
  }

  private async refetchAfterConflict(): Promise<void> {
    const epoch = this.store.getState().agentProfiles.version;
    const read = this.options.read ?? (() => listAgents({ cache: "no-store" }));
    const response = await read();
    if (!this.store.getState().applyAgentListSnapshot(response.agents, epoch)) {
      await getAgentListResourceScope(this.store).ensure();
    }
  }

  private replayAgainstLiveProfiles(agentId: string, queued: string[]): string[] {
    const live =
      this.store
        .getState()
        .settingsAgents.items.find((agent) => agent.id === agentId)
        ?.profiles.map((profile) => profile.id) ?? [];
    const queuedIds = new Set(queued);
    const liveIds = new Set<string>(live);
    return [...live.filter((id) => !queuedIds.has(id)), ...queued.filter((id) => liveIds.has(id))];
  }

  private clearIntent(agentId: string): void {
    this.store.getState().setAgentProfileOrderIntent(agentId, null, null);
  }
}

export function getProfileOrderQueue(
  store: StoreApi<AppState>,
  options: ProfileOrderQueueOptions,
): ProfileOrderQueue {
  const existing = queues.get(store);
  if (existing) {
    existing.setOptions(options);
    return existing;
  }
  const queue = new ProfileOrderQueue(store, options);
  queues.set(store, queue);
  return queue;
}
