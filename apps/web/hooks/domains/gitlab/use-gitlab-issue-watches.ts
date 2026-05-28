"use client";

import { useEffect, useCallback } from "react";
import {
  listIssueWatches,
  createIssueWatch,
  updateIssueWatch,
  deleteIssueWatch,
  triggerIssueWatch,
  triggerAllIssueWatches,
  type CreateIssueWatchRequest,
  type UpdateIssueWatchRequest,
} from "@/lib/api/domains/gitlab-api";
import { useAppStore } from "@/components/state-provider";

export function useGitLabIssueWatches(workspaceId?: string | null) {
  const items = useAppStore((state) => state.gitlabIssueWatches.items);
  const loaded = useAppStore((state) => state.gitlabIssueWatches.loaded);
  const loading = useAppStore((state) => state.gitlabIssueWatches.loading);
  const set = useAppStore((state) => state.setGitLabIssueWatches);
  const setLoading = useAppStore((state) => state.setGitLabIssueWatchesLoading);
  const add = useAppStore((state) => state.addGitLabIssueWatch);
  const upd = useAppStore((state) => state.updateGitLabIssueWatchInStore);
  const rm = useAppStore((state) => state.removeGitLabIssueWatch);

  useEffect(() => {
    if (workspaceId === null || loaded || loading) return;
    setLoading(true);
    listIssueWatches(workspaceId ?? undefined, { cache: "no-store" })
      .then((response) => set(response?.watches ?? []))
      .catch(() => set([]))
      .finally(() => setLoading(false));
  }, [workspaceId, loaded, loading, set, setLoading]);

  const create = useCallback(
    async (req: CreateIssueWatchRequest) => {
      const watch = await createIssueWatch(req);
      add(watch);
      return watch;
    },
    [add],
  );

  const update = useCallback(
    async (id: string, req: UpdateIssueWatchRequest) => {
      const watch = await updateIssueWatch(id, req);
      upd(watch);
      return watch;
    },
    [upd],
  );

  const remove = useCallback(
    async (id: string) => {
      await deleteIssueWatch(id);
      rm(id);
    },
    [rm],
  );

  const trigger = useCallback((id: string) => triggerIssueWatch(id), []);
  const triggerAll = useCallback(() => triggerAllIssueWatches(), []);

  return { items, loaded, loading, create, update, remove, trigger, triggerAll };
}
