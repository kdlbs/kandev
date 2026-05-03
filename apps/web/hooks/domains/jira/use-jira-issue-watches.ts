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
 * useJiraIssueWatches owns the JIRA-watcher list:
 *   - workspaceId: string    → fetch and operate on watches in one workspace
 *   - workspaceId: undefined → fetch every watch across all workspaces; the
 *                              caller supplies workspaceId to update/remove/trigger
 *                              calls per-row (those endpoints still validate it
 *                              against the watch's stored workspace as an IDOR guard)
 *   - workspaceId: null      → don't fetch
 *
 * Workspace changes reset the cached list so the user doesn't see the previous
 * workspace's stale rows during the swap.
 */
export function useJiraIssueWatches(workspaceId?: string | null) {
  const items = useAppStore((s) => s.jiraIssueWatches.items);
  const loaded = useAppStore((s) => s.jiraIssueWatches.loaded);
  const loading = useAppStore((s) => s.jiraIssueWatches.loading);
  const setWatches = useAppStore((s) => s.setJiraIssueWatches);
  const resetWatches = useAppStore((s) => s.resetJiraIssueWatches);
  const setLoading = useAppStore((s) => s.setJiraIssueWatchesLoading);
  const addWatch = useAppStore((s) => s.addJiraIssueWatch);
  const updateWatch = useAppStore((s) => s.updateJiraIssueWatch);
  const removeWatch = useAppStore((s) => s.removeJiraIssueWatch);

  const lastScope = useRef<string | null | undefined>(undefined);
  const scope: string | null = workspaceId ?? null;

  useEffect(() => {
    if (workspaceId === null) return;
    // Scope changed (workspace switch or all↔scoped flip) — invalidate so the
    // fetch effect re-runs. setWatches([]) would keep loaded=true; resetWatches
    // clears loaded so the fetch isn't short-circuited by the stale guard.
    if (lastScope.current !== undefined && lastScope.current !== scope) {
      resetWatches();
    }
    lastScope.current = scope;
  }, [workspaceId, scope, resetWatches]);

  useEffect(() => {
    if (workspaceId === null || loaded || loading) return;
    setLoading(true);
    listJiraIssueWatches(workspaceId ?? undefined, { cache: "no-store" })
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

  // Per-row mutations require the row's own workspace_id to satisfy the
  // backend's IDOR guard. Callers pass it explicitly when the hook itself
  // wasn't bound to a single workspace (the install-wide listing case).
  const update = useCallback(
    async (id: string, req: UpdateJiraIssueWatchInput, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error("workspaceId required");
      const watch = await updateJiraIssueWatch(ws, id, req);
      updateWatch(watch);
      return watch;
    },
    [workspaceId, updateWatch],
  );

  const remove = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error("workspaceId required");
      await deleteJiraIssueWatch(ws, id);
      removeWatch(id);
    },
    [workspaceId, removeWatch],
  );

  const trigger = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error("workspaceId required");
      return triggerJiraIssueWatch(ws, id);
    },
    [workspaceId],
  );

  return { items, loaded, loading, create, update, remove, trigger };
}
