import { useMemo } from "react";
import { useWorkflowSnapshot } from "@/hooks/use-workflow-snapshot";

export function useTasks(workflowId: string | null) {
  const snapshot = useWorkflowSnapshot(workflowId);

  const matchesActive = !!workflowId && snapshot.snapshotState?.workflowId === workflowId;
  const workflowTasks = useMemo(
    () => (matchesActive ? (snapshot.snapshotState?.tasks ?? []) : []),
    [matchesActive, snapshot.snapshotState?.tasks],
  );

  // Loading only while a snapshot fetch is in-flight; settles to false on success/error to avoid an infinite skeleton.
  const isLoading = !!workflowId && snapshot.isFetching && workflowTasks.length === 0;

  return { tasks: workflowTasks, isLoading };
}
