"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSession } from "@/hooks/domains/session/use-session";
import { useTaskFocus } from "@/hooks/domains/session/use-task-focus";
import type { TaskSession, TaskSessionState } from "@/lib/types/http";

const EMPTY_SESSIONS: TaskSession[] = [];

const TERMINAL_STATES = new Set<TaskSessionState>(["COMPLETED", "FAILED", "CANCELLED"]);

/**
 * Resolves the active ACP session for a task in the orchestrate advanced view.
 * Reads from the global store (populated by WS handlers) and subscribes to
 * real-time updates via useSession + useTaskFocus.
 */
export function useAdvancedSession(taskId: string) {
  const sessionsForTask = useAppStore((state) =>
    taskId ? (state.taskSessionsByTask.itemsByTaskId[taskId] ?? EMPTY_SESSIONS) : EMPTY_SESSIONS,
  );

  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useAppStore((state) =>
    activeSessionId ? (state.taskSessions.items[activeSessionId] ?? null) : null,
  );

  // Prefer the globally active session if it belongs to this task, otherwise
  // pick the newest non-terminal session for the task.
  const resolvedSessionId = useMemo(() => {
    if (activeSession && activeSession.task_id === taskId) {
      return activeSession.id;
    }
    // Walk backwards to find the newest non-terminal session
    for (let i = sessionsForTask.length - 1; i >= 0; i--) {
      if (!TERMINAL_STATES.has(sessionsForTask[i].state)) {
        return sessionsForTask[i].id;
      }
    }
    // Fall back to the newest session even if terminal
    return sessionsForTask[sessionsForTask.length - 1]?.id ?? null;
  }, [activeSession, sessionsForTask, taskId]);

  const { session, isActive } = useSession(resolvedSessionId);
  useTaskFocus(resolvedSessionId);

  const sessionState = session?.state ?? null;
  const isSessionEnded = sessionState !== null && TERMINAL_STATES.has(sessionState);

  return {
    sessionId: resolvedSessionId,
    sessionState,
    hasActiveSession: isActive,
    isSessionEnded,
  };
}
