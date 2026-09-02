import { useRef, useState } from "react";
import type { Task } from "@/components/kanban-card";

/**
 * Wraps a move function with a per-task in-flight guard (AC-UI-PIPELINE-ROW-005.2):
 * the guard is held per task id, not list-wide, so one task's move settling
 * never re-enables or disables another task's still-in-flight move. A second
 * request for a task already in flight is ignored rather than started, so the
 * guard holds even when a caller invokes it before a disabling re-render lands.
 */
export function useTaskMoveGuard(moveTask: (task: Task, targetStepId: string) => Promise<void>) {
  const [movingTaskIds, setMovingTaskIds] = useState<ReadonlySet<string>>(() => new Set());
  const movingTaskIdsRef = useRef(movingTaskIds);
  movingTaskIdsRef.current = movingTaskIds;

  const handleMoveTask = async (task: Task, targetStepId: string) => {
    if (movingTaskIdsRef.current.has(task.id)) return;
    movingTaskIdsRef.current = new Set(movingTaskIdsRef.current).add(task.id);
    setMovingTaskIds(movingTaskIdsRef.current);
    try {
      await moveTask(task, targetStepId);
    } finally {
      const next = new Set(movingTaskIdsRef.current);
      next.delete(task.id);
      movingTaskIdsRef.current = next;
      setMovingTaskIds(next);
    }
  };

  return { movingTaskIds, handleMoveTask };
}
