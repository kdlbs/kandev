import { useEffect } from 'react';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';

export function useTask(taskId: string | null) {
  const task = useAppStore((state) =>
    taskId ? state.kanban.tasks.find((item) => item.id === taskId) ?? null : null
  );

  useEffect(() => {
    if (!taskId) return;
    const client = getWebSocketClient();
    if (!client) return;
    const unsubscribe = client.subscribe(taskId);
    return () => {
      unsubscribe();
    };
  }, [taskId]);

  return task;
}
