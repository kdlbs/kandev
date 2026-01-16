import { useMemo } from 'react';
import { useAppStore } from '@/components/state-provider';
import type { TaskSession } from '@/lib/types/http';

type UseTaskSessionReturn = {
  session: TaskSession | null;
  taskId: string | null;
  taskDescription: string | null;
  isWorking: boolean;
};

/**
 * Hook to get task session details and derived task information from a session ID.
 * The session ID is the primary identifier - everything else is derived from it.
 *
 * Falls back to deriving task_id from messages if session isn't in store yet.
 */
export function useTaskSession(sessionId: string | null): UseTaskSessionReturn {
  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] ?? null : null
  );

  // Get task ID from session or fallback to messages
  const messages = useAppStore((state) => state.messages.items);
  const taskIdFromMessages = useMemo(() => {
    if (!sessionId || !messages.length) return null;
    const messageForSession = messages.find((m) => m.task_session_id === sessionId);
    return messageForSession?.task_id ?? null;
  }, [sessionId, messages]);

  const taskId = session?.task_id ?? taskIdFromMessages;

  const task = useAppStore((state) =>
    taskId ? state.kanban.tasks.find((t) => t.id === taskId) ?? null : null
  );

  const taskDescription = task?.description ?? null;

  // Get session state from store by task ID if session object not available
  const sessionStateFromTaskId = useAppStore((state) =>
    taskId ? state.taskSessionStatesByTaskId[taskId] ?? null : null
  );

  const isWorking = useMemo(() => {
    const state = session?.state ?? sessionStateFromTaskId;
    if (!state) return false;
    return state === 'STARTING' || state === 'RUNNING';
  }, [session, sessionStateFromTaskId]);

  return {
    session,
    taskId,
    taskDescription,
    isWorking,
  };
}
