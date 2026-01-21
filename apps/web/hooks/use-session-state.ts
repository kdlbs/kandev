import { useAppStore } from '@/components/state-provider';
import { useSession } from '@/hooks/use-session';
import { useTask } from '@/hooks/use-task';

export function useSessionState(sessionId: string | null) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const resolvedSessionId = sessionId ?? activeSessionId;

  const { session } = useSession(resolvedSessionId);
  const task = useTask(session?.task_id ?? null);

  const taskId = session?.task_id ?? null;
  const taskDescription = task?.description ?? null;
  const isStarting = session?.state === 'STARTING';
  const isWorking = isStarting || session?.state === 'RUNNING';
  const isAgentBusy = session?.state === 'CREATED' || session?.state === 'RUNNING';

  return {
    resolvedSessionId,
    session,
    task,
    taskId,
    taskDescription,
    isStarting,
    isWorking,
    isAgentBusy,
  };
}
