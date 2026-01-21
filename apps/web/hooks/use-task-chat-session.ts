import { useEffect, useMemo, useState } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import type { TaskSessionState } from '@/lib/types/http';

type UseTaskChatSessionReturn = {
  taskSessionId: string | null;
  taskSessionState: TaskSessionState | null;
  isTaskSessionWorking: boolean;
};

export function useTaskChatSession(taskId: string | null): UseTaskChatSessionReturn {
  const store = useAppStoreApi();
  const [taskSessionIdsByTaskId, setTaskSessionIdsByTaskId] = useState<Record<string, string>>({});
  const connectionStatus = useAppStore((state) => state.connection.status);
  const taskSessionId = useMemo(
    () => (taskId ? taskSessionIdsByTaskId[taskId] ?? null : null),
    [taskId, taskSessionIdsByTaskId]
  );
  const taskSessionState = useAppStore((state) =>
    taskSessionId ? state.taskSessions.items[taskSessionId]?.state ?? null : null
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
          session_id?: string;
        }>('task.execution', { task_id: taskId });

        setTaskSessionIdsByTaskId((prev) => {
          if (!taskId) return prev;
          if (!response.has_execution || !response.session_id) {
            if (!prev[taskId]) return prev;
            const next = { ...prev };
            delete next[taskId];
            return next;
          }
          if (prev[taskId] === response.session_id) {
            return prev;
          }
          return { ...prev, [taskId]: response.session_id };
        });

        // Store partial session object in the store so useSession can access it
        if (response.has_execution && response.session_id && response.state) {
          store.getState().setTaskSession({
            id: response.session_id,
            task_id: taskId,
            state: response.state as TaskSessionState,
            progress: 0,
            started_at: '',
            updated_at: '',
          });
        }
      } catch {
        // Failed to check task execution
      }
    };

    checkExecution();
    const interval = setInterval(() => {
      if (connectionStatus === 'connected') {
        checkExecution();
      }
    }, 2000);

    return () => clearInterval(interval);
  }, [connectionStatus, taskId, store]);

  const isTaskSessionWorking = taskSessionState === 'STARTING' || taskSessionState === 'RUNNING';

  return {
    taskSessionId,
    taskSessionState,
    isTaskSessionWorking,
  };
}
