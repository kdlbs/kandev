import { useMemo } from "react";
import {
  useEnsureWorkflowSnapshot,
  useKanbanMultiSnapshots,
} from "@/hooks/domains/kanban/use-kanban-snapshots";

export function useTasks(workflowId: string | null) {
  // Ensure the active task's workflow snapshot is in the TanStack Query
  // `qk.kanban.multi()` cache (fetches just this workflow if absent). On the home
  // page `useAllWorkflowSnapshots` already populates every workflow.
  const { isLoading: ensureLoading } = useEnsureWorkflowSnapshot(workflowId);

  // Observe the multi cache without triggering the full workspace fetch — the
  // single-workflow ensure above (or the home page) owns the fetch.
  const { snapshots } = useKanbanMultiSnapshots({ enabled: false });

  const tasks = useMemo(
    () => (workflowId ? (snapshots[workflowId]?.tasks ?? []) : []),
    [workflowId, snapshots],
  );
  const isLoading = !!workflowId && ensureLoading && !snapshots[workflowId];

  return { tasks, isLoading };
}
