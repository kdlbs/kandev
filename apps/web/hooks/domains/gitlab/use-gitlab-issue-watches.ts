"use client";

import { useEffect, useCallback } from "react";
import {
  listIssueWatches,
  createIssueWatch,
  updateIssueWatch,
  deleteIssueWatch,
  triggerIssueWatch,
  triggerAllIssueWatches,
  previewResetIssueWatch,
  resetIssueWatch,
  type CreateIssueWatchRequest,
  type UpdateIssueWatchRequest,
} from "@/lib/api/domains/gitlab-api";
import { useAppStore } from "@/components/state-provider";

const WORKSPACE_REQUIRED = "workspaceId required";

function filterByWorkspace<T extends { workspace_id: string }>(
  items: T[],
  workspaceId?: string | null,
) {
  return workspaceId ? items.filter((watch) => watch.workspace_id === workspaceId) : [];
}

export function useGitLabIssueWatches(workspaceId: string | null) {
  const items = useAppStore((state) => state.gitlabIssueWatches.items);
  const loaded = useAppStore((state) => state.gitlabIssueWatches.loaded);
  const loading = useAppStore((state) => state.gitlabIssueWatches.loading);
  const set = useAppStore((state) => state.setGitLabIssueWatches);
  const setLoading = useAppStore((state) => state.setGitLabIssueWatchesLoading);
  const add = useAppStore((state) => state.addGitLabIssueWatch);
  const upd = useAppStore((state) => state.updateGitLabIssueWatchInStore);
  const rm = useAppStore((state) => state.removeGitLabIssueWatch);

  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    setLoading(true);
    listIssueWatches(workspaceId, { cache: "no-store" })
      .then((response) => {
        if (!cancelled) set(response?.watches ?? []);
      })
      .catch(() => {
        if (!cancelled) set([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, set, setLoading]);

  const create = useCallback(
    async (req: CreateIssueWatchRequest) => {
      const watch = await createIssueWatch(req);
      add(watch);
      return watch;
    },
    [add],
  );

  const update = useCallback(
    async (id: string, req: UpdateIssueWatchRequest, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      const watch = await updateIssueWatch(id, ws, req);
      upd(watch);
      return watch;
    },
    [upd, workspaceId],
  );

  const remove = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      await deleteIssueWatch(id, ws);
      rm(id);
    },
    [rm, workspaceId],
  );

  const trigger = useCallback(
    (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      return triggerIssueWatch(id, ws);
    },
    [workspaceId],
  );
  const triggerAll = useCallback(() => {
    if (!workspaceId) throw new Error(WORKSPACE_REQUIRED);
    return triggerAllIssueWatches(workspaceId);
  }, [workspaceId]);
  const previewReset = useCallback(
    (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      return previewResetIssueWatch(id, ws);
    },
    [workspaceId],
  );
  const reset = useCallback(
    (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      return resetIssueWatch(id, ws);
    },
    [workspaceId],
  );

  return {
    items: filterByWorkspace(items, workspaceId),
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
