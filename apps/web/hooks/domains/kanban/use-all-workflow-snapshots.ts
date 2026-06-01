"use client";

import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { multiKanbanQueryOptions } from "@/lib/query/query-options/kanban";
import { qk } from "@/lib/query/keys";

/**
 * Fetches all workflow snapshots for `workspaceId` into the TanStack Query
 * cache at `qk.kanban.multi()`. The kanban bridge keeps it fresh from WS events;
 * consumers (board, swimlanes, sidebar, mention items, recent-task switcher)
 * read the cache via `useKanbanMultiSnapshots` / `useQuery`.
 *
 * Preserved signature: `useAllWorkflowSnapshots(workspaceId)`.
 */
export function useAllWorkflowSnapshots(workspaceId: string | null): void {
  const queryClient = useQueryClient();

  // Main fetch — disabled when no workspace selected.
  useQuery({
    ...multiKanbanQueryOptions(workspaceId ?? ""),
    enabled: !!workspaceId,
  });

  // On workspace change: invalidate the multi cache so we re-fetch fresh
  // snapshots for the newly-active workspace.
  useEffect(() => {
    if (!workspaceId) return;
    queryClient.invalidateQueries({ queryKey: qk.kanban.multi() });
  }, [workspaceId, queryClient]);
}
