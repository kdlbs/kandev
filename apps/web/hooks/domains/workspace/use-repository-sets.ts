import { useCallback, useEffect } from "react";
import type { StoreApi } from "zustand";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { RepositorySet } from "@/lib/types/http";
import { listRepositorySets } from "@/lib/api";

const EMPTY_REPOSITORY_SETS: RepositorySet[] = [];
const READ_TIMEOUT_MS = 30_000;
const requestsByStore = new WeakMap<StoreApi<AppState>, Map<string, Promise<void>>>();

async function readRepositorySets(
  store: StoreApi<AppState>,
  workspaceId: string,
  signal: AbortSignal,
): Promise<void> {
  let retry: boolean;
  do {
    const requestRevision = store.getState().repositorySets.revisionByWorkspaceId[workspaceId] ?? 0;
    const response = await listRepositorySets(workspaceId, { cache: "no-store", init: { signal } });
    const current = store.getState().repositorySets;
    retry =
      requestRevision !== (current.revisionByWorkspaceId[workspaceId] ?? 0) &&
      !current.loadedByWorkspaceId[workspaceId];
    // An event is only a partial list. Recover a complete snapshot before
    // settling an unloaded workspace whose response was overtaken.
    if (!retry)
      store.getState().setRepositorySets(workspaceId, response.repository_sets, requestRevision);
  } while (retry);
}

function loadRepositorySets(store: StoreApi<AppState>, workspaceId: string): Promise<void> {
  let requests = requestsByStore.get(store);
  if (!requests) {
    requests = new Map();
    requestsByStore.set(store, requests);
  }
  const existing = requests.get(workspaceId);
  if (existing) return existing;

  const controller = new AbortController();
  const deadline = setTimeout(() => controller.abort(), READ_TIMEOUT_MS);
  // Reserve ownership before publishing loading or starting the transport.
  const request = Promise.resolve()
    .then(() => readRepositorySets(store, workspaceId, controller.signal))
    .catch(() => {
      // Keep cached events and loaded status; a later mount or refresh can retry.
    })
    .finally(() => {
      clearTimeout(deadline);
      requests.delete(workspaceId);
      store.getState().setRepositorySetsLoading(workspaceId, false);
    });
  requests.set(workspaceId, request);
  store.getState().setRepositorySetsLoading(workspaceId, true);
  return request;
}

/**
 * Loads a workspace's repository sets from the store, fetching once when not yet
 * loaded.
 *
 * The boot payload hydrates this slice for every route that can open the task
 * dialog, so the common case does no fetch at all. A workspace with no sets is
 * still marked loaded at boot, which is why the fetch is gated on `isLoaded`
 * rather than on the list being non-empty.
 */
export function useRepositorySets(workspaceId: string | null, enabled = true) {
  const store = useAppStoreApi();
  const sets = useAppStore((state) =>
    workspaceId
      ? (state.repositorySets.itemsByWorkspaceId[workspaceId] ?? EMPTY_REPOSITORY_SETS)
      : EMPTY_REPOSITORY_SETS,
  );
  const isLoading = useAppStore((state) =>
    workspaceId ? (state.repositorySets.loadingByWorkspaceId[workspaceId] ?? false) : false,
  );
  const isLoaded = useAppStore((state) =>
    workspaceId ? (state.repositorySets.loadedByWorkspaceId[workspaceId] ?? false) : false,
  );
  const refresh = useCallback(async () => {
    if (!enabled || !workspaceId) return;
    await loadRepositorySets(store, workspaceId);
  }, [enabled, store, workspaceId]);

  useEffect(() => {
    if (!enabled || !workspaceId || isLoaded) return;
    if (store.getState().repositorySets.loadedByWorkspaceId[workspaceId]) return;
    void loadRepositorySets(store, workspaceId);
  }, [enabled, isLoaded, workspaceId, store]);

  return { sets, isLoading, refresh };
}
