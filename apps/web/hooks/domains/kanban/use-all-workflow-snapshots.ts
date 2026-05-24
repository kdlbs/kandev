"use client";

import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { multiKanbanQueryOptions } from "@/lib/query/query-options/kanban";
import { qk } from "@/lib/query/keys";

/**
 * Fetches all workflow snapshots for `workspaceId` into the TanStack Query
 * cache at `qk.kanban.multi()`.
 *
 * Replaces the old Zustand-based hook: no longer writes into `kanbanMulti`.
 * Consumers read from the TQ cache via `kanbanQueryOptions.multi(wsId)` or
 * `kanbanQueryOptions.workflow(wsId, wfId)` (derived via `select`).
 *
 * Preserved signature: `useAllWorkflowSnapshots(workspaceId)`.
 */
export function useAllWorkflowSnapshots(workspaceId: string | null): void {
  const queryClient = useQueryClient();

  // Main fetch — disabled when no workspace selected
  const { data: _data } = useQuery({
    ...multiKanbanQueryOptions(workspaceId ?? ""),
    enabled: !!workspaceId,
  });

  // On workspace change: invalidate the multi cache so we re-fetch fresh
  // snapshots. This mirrors the old `clearKanbanMulti()` + re-fetch pattern.
  useEffect(() => {
    if (!workspaceId) return;
    queryClient.invalidateQueries({ queryKey: qk.kanban.multi() });
  }, [workspaceId, queryClient]);
}
