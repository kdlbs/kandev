import { useEffect, useMemo, useState } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore } from '@/components/state-provider';
import type { TaskSessionState } from '@/lib/types/http';

type UseTaskChatSessionReturn = {
  taskSessionId: string | null;
  taskSessionState: TaskSessionState | null;
  isTaskSessionWorking: boolean;
};

export function useTaskChatSession(taskId: string | null): UseTaskChatSessionReturn {
  const [taskSessionIdsByTaskId, setTaskSessionIdsByTaskId] = useState<Record<string, string>>({});
  const connectionStatus = useAppStore((state) => state.connection.status);
  const taskSessionState = useAppStore((state) =>
    taskId ? state.taskSessionStatesByTaskId?.[taskId] ?? null : null
  );
  const taskSessionId = useMemo(
    () => (taskId ? taskSessionIdsByTaskId[taskId] ?? null : null),
    [taskId, taskSessionIdsByTaskId]
  );

  useEffect(() => {
    if (!taskId) return;
    if (connectionStatus !== 'connected') return;

    const checkExecution = async () => {
      const client = getWebSocketClient();
      if (!client) return;

      try {
        const response = await client.request<{
          has_execution: boolean;
          task_id: string;
          state?: string;
          task_session_id?: string;
        }>('task.execution', { task_id: taskId });

        setTaskSessionIdsByTaskId((prev) => {
          if (!taskId) return prev;
          if (!response.has_execution || !response.task_session_id) {
            if (!prev[taskId]) return prev;
            const next = { ...prev };
            delete next[taskId];
            return next;
          }
          if (prev[taskId] === response.task_session_id) {
            return prev;
          }
          return { ...prev, [taskId]: response.task_session_id };
        });
      } catch (err) {
        console.error('[useTaskChatSession] Failed to check task execution:', err);
      }
    };

    checkExecution();
    const interval = setInterval(() => {
      if (connectionStatus === 'connected') {
        checkExecution();
      }
    }, 2000);

    return () => clearInterval(interval);
  }, [connectionStatus, taskId]);

  const isTaskSessionWorking = taskSessionState === 'STARTING' || taskSessionState === 'RUNNING';

  return {
    taskSessionId,
    taskSessionState,
    isTaskSessionWorking,
  };
}
