import type { StoreApi } from "zustand";
import { listAgents } from "@/lib/api";
import { getBackendConfig } from "@/lib/config";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { AppState } from "@/lib/state/store";

const AGENT_LIST_RETRY_DELAYS_MS = [100, 250, 500, 1_000] as const;

export type AgentListResponse = Awaited<ReturnType<typeof listAgents>>;

export type AgentListSnapshot = {
  response: AgentListResponse | null;
  profileVersion: number;
  loading: boolean;
  loaded: boolean;
  error: unknown;
};

export class AgentListScopeRetiredError extends Error {
  constructor() {
    super("Agent-list scope is no longer active");
    this.name = "AgentListScopeRetiredError";
  }
}

const EMPTY_SNAPSHOT: AgentListSnapshot = {
  response: null,
  profileVersion: -1,
  loading: false,
  loaded: false,
  error: null,
};

type ScopeOwner = { identity: string; scope: AgentListResourceScope };
const scopes = new WeakMap<StoreApi<AppState>, ScopeOwner>();

export function agentListAuthScopeIdentity(state: AppState): string {
  return JSON.stringify([
    getBackendConfig().apiBaseUrl,
    state.auth.mode,
    state.auth.authenticated,
    state.auth.user?.id,
  ]);
}

function hasAgentProfiles(response: AgentListResponse): boolean {
  return response.agents.some((agent) => (agent.profiles?.length ?? 0) > 0);
}

function waitForAgentListRetry(delayMs: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, delayMs));
}

/**
 * Agent discovery can briefly return an empty list before startup discovery and
 * profile persistence settle. Keep the existing bounded retry window shared
 * by every consumer of the list.
 */
export async function listAgentsUntilSettled(): Promise<AgentListResponse> {
  for (const retryDelayMs of AGENT_LIST_RETRY_DELAYS_MS) {
    try {
      const response = await listAgents({ cache: "no-store" });
      if (hasAgentProfiles(response)) return response;
    } catch {}

    await waitForAgentListRetry(retryDelayMs);
  }

  return listAgents({ cache: "no-store" });
}

export class AgentListResourceScope {
  private snapshot: AgentListSnapshot = EMPTY_SNAPSHOT;
  private listeners = new Set<() => void>();
  private flight: Promise<AgentListResponse> | null = null;
  private queuedRefresh = false;
  private requestGeneration = 0;
  private active = true;
  private availableAgentsReconciled = false;

  constructor(private readonly store: StoreApi<AppState>) {}

  get isActive(): boolean {
    return this.active;
  }

  getSnapshot = (): AgentListSnapshot => (this.active ? this.snapshot : EMPTY_SNAPSHOT);

  subscribe = (listener: () => void): (() => void) => {
    if (!this.active) return () => {};
    this.listeners.add(listener);
    if (!this.snapshot.loaded || this.snapshot.profileVersion !== this.currentProfileVersion()) {
      void this.ensure().catch(() => undefined);
    }
    return () => this.listeners.delete(listener);
  };

  ensure(): Promise<AgentListResponse> {
    if (!this.active) return Promise.reject(new AgentListScopeRetiredError());
    if (this.flight) return this.flight;
    if (
      this.snapshot.loaded &&
      this.snapshot.profileVersion === this.currentProfileVersion() &&
      this.snapshot.response
    ) {
      return Promise.resolve(this.snapshot.response);
    }
    return this.startRead();
  }

  refreshAfterAvailableAgents(): Promise<AgentListResponse> {
    if (this.availableAgentsReconciled) return this.ensure();
    this.availableAgentsReconciled = true;
    if (this.flight) {
      this.queuedRefresh = true;
      return this.flight;
    }
    return this.startRead();
  }

  retire(): void {
    this.active = false;
    this.requestGeneration += 1;
    this.snapshot = EMPTY_SNAPSHOT;
    this.listeners.forEach((listener) => listener());
    this.listeners.clear();
  }

  profileVersionChanged(version: number): void {
    if (!this.active || this.snapshot.profileVersion === version) return;
    if (this.flight) {
      this.queuedRefresh = true;
      return;
    }
    if (this.listeners.size > 0) void this.ensure().catch(() => undefined);
  }

  private currentProfileVersion(): number {
    return this.store.getState().agentProfiles.version;
  }

  private publish(snapshot: AgentListSnapshot): void {
    this.snapshot = snapshot;
    this.listeners.forEach((listener) => listener());
  }

  private apply(response: AgentListResponse): void {
    const state = this.store.getState();
    state.setSettingsAgents(response.agents);
    state.setAgentProfiles(
      response.agents.flatMap((agent) =>
        agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
      ),
    );
  }

  private startRead(): Promise<AgentListResponse> {
    const profileVersion = this.currentProfileVersion();
    const generation = ++this.requestGeneration;
    this.publish({ ...this.snapshot, profileVersion, loading: true, error: null });

    const flight: Promise<AgentListResponse> = listAgentsUntilSettled()
      .then((response) => {
        if (!this.active || this.requestGeneration !== generation) {
          throw new AgentListScopeRetiredError();
        }
        if (this.queuedRefresh || this.currentProfileVersion() !== profileVersion) {
          this.queuedRefresh = false;
          if (this.flight === flight) this.flight = null;
          return this.startRead();
        }
        this.apply(response);
        this.publish({ response, profileVersion, loading: false, loaded: true, error: null });
        return response;
      })
      .catch((error: unknown) => {
        if (!this.active || this.requestGeneration !== generation) throw error;
        if (this.queuedRefresh || this.currentProfileVersion() !== profileVersion) {
          this.queuedRefresh = false;
          if (this.flight === flight) this.flight = null;
          return this.startRead();
        }
        this.publish({ ...this.snapshot, profileVersion, loading: false, loaded: false, error });
        throw error;
      })
      .finally(() => {
        if (this.flight === flight) this.flight = null;
      });

    this.flight = flight;
    return flight;
  }
}

export function getAgentListResourceScope(store: StoreApi<AppState>): AgentListResourceScope {
  let owner = scopes.get(store);
  const identity = agentListAuthScopeIdentity(store.getState());
  if (!owner) {
    owner = { identity, scope: new AgentListResourceScope(store) };
    scopes.set(store, owner);
    const scopeOwner = owner;
    let profileVersion = store.getState().agentProfiles.version;
    store.subscribe((state) => {
      const nextIdentity = agentListAuthScopeIdentity(state);
      if (scopeOwner.identity !== nextIdentity) {
        scopeOwner.scope.retire();
        scopeOwner.identity = nextIdentity;
        scopeOwner.scope = new AgentListResourceScope(store);
        profileVersion = state.agentProfiles.version;
        return;
      }
      if (profileVersion !== state.agentProfiles.version) {
        profileVersion = state.agentProfiles.version;
        scopeOwner.scope.profileVersionChanged(profileVersion);
      }
    });
  } else if (owner.identity !== identity) {
    owner.scope.retire();
    owner.identity = identity;
    owner.scope = new AgentListResourceScope(store);
  }
  return owner.scope;
}
