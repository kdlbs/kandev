"use client";

import { useCallback, useEffect, useSyncExternalStore } from "react";
import {
  getRepositoryDiscoveryAction,
  refreshRepositoryDiscoveryAction,
} from "@/app/actions/workspaces";
import type { RepositoryDiscoveryRefreshTrigger } from "@/app/actions/repository-discovery";
import type { RepositoryDiscoveryResponse } from "@/lib/types/http";

export const REPOSITORY_DISCOVERY_REFRESH_AGE_MS = 30 * 60 * 1000;

export type RepositoryDiscoveryState = {
  response: RepositoryDiscoveryResponse | null;
  isLoading: boolean;
  isRefreshing: boolean;
  error: Error | null;
};

export type RepositoryDiscoveryClient = {
  getSnapshot: (workspaceId: string) => Promise<RepositoryDiscoveryResponse>;
  refresh: (
    workspaceId: string,
    trigger?: RepositoryDiscoveryRefreshTrigger,
  ) => Promise<RepositoryDiscoveryResponse>;
};

type VisibilityDocument = {
  visibilityState: DocumentVisibilityState;
  addEventListener: Document["addEventListener"];
  removeEventListener: Document["removeEventListener"];
};

type CoordinatorEntry = {
  state: RepositoryDiscoveryState;
  listeners: Set<() => void>;
  leases: number;
  snapshotPromise: Promise<void> | null;
  refreshPromise: Promise<void> | null;
};

const EMPTY_STATE: RepositoryDiscoveryState = {
  response: null,
  isLoading: false,
  isRefreshing: false,
  error: null,
};

const EMPTY_REPOSITORIES: RepositoryDiscoveryResponse["repositories"] = [];
const EMPTY_ROOTS: RepositoryDiscoveryResponse["roots"] = [];
const EMPTY_ROOT_STATES: NonNullable<RepositoryDiscoveryResponse["root_states"]> = [];
const EMPTY_FAILED_ROOTS: NonNullable<RepositoryDiscoveryResponse["failed_roots"]> = [];

const DEFAULT_CLIENT: RepositoryDiscoveryClient = {
  getSnapshot: (...args) => getRepositoryDiscoveryAction(...args),
  refresh: (workspaceId, trigger) =>
    refreshRepositoryDiscoveryAction(workspaceId, undefined, trigger),
};

function browserDocument(): VisibilityDocument | undefined {
  return typeof document === "undefined" ? undefined : document;
}

function asError(value: unknown): Error {
  return value instanceof Error ? value : new Error(String(value));
}

function normalizedResponse(response: RepositoryDiscoveryResponse): RepositoryDiscoveryResponse {
  return {
    ...response,
    roots: response.roots ?? [],
    repositories: response.repositories ?? [],
    total: response.total ?? response.repositories?.length ?? 0,
    root_states: response.root_states ?? [],
    refreshing: response.refreshing === true,
    cached: response.cached === true,
    home_confirmation_required: response.home_confirmation_required === true,
    failed_roots: response.failed_roots ?? [],
  };
}

function hasEffectiveRoots(response: RepositoryDiscoveryResponse): boolean {
  return (response.roots?.length ?? 0) > 0;
}

function isStale(
  response: RepositoryDiscoveryResponse | null,
  now: () => number,
  age: number,
): boolean {
  if (!response || !hasEffectiveRoots(response)) return false;
  // A failed root is an explicit recovery state. Do not retry it on every
  // visible activation; Reconnect, Remove, or manual Refresh is the user's
  // decision to touch that path again.
  if (response.root_states?.some((root) => root.state === "reconnect_required")) {
    return false;
  }
  if ((response.failed_roots?.length ?? 0) > 0) return false;
  if (!response.scan_time) return true;
  const timestamp = Date.parse(response.scan_time);
  return !Number.isFinite(timestamp) || now() - timestamp >= age;
}

/**
 * Owns repository-discovery leases for one browser tab. It has no timer:
 * visibility changes and explicit user actions are the only refresh triggers.
 */
export class RepositoryDiscoveryCoordinator {
  private readonly entries = new Map<string, CoordinatorEntry>();
  private readonly client: RepositoryDiscoveryClient;
  private readonly visibilityDocument?: VisibilityDocument;
  private readonly now: () => number;
  private readonly refreshAge: number;
  private listeningForVisibility = false;

  constructor(
    client: RepositoryDiscoveryClient = DEFAULT_CLIENT,
    options: {
      document?: VisibilityDocument;
      now?: () => number;
      refreshAge?: number;
    } = {},
  ) {
    this.client = client;
    this.visibilityDocument = options.document ?? browserDocument();
    this.now = options.now ?? Date.now;
    this.refreshAge = options.refreshAge ?? REPOSITORY_DISCOVERY_REFRESH_AGE_MS;
  }

  getSnapshot(workspaceId: string | null): RepositoryDiscoveryState {
    if (!workspaceId) return EMPTY_STATE;
    return this.entry(workspaceId).state;
  }

  subscribe(workspaceId: string | null, listener: () => void): () => void {
    if (!workspaceId) return () => undefined;
    const entry = this.entry(workspaceId);
    entry.listeners.add(listener);
    return () => entry.listeners.delete(listener);
  }

  acquire(workspaceId: string): () => void {
    const entry = this.entry(workspaceId);
    entry.leases += 1;
    this.updateVisibilityListener();
    this.loadAndRefreshIfNeeded(workspaceId);
    let released = false;
    return () => {
      if (released) return;
      released = true;
      entry.leases = Math.max(0, entry.leases - 1);
      this.updateVisibilityListener();
    };
  }

  async refresh(
    workspaceId: string,
    trigger: RepositoryDiscoveryRefreshTrigger = "manual_refresh",
  ): Promise<RepositoryDiscoveryResponse | null> {
    const entry = this.entry(workspaceId);
    await this.startRefresh(workspaceId, entry, trigger);
    return entry.state.response;
  }

  async load(workspaceId: string): Promise<RepositoryDiscoveryResponse | null> {
    const entry = this.entry(workspaceId);
    await this.startSnapshot(workspaceId, entry);
    return entry.state.response;
  }

  dispose(): void {
    if (this.listeningForVisibility && this.visibilityDocument) {
      this.visibilityDocument.removeEventListener("visibilitychange", this.handleVisibilityChange);
    }
    this.listeningForVisibility = false;
    this.entries.clear();
  }

  private readonly handleVisibilityChange = () => {
    if (!this.isVisible()) return;
    for (const [workspaceId, entry] of this.entries) {
      if (entry.leases > 0) this.loadAndRefreshIfNeeded(workspaceId);
    }
  };

  private entry(workspaceId: string): CoordinatorEntry {
    const existing = this.entries.get(workspaceId);
    if (existing) return existing;
    const created: CoordinatorEntry = {
      state: { ...EMPTY_STATE },
      listeners: new Set(),
      leases: 0,
      snapshotPromise: null,
      refreshPromise: null,
    };
    this.entries.set(workspaceId, created);
    return created;
  }

  private isVisible(): boolean {
    return !this.visibilityDocument || this.visibilityDocument.visibilityState === "visible";
  }

  private updateVisibilityListener(): void {
    if (!this.visibilityDocument) return;
    const shouldListen = [...this.entries.values()].some((entry) => entry.leases > 0);
    if (shouldListen && !this.listeningForVisibility) {
      this.visibilityDocument.addEventListener("visibilitychange", this.handleVisibilityChange);
      this.listeningForVisibility = true;
    } else if (!shouldListen && this.listeningForVisibility) {
      this.visibilityDocument.removeEventListener("visibilitychange", this.handleVisibilityChange);
      this.listeningForVisibility = false;
    }
  }

  private notify(entry: CoordinatorEntry): void {
    for (const listener of entry.listeners) listener();
  }

  private setState(entry: CoordinatorEntry, patch: Partial<RepositoryDiscoveryState>): void {
    entry.state = { ...entry.state, ...patch };
    this.notify(entry);
  }

  private loadAndRefreshIfNeeded(workspaceId: string): void {
    const entry = this.entry(workspaceId);
    if (!entry.state.response) {
      void this.startSnapshot(workspaceId, entry);
      return;
    }
    if (
      entry.leases > 0 &&
      this.isVisible() &&
      (entry.state.response?.refreshing === true ||
        isStale(entry.state.response, this.now, this.refreshAge))
    ) {
      void this.startRefresh(workspaceId, entry, "stale_refresh");
    }
  }

  private async startSnapshot(workspaceId: string, entry: CoordinatorEntry): Promise<void> {
    if (entry.snapshotPromise) return entry.snapshotPromise;
    this.setState(entry, { isLoading: true, error: null });
    entry.snapshotPromise = this.client
      .getSnapshot(workspaceId)
      .then((response) => {
        this.setState(entry, {
          response: normalizedResponse(response),
          isLoading: false,
          error: null,
        });
      })
      .catch((error: unknown) => {
        this.setState(entry, { isLoading: false, error: asError(error) });
      })
      .finally(() => {
        entry.snapshotPromise = null;
        if (
          entry.leases > 0 &&
          this.isVisible() &&
          (entry.state.response?.refreshing === true ||
            isStale(entry.state.response, this.now, this.refreshAge))
        ) {
          void this.startRefresh(workspaceId, entry, "stale_refresh");
        }
      });
    return entry.snapshotPromise;
  }

  private async startRefresh(
    workspaceId: string,
    entry: CoordinatorEntry,
    trigger: RepositoryDiscoveryRefreshTrigger,
  ): Promise<void> {
    if (entry.refreshPromise) return entry.refreshPromise;
    this.setState(entry, {
      isRefreshing: true,
      error: null,
    });
    entry.refreshPromise = this.client
      .refresh(workspaceId, trigger)
      .then((response) => {
        const normalized = normalizedResponse(response);
        this.setState(entry, {
          response: normalized,
          isRefreshing: normalized.refreshing === true,
          error: null,
        });
      })
      .catch((error: unknown) => {
        this.setState(entry, { isRefreshing: false, error: asError(error) });
      })
      .finally(() => {
        entry.refreshPromise = null;
      });
    return entry.refreshPromise;
  }
}

export const repositoryDiscoveryCoordinator = new RepositoryDiscoveryCoordinator();

function discoveryView(
  state: RepositoryDiscoveryState,
  refresh: () => Promise<RepositoryDiscoveryResponse | null>,
  load: () => Promise<RepositoryDiscoveryResponse | null>,
) {
  const response = state.response;
  return {
    repositories: response?.repositories ?? EMPTY_REPOSITORIES,
    roots: response?.roots ?? EMPTY_ROOTS,
    rootStates: response?.root_states ?? EMPTY_ROOT_STATES,
    scanTime: response?.scan_time,
    failedRoots: response?.failed_roots ?? EMPTY_FAILED_ROOTS,
    homeConfirmationRequired: response?.home_confirmation_required === true,
    isLoading: state.isLoading,
    isRefreshing: state.isRefreshing || response?.refreshing === true,
    error: state.error,
    cached: response?.cached === true,
    hasSnapshot: response !== null,
    desktopRuntime: response?.desktop_runtime === true,
    refresh,
    load,
  };
}

export function useRepositoryDiscovery(workspaceId: string | null, enabled = true) {
  const activeWorkspaceId = enabled ? workspaceId : null;
  const subscribe = useCallback(
    (listener: () => void) =>
      activeWorkspaceId
        ? repositoryDiscoveryCoordinator.subscribe(activeWorkspaceId, listener)
        : () => undefined,
    [activeWorkspaceId],
  );
  const getSnapshot = useCallback(
    () => repositoryDiscoveryCoordinator.getSnapshot(activeWorkspaceId),
    [activeWorkspaceId],
  );
  const state = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);

  useEffect(() => {
    if (!activeWorkspaceId) return;
    return repositoryDiscoveryCoordinator.acquire(activeWorkspaceId);
  }, [activeWorkspaceId]);

  const refresh = useCallback(async () => {
    if (!activeWorkspaceId) return null;
    return repositoryDiscoveryCoordinator.refresh(activeWorkspaceId, "manual_refresh");
  }, [activeWorkspaceId]);
  const load = useCallback(async () => {
    if (!activeWorkspaceId) return null;
    return repositoryDiscoveryCoordinator.load(activeWorkspaceId);
  }, [activeWorkspaceId]);

  return discoveryView(state, refresh, load);
}
