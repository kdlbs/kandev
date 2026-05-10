import { useMemo } from "react";
import { useWorkflowSnapshot } from "@/hooks/use-workflow-snapshot";
import { useAppStore } from "@/components/state-provider";

export function useTasks(workflowId: string | null) {
  useWorkflowSnapshot(workflowId);

  const kanbanWorkflowId = useAppStore((state) => state.kanban.workflowId);
  const kanbanIsLoading = useAppStore((state) => state.kanban.isLoading ?? false);
  const tasks = useAppStore((state) => state.kanban.tasks);

  const matchesActive = !!workflowId && kanbanWorkflowId === workflowId;
  const workflowTasks = useMemo(() => (matchesActive ? tasks : []), [matchesActive, tasks]);

  // Skeleton shows only while a snapshot fetch is actively in flight. Once the
  // fetch settles (success or failure) `kanban.isLoading` drops to false, so
  // a failed fetch falls back to the empty-state UI rather than spinning
  // forever. See `useWorkflowSnapshot` for the in-flight signal.
  const isLoading = !!workflowId && kanbanIsLoading;

  return { tasks: workflowTasks, isLoading };
}
