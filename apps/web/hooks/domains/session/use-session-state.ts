import { useAppStore } from "@/components/state-provider";
import { useSession } from "@/hooks/domains/session/use-session";
import { useTask } from "@/hooks/use-task";
import type { TaskSession } from "@/lib/types/http";

function deriveSessionFlags(state: TaskSession["state"] | undefined, errorMessage?: string) {
  const isStarting = state === "STARTING";
  const isAgentBusy = state === "RUNNING";
  const isWorking = isStarting || isAgentBusy;
  const isFailed = state === "FAILED" || state === "CANCELLED";
  const needsRecovery = state === "WAITING_FOR_INPUT" && !!errorMessage;
  return { isStarting, isWorking, isAgentBusy, isFailed, needsRecovery };
}

export function useSessionState(sessionId: string | null) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const resolvedSessionId = sessionId ?? activeSessionId;

  const { session } = useSession(resolvedSessionId);
  const task = useTask(session?.task_id ?? null);

  const taskId = session?.task_id ?? null;
  const taskDescription = task?.description ?? null;
  const flags = deriveSessionFlags(session?.state, session?.error_message);

  return {
    resolvedSessionId,
    session,
    task,
    taskId,
    taskDescription,
    ...flags,
  };
}
