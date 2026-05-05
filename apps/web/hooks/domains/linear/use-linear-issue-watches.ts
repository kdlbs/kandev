"use client";

import { useEffect, useCallback, useRef } from "react";
import {
  listLinearIssueWatches,
  createLinearIssueWatch,
  updateLinearIssueWatch,
  deleteLinearIssueWatch,
  triggerLinearIssueWatch,
} from "@/lib/api/domains/linear-api";
import { useAppStore } from "@/components/state-provider";
import type { CreateLinearIssueWatchInput, UpdateLinearIssueWatchInput } from "@/lib/types/linear";

/**
 * useLinearIssueWatches owns the Linear-watcher list:
 *   - workspaceId: string    → fetch and operate on watches in one workspace
 *   - workspaceId: undefined → fetch every watch across all workspaces; the
 *                              caller supplies workspaceId to update/remove/trigger
 *                              calls per-row (those endpoints still validate it
 *                              against the watch's stored workspace as an IDOR guard)
 *   - workspaceId: null      → don't fetch
 *
 * Mirrors `useJiraIssueWatches`. Workspace changes reset the cached list so
 * the user doesn't see the previous workspace's stale rows during the swap.
 */
export function useLinearIssueWatches(workspaceId?: string | null) {
  const items = useAppStore((s) => s.linearIssueWatches.items);
  const loaded = useAppStore((s) => s.linearIssueWatches.loaded);
  const loading = useAppStore((s) => s.linearIssueWatches.loading);
  const setWatches = useAppStore((s) => s.setLinearIssueWatches);
  const resetWatches = useAppStore((s) => s.resetLinearIssueWatches);
  const setLoading = useAppStore((s) => s.setLinearIssueWatchesLoading);
  const addWatch = useAppStore((s) => s.addLinearIssueWatch);
  const updateWatch = useAppStore((s) => s.updateLinearIssueWatch);
  const removeWatch = useAppStore((s) => s.removeLinearIssueWatch);

  const lastScope = useRef<string | null | undefined>(undefined);
  const scope: string | null = workspaceId ?? null;

  useEffect(() => {
    if (workspaceId === null) return;
    if (lastScope.current !== undefined && lastScope.current !== scope) {
      resetWatches();
    }
    lastScope.current = scope;
  }, [workspaceId, scope, resetWatches]);

  useEffect(() => {
    if (workspaceId === null || loaded || loading) return;
    setLoading(true);
    listLinearIssueWatches(workspaceId ?? undefined, { cache: "no-store" })
      .then((res) => setWatches(res ?? []))
      .catch(() => setWatches([]))
      .finally(() => setLoading(false));
  }, [workspaceId, loaded, loading, setWatches, setLoading]);

  const create = useCallback(
    async (req: CreateLinearIssueWatchInput) => {
      const watch = await createLinearIssueWatch(req);
      addWatch(watch);
      return watch;
    },
    [addWatch],
  );

  // Per-row mutations require the row's own workspace_id to satisfy the
  // backend's IDOR guard. Callers pass it explicitly when the hook itself
  // wasn't bound to a single workspace (the install-wide listing case).
  const update = useCallback(
    async (id: string, req: UpdateLinearIssueWatchInput, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error("workspaceId required");
      const watch = await updateLinearIssueWatch(ws, id, req);
      updateWatch(watch);
      return watch;
    },
    [workspaceId, updateWatch],
  );

  const remove = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error("workspaceId required");
      await deleteLinearIssueWatch(ws, id);
      removeWatch(id);
    },
    [workspaceId, removeWatch],
  );

  const trigger = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error("workspaceId required");
      return triggerLinearIssueWatch(ws, id);
    },
    [workspaceId],
  );

  return { items, loaded, loading, create, update, remove, trigger };
}
