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
  const workflowName = useAppStore((state) => {
    if (!task) return null;
    const snapshot = Object.values(state.kanbanMulti.snapshots).find((entry) =>
      entry.tasks.some((item) => item.id === taskId),
    );
    const workflowId =
      task.workflowId ??
      snapshot?.workflowId ??
      (state.kanban.tasks.some((item) => item.id === taskId) ? state.kanban.workflowId : null);
    return (
      state.workflows.items.find((workflow) => workflow.id === workflowId)?.name ??
      snapshot?.workflowName ??
      null
    );
  });
  return { task, workflowName, parentTask, createdTimestamp };
}
