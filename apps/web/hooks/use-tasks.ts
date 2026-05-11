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

  // Loading only while a snapshot fetch is in-flight; settles to false on success/error to avoid an infinite skeleton.
  const isLoading = !!workflowId && kanbanIsLoading;

  return { tasks: workflowTasks, isLoading };
}
