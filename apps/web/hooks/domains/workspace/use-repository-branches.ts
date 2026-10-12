"use client";

import { useCallback, useEffect } from "react";
import type { StoreApi } from "zustand";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listBranches, listRepositoryBranches } from "@/lib/api";
import type { AppState } from "@/lib/state/store";
import type { Branch } from "@/lib/types/http";

const EMPTY_BRANCHES: Branch[] = [];
const BRANCH_LIST_RETRY_DELAYS_MS = [100, 250, 500, 1_000] as const;
const requestOwners = new WeakMap<StoreApi<AppState>, Map<string, symbol>>();

/**
 * Source of branches for a row: either a workspace repo (by id) or an
 * on-machine folder (by path). Both routes go through one backend endpoint
 * (`/workspaces/:id/branches`) and share one Zustand cache slice — id-based
 * entries are keyed by the repo id, path-based entries get a synthetic key.
 *
 * `workspaceId` is always required because the route segment needs it.
 */
export type BranchSource =
  | { kind: "id"; workspaceId: string; repositoryId: string }
  | { kind: "path"; workspaceId: string; path: string };

function cacheKeyFor(source: BranchSource | null): string {
  if (!source) return "";
  return source.kind === "id" ? source.repositoryId : `path::${source.workspaceId}::${source.path}`;
}

async function listBranchesUntilSettled(source: BranchSource): Promise<Branch[]> {
  for (const retryDelayMs of BRANCH_LIST_RETRY_DELAYS_MS) {
    try {
      const response =
        source.kind === "id"
          ? await listBranches(source.workspaceId, { repositoryId: source.repositoryId })
          : await listBranches(source.workspaceId, { path: source.path });
      return response.branches;
    } catch {
      await new Promise((resolve) => setTimeout(resolve, retryDelayMs));
    }
  }
  const response =
    source.kind === "id"
      ? await listBranches(source.workspaceId, { repositoryId: source.repositoryId })
      : await listBranches(source.workspaceId, { path: source.path });
  return response.branches;
}

async function loadBranchRequest(
  store: StoreApi<AppState>,
  key: string,
  read: () => Promise<Branch[]>,
): Promise<void> {
  let owners = requestOwners.get(store);
  if (!owners) {
    owners = new Map();
    requestOwners.set(store, owners);
  }
  const token = Symbol();
  owners.set(key, token);
  store.getState().setRepositoryBranchesLoading(key, true);
  try {
    const branches = await read();
    if (owners.get(key) === token) store.getState().setRepositoryBranches(key, branches);
  } catch {
    // Failed reads preserve the accepted list and its loaded status.
  } finally {
    // Publication can synchronously start another read through a subscriber.
    if (owners.get(key) === token) {
      owners.delete(key);
      store.getState().setRepositoryBranchesLoading(key, false);
    }
  }
}

/**
 * Loads git branches for a workspace repo or an on-machine path. One hook,
 * one cache, one backend endpoint — the source shape decides which query
 * param goes on the wire and which key the cache uses.
 */
export type UseBranchesResult = {
  branches: Branch[];
  /** True after a successful response is accepted, including an empty list. */
  isLoaded: boolean;
  isLoading: boolean;
  /**
   * Refreshes the branch list. For id-based sources the backend runs
   * `git fetch` first (force-refresh). For path-based sources we re-issue
   * the standard list call — there's no fetch endpoint for unimported
   * folders, but re-reading still surfaces newly created local branches.
   */
  refresh?: () => Promise<void>;
};

export function useBranches(source: BranchSource | null, enabled = true): UseBranchesResult {
  const store = useAppStoreApi();
  const key = cacheKeyFor(source);
  const branches = useAppStore((state) =>
    key ? (state.repositoryBranches.itemsByRepositoryId[key] ?? EMPTY_BRANCHES) : EMPTY_BRANCHES,
  );
  const isLoaded = useAppStore((state) =>
    key ? (state.repositoryBranches.loadedByRepositoryId[key] ?? false) : false,
  );
  const isLoading = useAppStore((state) =>
    key ? (state.repositoryBranches.loadingByRepositoryId[key] ?? false) : false,
  );
  useEffect(() => {
    if (!enabled || !source) return;
    if (
      store.getState().repositoryBranches.loadedByRepositoryId[key] ||
      requestOwners.get(store)?.has(key)
    )
      return;
    void loadBranchRequest(store, key, () => listBranchesUntilSettled(source));
    // eslint-disable-next-line react-hooks/exhaustive-deps -- key encodes source identity; listing every field re-fires on every render
  }, [enabled, isLoaded, key, store]);

  const refresh = useCallback(async () => {
    if (!source) return;
    await loadBranchRequest(store, key, async () => {
      const response =
        source.kind === "id"
          ? await listRepositoryBranches(source.repositoryId, { refresh: true })
          : await listBranches(source.workspaceId, { path: source.path });
      return response.branches;
    });
  }, [source, key, store]);

  return {
    branches,
    isLoaded,
    isLoading,
    refresh: source ? refresh : undefined,
  };
}
