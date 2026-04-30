"use client";

import { useEffect, useCallback, useRef } from "react";
import {
  listJiraIssueWatches,
  createJiraIssueWatch,
  updateJiraIssueWatch,
  deleteJiraIssueWatch,
  triggerJiraIssueWatch,
} from "@/lib/api/domains/jira-api";
import { useAppStore } from "@/components/state-provider";
import type { CreateJiraIssueWatchInput, UpdateJiraIssueWatchInput } from "@/lib/types/jira";

/**
 * useJiraIssueWatches owns the JIRA-watcher list for a workspace: it fetches
 * once per workspace and exposes CRUD callbacks that mirror the GitHub
 * useIssueWatches hook so UI components can be ported with minimal friction.
 *
 * The store's `loaded` flag is global, not workspace-scoped — so a `workspaceId`
 * change (workspace switch, navigating back to settings) needs to reset the
 * cached list before the fetch effect runs, otherwise the user sees the
 * previous workspace's watchers stale-rendered.
 */
export function useJiraIssueWatches(workspaceId: string | null) {
  const items = useAppStore((s) => s.jiraIssueWatches.items);
  const loaded = useAppStore((s) => s.jiraIssueWatches.loaded);
  const loading = useAppStore((s) => s.jiraIssueWatches.loading);
  const setWatches = useAppStore((s) => s.setJiraIssueWatches);
  const setLoading = useAppStore((s) => s.setJiraIssueWatchesLoading);
  const addWatch = useAppStore((s) => s.addJiraIssueWatch);
  const updateWatch = useAppStore((s) => s.updateJiraIssueWatch);
  const removeWatch = useAppStore((s) => s.removeJiraIssueWatch);

  const lastWorkspaceId = useRef<string | null>(null);

  useEffect(() => {
    if (!workspaceId) return;
    // Workspace changed — invalidate the cached list so the fetch effect below
    // re-runs against the new workspace instead of short-circuiting on stale
    // `loaded`. Skipped on the first mount when ref === workspaceId already.
    if (lastWorkspaceId.current !== null && lastWorkspaceId.current !== workspaceId) {
      setWatches([]);
    }
    lastWorkspaceId.current = workspaceId;
  }, [workspaceId, setWatches]);

  useEffect(() => {
    if (!workspaceId || loaded || loading) return;
    setLoading(true);
    listJiraIssueWatches(workspaceId, { cache: "no-store" })
      .then((res) => setWatches(res ?? []))
      .catch(() => setWatches([]))
      .finally(() => setLoading(false));
  }, [workspaceId, loaded, loading, setWatches, setLoading]);

  const create = useCallback(
    async (req: CreateJiraIssueWatchInput) => {
      const watch = await createJiraIssueWatch(req);
      addWatch(watch);
      return watch;
    },
    [addWatch],
  );

  const update = useCallback(
    async (id: string, req: UpdateJiraIssueWatchInput) => {
      const watch = await updateJiraIssueWatch(id, req);
      updateWatch(watch);
      return watch;
    },
    [updateWatch],
  );

  const remove = useCallback(
    async (id: string) => {
      await deleteJiraIssueWatch(id);
      removeWatch(id);
    },
    [removeWatch],
  );

  const trigger = useCallback(async (id: string) => {
    return triggerJiraIssueWatch(id);
  }, []);

  return { items, loaded, loading, create, update, remove, trigger };
}
