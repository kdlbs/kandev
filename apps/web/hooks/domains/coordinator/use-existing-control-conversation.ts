import { useEffect, useState } from "react";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import type { Task } from "@/lib/types/http";

/** Resolve only an existing task; no prepare, conversation-open, or recovery request. */
export function useExistingControlConversation(
  workspaceId: string,
  coordinatorId: string,
  taskId: string,
) {
  const [result, setResult] = useState<{ taskId: string; task: Task | null; error: boolean }>();
  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    setResult(undefined);
    fetchTask(taskId, { cache: "no-store", init: { signal: controller.signal } })
      .then((task) => {
        if (!active) return;
        const matches =
          task.id === taskId &&
          task.workspace_id === workspaceId &&
          task.origin === "coordinator" &&
          task.metadata?.coordinator_id === coordinatorId;
        setResult({ taskId, task: matches ? task : null, error: !matches });
      })
      .catch(() => {
        if (active) setResult({ taskId, task: null, error: true });
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [workspaceId, coordinatorId, taskId]);
  const current = result?.taskId === taskId ? result : undefined;
  const sessions = useTaskSessions(current?.task?.id ?? null);
  return { task: current?.task, taskError: current?.error, taskLoading: !current, ...sessions };
}
