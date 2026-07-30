import { useAppStore } from "@/components/state-provider";

/**
 * Resolves which session an "Enhance note with AI" request should target.
 *
 * When the note panel shows the currently active task, the live active
 * session is used so switching session tabs is reflected immediately.
 * Otherwise (e.g. the kanban "Edit notes" shortcut opened for a task that
 * isn't active) the task's own primary session is used instead, so the
 * request never lands on an unrelated task's conversation.
 */
export function resolveNoteSessionId(params: {
  taskId: string | null;
  activeTaskId: string | null;
  activeSessionId: string | null;
  taskPrimarySessionId: string | null;
}): string | null {
  const { taskId, activeTaskId, activeSessionId, taskPrimarySessionId } = params;
  if (taskId !== null && taskId === activeTaskId) return activeSessionId;
  return taskPrimarySessionId;
}

export function useNoteSessionId(taskId: string | null): string | null {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const taskPrimarySessionId = useAppStore(
    (state) => state.kanban.tasks.find((t) => t.id === taskId)?.primarySessionId ?? null,
  );
  return resolveNoteSessionId({ taskId, activeTaskId, activeSessionId, taskPrimarySessionId });
}
