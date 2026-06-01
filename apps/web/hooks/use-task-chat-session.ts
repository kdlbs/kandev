import { useMemo } from "react";
import { useTaskSessionsByTask } from "@/hooks/domains/session/use-task-session-by-id";
import type { TaskSessionState } from "@/lib/types/http";

type UseTaskChatSessionReturn = {
  taskSessionId: string | null;
  taskSessionState: TaskSessionState | null;
  isTaskSessionWorking: boolean;
};

export function useTaskChatSession(taskId: string | null): UseTaskChatSessionReturn {
  const { sessions: sessionsForTask } = useTaskSessionsByTask(taskId);

  // Prefer the primary session, fall back to the first (most recent)
  const currentSession = useMemo(() => {
    if (sessionsForTask.length === 0) return null;
    return sessionsForTask.find((s) => s.is_primary) ?? sessionsForTask[0] ?? null;
  }, [sessionsForTask]);

  const taskSessionId = currentSession?.id ?? null;
  const taskSessionState = currentSession?.state ?? null;
  const isTaskSessionWorking = taskSessionState === "STARTING" || taskSessionState === "RUNNING";

  return {
    taskSessionId,
    taskSessionState,
    isTaskSessionWorking,
  };
}
