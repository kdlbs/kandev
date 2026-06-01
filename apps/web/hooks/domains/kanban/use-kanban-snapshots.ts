"use client";

import { useCallback, useEffect, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { fetchWorkflowSnapshot } from "@/lib/api/domains/kanban-api";
import { qk } from "@/lib/query/keys";
import {
  multiKanbanQueryOptions,
  snapshotToWorkflowSnapshotData,
  workflowsListQueryOptions,
  type KanbanMultiData,
  type WorkflowsListData,
} from "@/lib/query/query-options/kanban";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";

const EMPTY_SNAPSHOTS: Record<string, WorkflowSnapshotData> = {};
const EMPTY_WORKFLOWS: WorkflowsListData = [];

/**
 * Reads all workflow snapshots for the active workspace from the TanStack
 * Query `qk.kanban.multi()` cache (populated by `useAllWorkflowSnapshots` and
 * kept fresh by the kanban bridge). Read-only — observes the cache without
 * triggering its own fetch when `enabled` is false; otherwise fetches.
 */
export function useKanbanMultiSnapshots(options?: { enabled?: boolean }): {
  snapshots: Record<string, WorkflowSnapshotData>;
  isLoading: boolean;
} {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { data, isFetching } = useQuery({
    ...multiKanbanQueryOptions(workspaceId ?? ""),
    enabled: (options?.enabled ?? true) && !!workspaceId,
  });
  const snapshots = (data as KanbanMultiData | undefined)?.snapshots ?? EMPTY_SNAPSHOTS;
  const isLoading = isFetching && Object.keys(snapshots).length === 0;
  return { snapshots, isLoading };
}

/**
 * Reads the workspace-scoped workflows list from the TanStack Query
 * `qk.kanban.workflowsList()` cache. Pass an explicit `workspaceId` to scope a
 * specific workspace; defaults to the active workspace. Observe-only: relies on
 * the cache being populated elsewhere (the kanban board / pickers fetch it).
 */
export function useWorkflowItems(workspaceIdArg?: string | null): WorkflowsListData {
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const workspaceId = workspaceIdArg === undefined ? activeWorkspaceId : workspaceIdArg;
  const { data } = useQuery({
    ...workflowsListQueryOptions(workspaceId ?? ""),
    enabled: !!workspaceId,
  });
  return data ?? EMPTY_WORKFLOWS;
}

/**
 * Optimistically reorders the workspace-scoped workflows-list cache to match
 * `workflowIds` (any items not listed are appended, preserving their order).
 * Mirrors the deleted Zustand `reorderWorkflowItems` action.
 */
export function useReorderWorkflowItems() {
  const queryClient = useQueryClient();
  return useCallback(
    (workspaceId: string, workflowIds: string[]): void => {
      queryClient.setQueryData<WorkflowsListData>(qk.kanban.workflowsList(workspaceId), (prev) => {
        if (!prev) return prev;
        const byId = new Map(prev.map((w) => [w.id, w]));
        const reordered = workflowIds
          .map((id) => byId.get(id))
          .filter((w): w is NonNullable<typeof w> => w != null);
        for (const item of prev) {
          if (!workflowIds.includes(item.id)) reordered.push(item);
        }
        return reordered;
      });
    },
    [queryClient],
  );
}

/**
 * Derives the active task's workflow context (`workflowId` + `steps`) from the
 * TanStack Query snapshot cache. Used by the task-page sidebar/header where the
 * "task context" workflow (the snapshot containing the active task) differs from
 * the homepage `workflows.activeId` filter — selecting the task's snapshot keeps
 * an "All Workflows" homepage filter from being clobbered during navigation.
 *
 * Resolution order: the snapshot containing `activeTaskId`; otherwise, when the
 * cache holds exactly one snapshot (the SSR-seeded task workflow), that one.
 */
export function useActiveTaskWorkflow(activeTaskId: string | null): {
  workflowId: string | null;
  steps: WorkflowSnapshotData["steps"];
} {
  const { snapshots } = useKanbanMultiSnapshots({ enabled: false });
  return useMemo(() => {
    const entries = Object.entries(snapshots);
    if (activeTaskId) {
      const match = entries.find(([, snap]) => snap.tasks.some((t) => t.id === activeTaskId));
      if (match) return { workflowId: match[0], steps: match[1].steps };
    }
    if (entries.length === 1) {
      const [wfId, snap] = entries[0];
      return { workflowId: wfId, steps: snap.steps };
    }
    return { workflowId: null, steps: [] };
  }, [snapshots, activeTaskId]);
}

/**
 * Ensures the snapshot for `workflowId` is present in the `qk.kanban.multi()`
 * cache, fetching it once if absent (and on workflowId change / reconnect). This
 * is the task-page counterpart to the home page's `useAllWorkflowSnapshots`,
 * which fetches every workspace workflow; here we only need the active task's
 * single workflow. Returns the per-workflow loading flag.
 */
export function useEnsureWorkflowSnapshot(workflowId: string | null): { isLoading: boolean } {
  const queryClient = useQueryClient();
  const connectionStatus = useAppStore((s) => s.connection.status);
  const { snapshots } = useKanbanMultiSnapshots({ enabled: false });
  // Only fetch when the multi cache doesn't already cover this workflow. The
  // workflow is considered covered if its snapshot is present, OR if the cache
  // is non-empty (the home page / `useAllWorkflowSnapshots` populates every
  // workspace workflow, so a non-empty cache is authoritative).
  const alreadyCached = !!(
    workflowId &&
    (snapshots[workflowId] || Object.keys(snapshots).length > 0)
  );
  const { data, isFetching } = useQuery({
    queryKey: ["kanban", "workflow-snapshot", workflowId ?? ""],
    queryFn: async (): Promise<WorkflowSnapshotData | null> => {
      if (!workflowId) return null;
      const raw = await fetchWorkflowSnapshot(workflowId, { cache: "no-store" });
      const name = raw.workflow?.name ?? workflowId;
      return snapshotToWorkflowSnapshotData(workflowId, name, raw);
    },
    enabled: !!workflowId && !alreadyCached,
    staleTime: 30_000,
  });

  useEffect(() => {
    if (!workflowId || !data) return;
    queryClient.setQueryData<KanbanMultiData>(qk.kanban.multi(), (prev) => {
      if (!prev) return { snapshots: { [workflowId]: data } };
      return { ...prev, snapshots: { ...prev.snapshots, [workflowId]: data } };
    });
  }, [workflowId, data, queryClient, connectionStatus]);

  return { isLoading: !alreadyCached && !!workflowId && isFetching && !data };
}

/**
 * Imperative read/write access to the `qk.kanban.multi()` snapshot cache, for
 * optimistic drag-and-drop / multi-select mutations that previously wrote the
 * Zustand `kanbanMulti` mirror. Mutations are local cache patches; the kanban
 * bridge reconciles them against authoritative WS events.
 */
export function useKanbanSnapshotMutator() {
  const queryClient = useQueryClient();

  const getSnapshot = useCallback(
    (workflowId: string): WorkflowSnapshotData | undefined =>
      queryClient.getQueryData<KanbanMultiData>(qk.kanban.multi())?.snapshots[workflowId],
    [queryClient],
  );

  const setSnapshot = useCallback(
    (workflowId: string, snapshot: WorkflowSnapshotData): void => {
      queryClient.setQueryData<KanbanMultiData>(qk.kanban.multi(), (prev) => {
        if (!prev) return { snapshots: { [workflowId]: snapshot } };
        return { ...prev, snapshots: { ...prev.snapshots, [workflowId]: snapshot } };
      });
    },
    [queryClient],
  );

  const getSnapshots = useCallback(
    (): Record<string, WorkflowSnapshotData> =>
      queryClient.getQueryData<KanbanMultiData>(qk.kanban.multi())?.snapshots ?? EMPTY_SNAPSHOTS,
    [queryClient],
  );

  return { getSnapshot, setSnapshot, getSnapshots };
}
