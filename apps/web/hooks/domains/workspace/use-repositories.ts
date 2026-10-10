import { useCallback, useEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { Repository } from "@/lib/types/http";
import { readJourneyRepositories } from "@/hooks/journey-metadata-resources";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";

const EMPTY_REPOSITORIES: Repository[] = [];
const repositoryRequestsByStore = new WeakMap<StoreApi<AppState>, Map<string, number>>();

function beginRepositoryRequest(
  store: StoreApi<AppState>,
  workspaceId: string,
  setRepositoriesLoading: (workspaceId: string, loading: boolean) => void,
): () => void {
  const activeRepositoryRequests =
    repositoryRequestsByStore.get(store) ?? new Map<string, number>();
  repositoryRequestsByStore.set(store, activeRepositoryRequests);
  activeRepositoryRequests.set(workspaceId, (activeRepositoryRequests.get(workspaceId) ?? 0) + 1);
  setRepositoriesLoading(workspaceId, true);
  let released = false;
  return () => {
    if (released) return;
    released = true;
    const remaining = (activeRepositoryRequests.get(workspaceId) ?? 1) - 1;
    if (remaining > 0) {
      activeRepositoryRequests.set(workspaceId, remaining);
      setRepositoriesLoading(workspaceId, true);
    } else {
      activeRepositoryRequests.delete(workspaceId);
      setRepositoriesLoading(workspaceId, false);
    }
  };
}

/**
 * Loads a workspace's repositories from the store, fetching once when not yet
 * loaded. Pass `forceRefresh` to instead pull a fresh list once per workspace on
 * mount (e.g. a picker that must reflect repos created since the slice was first
 * loaded) — the lazy path is disabled in that mode so there's no double fetch,
 * and the per-workspace guard is only marked on success so a failed fetch can
 * retry on the next mount.
 */
export function useRepositories(workspaceId: string | null, enabled = true, forceRefresh = false) {
  const store = useAppStoreApi();
  const repositories = useAppStore((state) =>
    workspaceId
      ? (state.repositories.itemsByWorkspaceId[workspaceId] ?? EMPTY_REPOSITORIES)
      : EMPTY_REPOSITORIES,
  );
  const isLoading = useAppStore((state) =>
    workspaceId ? (state.repositories.loadingByWorkspaceId[workspaceId] ?? false) : false,
  );
  const isLoaded = useAppStore((state) =>
    workspaceId ? (state.repositories.loadedByWorkspaceId[workspaceId] ?? false) : false,
  );
  const setRepositories = useAppStore((state) => state.setRepositories);
  const setRepositoriesLoading = useAppStore((state) => state.setRepositoriesLoading);
  const forcedRef = useRef<string | null>(null);

  const refresh = useCallback(async () => {
    if (!enabled || !workspaceId) return;
    const releaseRequest = beginRepositoryRequest(store, workspaceId, setRepositoriesLoading);
    try {
      const response = await readJourneyRepositories(store, workspaceId);
      setRepositories(workspaceId, response.repositories);
    } catch {
      // Keep the existing cached repositories when a manual refresh fails.
    } finally {
      releaseRequest();
    }
  }, [enabled, setRepositories, setRepositoriesLoading, store, workspaceId]);

  // Force-refresh: pull a fresh list once per workspace, bypassing the
  // isLoaded cache. forcedRef is set only on success so a failed fetch retries.
  useEffect(() => {
    if (!enabled || !workspaceId || !forceRefresh) return;
    if (forcedRef.current === workspaceId) return;
    let cancelled = false;
    const controller = new AbortController();
    const releaseRequest = beginRepositoryRequest(store, workspaceId, setRepositoriesLoading);
    readJourneyRepositories(store, workspaceId, { signal: controller.signal })
      .then((response) => {
        if (cancelled) return;
        forcedRef.current = workspaceId;
        setRepositories(workspaceId, response.repositories);
      })
      .catch(() => {
        // Leave forcedRef unset so the next mount retries; keep cached repos.
      })
      .finally(releaseRequest);
    return () => {
      cancelled = true;
      controller.abort();
      releaseRequest();
    };
  }, [enabled, forceRefresh, workspaceId, setRepositories, setRepositoriesLoading, store]);

  useEffect(() => {
    if (!enabled || !workspaceId || forceRefresh) return;
    if (isLoaded) return;
    let cancelled = false;
    const controller = new AbortController();
    const releaseRequest = beginRepositoryRequest(store, workspaceId, setRepositoriesLoading);
    readJourneyRepositories(store, workspaceId, { signal: controller.signal })
      .then((response) => {
        if (cancelled) return;
        setRepositories(workspaceId, response.repositories);
      })
      .catch(() => {
        // Keep the cache unloaded after a failed request. The next dialog
        // mount can retry instead of treating an empty fallback as real.
      })
      .finally(releaseRequest);
    return () => {
      cancelled = true;
      controller.abort();
      releaseRequest();
    };
  }, [
    enabled,
    forceRefresh,
    isLoaded,
    setRepositories,
    setRepositoriesLoading,
    store,
    workspaceId,
  ]);

  return { repositories, isLoading, refresh };
}
