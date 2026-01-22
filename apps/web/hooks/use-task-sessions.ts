import { useCallback, useEffect } from 'react';
import { useAppStore } from '@/components/state-provider';
import { listTaskSessions } from '@/lib/api';
import type { TaskSession } from '@/lib/types/http';

const EMPTY_SESSIONS: TaskSession[] = [];

export function useTaskSessions(taskId: string | null) {
  const sessions = useAppStore((state) =>
    taskId ? state.taskSessionsByTask.itemsByTaskId[taskId] ?? EMPTY_SESSIONS : EMPTY_SESSIONS
  );
  const isLoading = useAppStore((state) =>
    taskId ? state.taskSessionsByTask.loadingByTaskId[taskId] ?? false : false
  );
  const isLoaded = useAppStore((state) =>
    taskId ? state.taskSessionsByTask.loadedByTaskId[taskId] ?? false : false
  );
  const setTaskSessionsForTask = useAppStore((state) => state.setTaskSessionsForTask);
  const setTaskSessionsLoading = useAppStore((state) => state.setTaskSessionsLoading);

  const loadSessions = useCallback(async (force = false) => {
    if (!taskId) return;
    if (!force && (isLoading || isLoaded)) return;
    setTaskSessionsLoading(taskId, true);
    try {
      const response = await listTaskSessions(taskId, { cache: 'no-store' });
      setTaskSessionsForTask(taskId, response.sessions ?? []);
    } catch {
      setTaskSessionsForTask(taskId, []);
    } finally {
      setTaskSessionsLoading(taskId, false);
    }
  }, [isLoaded, isLoading, setTaskSessionsForTask, setTaskSessionsLoading, taskId]);

  useEffect(() => {
    if (!taskId) return;
    if (isLoaded || isLoading) return;
    loadSessions();
  }, [isLoaded, isLoading, loadSessions, taskId]);

  return { sessions, isLoading, isLoaded, loadSessions };
}
