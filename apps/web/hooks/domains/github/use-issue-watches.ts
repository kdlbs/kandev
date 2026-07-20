"use client";

import { useEffect, useCallback, useRef } from "react";
import {
  listIssueWatches,
  createIssueWatch,
  updateIssueWatch,
  deleteIssueWatch,
  triggerIssueWatch,
  triggerAllIssueWatches,
  previewResetIssueWatch,
  resetIssueWatch,
} from "@/lib/api/domains/github-api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { CreateIssueWatchRequest, UpdateIssueWatchRequest } from "@/lib/types/github";

// useIssueWatches has three modes:
//   - workspaceId: string         → fetch watches scoped to one workspace
//   - workspaceId: undefined      → fetch watches across all workspaces
//   - workspaceId: null           → don't fetch (caller hasn't resolved a workspace yet)
export function useIssueWatches(workspaceId?: string | null) {
  const items = useAppStore((state) => state.issueWatches.items);
  const loaded = useAppStore((state) => state.issueWatches.loaded);
  const loading = useAppStore((state) => state.issueWatches.loading);
  const setIssueWatches = useAppStore((state) => state.setIssueWatches);
  const setIssueWatchesLoading = useAppStore((state) => state.setIssueWatchesLoading);
  const addWatch = useAppStore((state) => state.addIssueWatch);
  const updateWatch = useAppStore((state) => state.updateIssueWatch);
  const removeWatch = useAppStore((state) => state.removeIssueWatch);
  const loadedScopeRef = useRef<string | null>(null);
  // storeApi exposes getState() without subscribing — used in reset() to
  // read the current watch row outside of the React render cycle so the
  // callback doesn't need `items` as a dependency.
  const storeApi = useAppStoreApi();

  useEffect(() => {
    if (workspaceId === null) return;
    const scopeKey = workspaceId ?? "__all__";
    if (loadedScopeRef.current === scopeKey) return;
    let cancelled = false;
    loadedScopeRef.current = scopeKey;
    setIssueWatchesLoading(true);
    listIssueWatches(workspaceId ?? undefined, { cache: "no-store" })
      .then((response) => {
        if (cancelled) return;
        setIssueWatches(response?.watches ?? []);
      })
      .catch(() => {
        if (cancelled) return;
        setIssueWatches([]);
      })
      .finally(() => {
        if (cancelled) return;
        setIssueWatchesLoading(false);
      });
    return () => {
      cancelled = true;
      // Clear the scope guard so a same-scope re-mount (React StrictMode's
      // double-invoke, or an effect re-run) re-issues the fetch. Without this,
      // the cancelled first fetch drops its response while the guard blocks the
      // second run, leaving the store stuck at loading with no items.
      if (loadedScopeRef.current === scopeKey) loadedScopeRef.current = null;
    };
  }, [workspaceId, setIssueWatches, setIssueWatchesLoading]);

  const create = useCallback(
    async (req: CreateIssueWatchRequest) => {
      const watch = await createIssueWatch(req);
      addWatch(watch);
      return watch;
    },
    [addWatch],
  );

  const update = useCallback(
    async (id: string, watchWorkspaceId: string, req: UpdateIssueWatchRequest) => {
      const watch = await updateIssueWatch(id, watchWorkspaceId, req);
      updateWatch(watch);
      return watch;
    },
    [updateWatch],
  );

  const remove = useCallback(
    async (id: string, watchWorkspaceId: string) => {
      await deleteIssueWatch(id, watchWorkspaceId);
      removeWatch(id);
    },
    [removeWatch],
  );

  const trigger = useCallback(async (id: string, watchWorkspaceId: string) => {
    return triggerIssueWatch(id, watchWorkspaceId);
  }, []);

  const triggerAll = useCallback(async () => {
    if (!workspaceId) return null;
    return triggerAllIssueWatches(workspaceId);
  }, [workspaceId]);

  const previewReset = useCallback(async (id: string, watchWorkspaceId: string) => {
    return previewResetIssueWatch(id, watchWorkspaceId);
  }, []);

  const reset = useCallback(
    async (id: string, watchWorkspaceId: string) => {
      const res = await resetIssueWatch(id, watchWorkspaceId);
      // Patch the cached watch so the "Last polled" column reflects the
      // reset immediately without waiting for the next poll tick.
      const current = storeApi.getState().issueWatches.items.find((w) => w.id === id);
      if (current) updateWatch({ ...current, last_polled_at: null });
      return res;
    },
    [storeApi, updateWatch],
  );

  return {
    items,
    loaded,
    loading,
    create,
    update,
    remove,
    trigger,
    triggerAll,
    previewReset,
    reset,
  };
}
