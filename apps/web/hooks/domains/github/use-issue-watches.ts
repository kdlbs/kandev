"use client";

import { useEffect, useCallback } from "react";
import {
  listIssueWatches,
  createIssueWatch,
  updateIssueWatch,
  deleteIssueWatch,
  triggerIssueWatch,
  triggerAllIssueWatches,
} from "@/lib/api/domains/github-api";
import { useAppStore } from "@/components/state-provider";
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

  useEffect(() => {
    if (workspaceId === null || loaded || loading) return;
    setIssueWatchesLoading(true);
    listIssueWatches(workspaceId ?? undefined, { cache: "no-store" })
      .then((response) => {
        setIssueWatches(response?.watches ?? []);
      })
      .catch(() => {
        setIssueWatches([]);
      })
      .finally(() => {
        setIssueWatchesLoading(false);
      });
  }, [workspaceId, loaded, loading, setIssueWatches, setIssueWatchesLoading]);

  const create = useCallback(
    async (req: CreateIssueWatchRequest) => {
      const watch = await createIssueWatch(req);
      addWatch(watch);
      return watch;
    },
    [addWatch],
  );

  const update = useCallback(
    async (id: string, req: UpdateIssueWatchRequest) => {
      const watch = await updateIssueWatch(id, req);
      updateWatch(watch);
      return watch;
    },
    [updateWatch],
  );

  const remove = useCallback(
    async (id: string) => {
      await deleteIssueWatch(id);
      removeWatch(id);
    },
    [removeWatch],
  );

  const trigger = useCallback(async (id: string) => {
    return triggerIssueWatch(id);
  }, []);

  const triggerAll = useCallback(async () => {
    if (!workspaceId) return null;
    return triggerAllIssueWatches(workspaceId);
  }, [workspaceId]);

  return {
    items,
    loaded,
    loading,
    create,
    update,
    remove,
    trigger,
    triggerAll,
  };
}
