"use client";

import { useEffect, useMemo, useSyncExternalStore } from "react";
import type { StoreApi } from "zustand";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  listAgentProfileUsage,
  type AgentProfileUsage,
} from "@/lib/api/domains/agent-profile-usage-api";
import type { AppState } from "@/lib/state/store";
import { getBackendConfig } from "@/lib/config";

type UsageSnapshot = {
  byProfile: ReadonlyMap<string, AgentProfileUsage>;
  loaded: boolean;
  loading: boolean;
  failed: boolean;
};

const EMPTY: UsageSnapshot = { byProfile: new Map(), loaded: false, loading: false, failed: false };

// Each application store owns its request and cache. Account observations from
// another login/backend/profile revision cannot publish into this scope.
class UsageScope {
  private snapshot = EMPTY;
  private listeners = new Set<() => void>();
  private flight: Promise<void> | null = null;
  private fetchedAt = 0;

  constructor(private readonly current: () => boolean) {}

  getSnapshot = () => (this.current() ? this.snapshot : EMPTY);

  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  private publish(snapshot: UsageSnapshot) {
    if (!this.current()) return;
    this.snapshot = snapshot;
    this.listeners.forEach((listener) => listener());
  }

  read = (refresh = false): Promise<void> => {
    if (!this.current()) return Promise.resolve();
    if (this.flight) return this.flight;
    if (this.snapshot.loaded && !refresh && Date.now() - this.fetchedAt < 5 * 60_000)
      return Promise.resolve();
    this.publish({ ...this.snapshot, loading: true, failed: false });
    this.flight = listAgentProfileUsage()
      .then(({ profiles }) => {
        this.publish({
          byProfile: new Map(profiles.map((row) => [row.profile_id, row])),
          loaded: true,
          loading: false,
          failed: false,
        });
      })
      .catch(() => this.publish({ ...EMPTY, loaded: true, failed: true }))
      .finally(() => {
        this.fetchedAt = Date.now();
        this.flight = null;
      });
    return this.flight;
  };
}

type ScopeOwner = { key: string; scope: UsageScope };
const owners = new WeakMap<StoreApi<AppState>, ScopeOwner>();

function scopeKey(state: AppState): string {
  return JSON.stringify([
    getBackendConfig().apiBaseUrl,
    state.auth.mode,
    state.auth.authenticated,
    state.auth.user?.id,
    state.agentProfiles.version,
    state.features.providerUsageLimits,
  ]);
}

function scopeFor(store: StoreApi<AppState>, key: string): UsageScope {
  const owner = owners.get(store);
  if (owner?.key === key) return owner.scope;
  const scope: UsageScope = new UsageScope(
    (): boolean => owners.get(store)?.scope === scope && scopeKey(store.getState()) === key,
  );
  owners.set(store, { key, scope });
  return scope;
}

// The enabled-only component mounts this reader. There is no refresh interval;
// all rows share the initial read and explicit refresh request.
export function useAgentProfileUsage(profileId: string) {
  const store = useAppStoreApi();
  const key = useAppStore(scopeKey);
  const scope = useMemo(() => scopeFor(store, key), [store, key]);
  const snapshot = useSyncExternalStore(scope.subscribe, scope.getSnapshot, () => EMPTY);
  useEffect(() => {
    void scope.read();
  }, [scope]);
  return { ...snapshot, usage: snapshot.byProfile.get(profileId), refresh: () => scope.read(true) };
}
