import { useAppStore } from "@/components/state-provider";
import { useTaskDependencies } from "@/hooks/domains/task/use-task-dependencies";
import { useTaskSubtasks } from "@/hooks/domains/kanban/use-task-subtasks";
import { useSubtaskCountState } from "@/hooks/use-subtask-count";

function useStoreParentId(taskId: string | null): string | null {
  return useAppStore((state) => {
    if (!taskId) return null;
    const fromBoard = state.kanban?.tasks?.find((t) => t.id === taskId);
    if (fromBoard) return fromBoard.parentTaskId ?? null;
    for (const snapshot of Object.values(state.kanbanMulti?.snapshots ?? {})) {
      const hit = snapshot.tasks?.find((t) => t.id === taskId);
      if (hit) return hit.parentTaskId ?? null;
    }
    return null;
  });
}

/**
 * Whether a task has anything to graph: a dependency edge, a parent, or a
 * subtask. The server subtask count is read only when the store cannot answer.
 */
export function useTaskGraphAvailable(taskId: string | null): boolean {
  const deps = useTaskDependencies(taskId);
  const parentId = useStoreParentId(taskId);
  const subtasks = useTaskSubtasks(taskId);
  const hasRelated = useHasRelatedTasks(taskId);
  const knownFromStore = Boolean(deps) || Boolean(parentId) || subtasks.length > 0 || hasRelated;
  const serverCount = useSubtaskCountState(Boolean(taskId) && !knownFromStore, taskId ?? undefined);
  if (!taskId) return false;
  return knownFromStore || serverCount.total > 0;
}

function useHasRelatedTasks(taskId: string | null): boolean {
  return useAppStore((state) => {
    if (!taskId) return false;
    const task =
      state.kanban?.tasks?.find((item) => item.id === taskId) ??
      Object.values(state.kanbanMulti?.snapshots ?? {})
        .flatMap((snapshot) => snapshot.tasks ?? [])
        .find((item) => item.id === taskId);
    return Boolean(task?.related?.length);
  });
}
