"use client";

import { useEffect, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { multiKanbanQueryOptions } from "@/lib/query/query-options/kanban";
import { qk } from "@/lib/query/keys";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";

/**
 * Fetches all workflow snapshots for `workspaceId` into the TanStack Query
 * cache at `qk.kanban.multi()`, and bootstraps the Zustand
 * `kanbanMulti.snapshots` slice so legacy readers (kanban swimlanes, task
 * sidebar, sidebar filter, mention items, recent-task switcher, etc.) keep
 * rendering correctly during the transitional wave-3 migration.
 *
 * Mirror policy: write to Zustand when (a) the workspaceId changes (covers
 * workspace switch — even if the new workspace coincidentally exposes the
 * same workflow IDs, the snapshot contents differ), or (b) the SET of
 * workflow IDs changes (workflow create/delete). Per-task updates already
 * flow into the Zustand snapshot via `lib/ws/handlers/tasks.ts`, so
 * re-mirroring on every WS task event would just trigger a redundant
 * Zustand write + re-render storm — fatal for the timing-fragile multi-task
 * workflow E2E tests that rely on the mock agent processing many tasks
 * back-to-back.
 *
 * Preserved signature: `useAllWorkflowSnapshots(workspaceId)`.
 */
export function useAllWorkflowSnapshots(workspaceId: string | null): void {
  const queryClient = useQueryClient();
  const storeApi = useAppStoreApi();
  const setKanbanMultiSnapshots = useAppStore((s) => s.setKanbanMultiSnapshots);
  const lastMirroredWorkspaceId = useRef<string | null>(null);

  // Main fetch — disabled when no workspace selected
  const { data } = useQuery({
    ...multiKanbanQueryOptions(workspaceId ?? ""),
    enabled: !!workspaceId,
  });

  useEffect(() => {
    if (!data || !workspaceId) return;
    const incoming = data.snapshots;
    const workspaceChanged = lastMirroredWorkspaceId.current !== workspaceId;
    if (!workspaceChanged) {
      const current = storeApi.getState().kanbanMulti.snapshots;
      const currentKeys = Object.keys(current).sort().join("|");
      const incomingKeys = Object.keys(incoming).sort().join("|");
      if (currentKeys === incomingKeys) return; // task-level updates handled by Zustand WS handlers
    }
    setKanbanMultiSnapshots(incoming);
    lastMirroredWorkspaceId.current = workspaceId;
  }, [data, workspaceId, setKanbanMultiSnapshots, storeApi]);

  // On workspace change: invalidate the multi cache so we re-fetch fresh
  // snapshots. This mirrors the old `clearKanbanMulti()` + re-fetch pattern.
  useEffect(() => {
    if (!workspaceId) return;
    queryClient.invalidateQueries({ queryKey: qk.kanban.multi() });
  }, [workspaceId, queryClient]);
}
