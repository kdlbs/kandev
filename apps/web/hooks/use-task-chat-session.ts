import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { TaskSessionState, TaskSession } from "@/lib/types/http";

const EMPTY_SESSIONS: TaskSession[] = [];

type UseTaskChatSessionReturn = {
  taskSessionId: string | null;
  taskSessionState: TaskSessionState | null;
  isTaskSessionWorking: boolean;
};

export function useTaskChatSession(taskId: string | null): UseTaskChatSessionReturn {
  const sessionsForTask = useAppStore((state) =>
    taskId ? (state.taskSessionsByTask.itemsByTaskId[taskId] ?? EMPTY_SESSIONS) : EMPTY_SESSIONS,
  );

  // Get the first (most recent) session for this task from the store
  const currentSession = useMemo(() => {
    return sessionsForTask[0] ?? null;
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
