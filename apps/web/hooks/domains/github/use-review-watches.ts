"use client";

import { useEffect, useCallback, useRef } from "react";
import {
  listReviewWatches,
  createReviewWatch,
  updateReviewWatch,
  deleteReviewWatch,
  triggerReviewWatch,
  triggerAllReviewWatches,
  previewResetReviewWatch,
  resetReviewWatch,
} from "@/lib/api/domains/github-api";
import { useAppStore } from "@/components/state-provider";
import type { CreateReviewWatchRequest, UpdateReviewWatchRequest } from "@/lib/types/github";

// useReviewWatches has three modes:
//   - workspaceId: string         → fetch watches scoped to one workspace
//   - workspaceId: undefined      → fetch watches across all workspaces
//   - workspaceId: null           → don't fetch (caller hasn't resolved a workspace yet)
export function useReviewWatches(workspaceId?: string | null) {
  const items = useAppStore((state) => state.reviewWatches.items);
  const loaded = useAppStore((state) => state.reviewWatches.loaded);
  const loading = useAppStore((state) => state.reviewWatches.loading);
  const setReviewWatches = useAppStore((state) => state.setReviewWatches);
  const setReviewWatchesLoading = useAppStore((state) => state.setReviewWatchesLoading);
  const addWatch = useAppStore((state) => state.addReviewWatch);
  const updateWatch = useAppStore((state) => state.updateReviewWatch);
  const removeWatch = useAppStore((state) => state.removeReviewWatch);
  const loadedScopeRef = useRef<string | null>(null);

  useEffect(() => {
    if (workspaceId === null) return;
    const scopeKey = workspaceId ?? "__all__";
    if (loadedScopeRef.current === scopeKey) return;
    let cancelled = false;
    loadedScopeRef.current = scopeKey;
    setReviewWatchesLoading(true);
    listReviewWatches(workspaceId ?? undefined, { cache: "no-store" })
      .then((response) => {
        if (cancelled) return;
        setReviewWatches(response?.watches ?? []);
      })
      .catch(() => {
        if (cancelled) return;
        setReviewWatches([]);
      })
      .finally(() => {
        if (cancelled) return;
        setReviewWatchesLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, setReviewWatches, setReviewWatchesLoading]);

  const create = useCallback(
    async (req: CreateReviewWatchRequest) => {
      const watch = await createReviewWatch(req);
      addWatch(watch);
      return watch;
    },
    [addWatch],
  );

  const update = useCallback(
    async (id: string, watchWorkspaceId: string, req: UpdateReviewWatchRequest) => {
      const watch = await updateReviewWatch(id, watchWorkspaceId, req);
      updateWatch(watch);
      return watch;
    },
    [updateWatch],
  );

  const remove = useCallback(
    async (id: string, watchWorkspaceId: string) => {
      await deleteReviewWatch(id, watchWorkspaceId);
      removeWatch(id);
    },
    [removeWatch],
  );

  const trigger = useCallback(async (id: string, watchWorkspaceId: string) => {
    return triggerReviewWatch(id, watchWorkspaceId);
  }, []);

  const triggerAll = useCallback(async () => {
    if (!workspaceId) return null;
    return triggerAllReviewWatches(workspaceId);
  }, [workspaceId]);

  const previewReset = useCallback(async (id: string, watchWorkspaceId: string) => {
    return previewResetReviewWatch(id, watchWorkspaceId);
  }, []);

  const reset = useCallback(
    async (id: string, watchWorkspaceId: string) => {
      const result = await resetReviewWatch(id, watchWorkspaceId);
      try {
        const response = await listReviewWatches(workspaceId ?? undefined, { cache: "no-store" });
        setReviewWatches(response?.watches ?? []);
      } catch {
        // Reset succeeded; a stale settings table is less harmful than failing the action.
      }
      return result;
    },
    [setReviewWatches, workspaceId],
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
