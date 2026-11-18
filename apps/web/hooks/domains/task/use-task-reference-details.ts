import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";

/** Read optional preview details without acquiring a task subscription. */
export function useTaskReferenceDetails(taskId: string) {
  const task = useAppStore((state) => state.taskOverview.byId[taskId] ?? null);
  const parentTask = useAppStore((state) =>
    task?.parentTaskId ? (state.taskOverview.byId[task.parentTaskId] ?? null) : null,
  );
  const createdTimestamp =
    task?.createdAt && parseTurnTimestamp(task.createdAt) !== null
      ? Date.parse(task.createdAt)
      : null;
  const workflows = useAppStore((state) => state.workflows.items);
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const board = useAppStore((state) => state.kanban);
  const workflowName = useMemo(() => {
    if (!task) return null;
    if (task.workflowId) {
      return (
        workflows.find((workflow) => workflow.id === task.workflowId)?.name ??
        snapshots[task.workflowId]?.workflowName ??
        null
      );
    }
    const snapshot = Object.values(snapshots).find((entry) =>
      entry.tasks.some((item) => item.id === taskId),
    );
    const workflowId =
      snapshot?.workflowId ??
      (board.tasks.some((item) => item.id === taskId) ? board.workflowId : null);
    return (
      workflows.find((workflow) => workflow.id === workflowId)?.name ??
      snapshot?.workflowName ??
      null
    );
  }, [task, taskId, workflows, snapshots, board]);
  return { task, workflowName, parentTask, createdTimestamp };
}
