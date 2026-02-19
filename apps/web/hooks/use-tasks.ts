import { useMemo } from "react";
import { useWorkflowSnapshot } from "@/hooks/use-workflow-snapshot";
import { useAppStore } from "@/components/state-provider";

export function useTasks(workflowId: string | null) {
  useWorkflowSnapshot(workflowId);

  const kanbanWorkflowId = useAppStore((state) => state.kanban.workflowId);
  const tasks = useAppStore((state) => state.kanban.tasks);

  const workflowTasks = useMemo(() => {
    if (!workflowId || kanbanWorkflowId !== workflowId) {
      return [];
    }
    return tasks;
  }, [workflowId, kanbanWorkflowId, tasks]);

  return { tasks: workflowTasks };
}
