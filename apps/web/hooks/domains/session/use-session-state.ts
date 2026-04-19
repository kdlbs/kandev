import { useAppStore } from "@/components/state-provider";
import { useSession } from "@/hooks/domains/session/use-session";
import { useTask } from "@/hooks/use-task";
import type { TaskSession } from "@/lib/types/http";

function deriveSessionFlags(
  state: TaskSession["state"] | undefined,
  errorMessage?: string,
  hasTurns?: boolean,
) {
  // A session is "starting" if it hasn't completed any turns yet and is in
  // a pre-ready state. This avoids flicker when the session transitions
  // through CREATED → WAITING_FOR_INPUT → STARTING → RUNNING during launch.
  const isStarting =
    state === "STARTING" ||
    state === "CREATED" ||
    (state === "WAITING_FOR_INPUT" && !hasTurns && !errorMessage);
  const isAgentBusy = state === "RUNNING";
  const isWorking = isStarting || isAgentBusy;
  const isFailed = state === "FAILED" || state === "CANCELLED";
  const needsRecovery = state === "WAITING_FOR_INPUT" && !!errorMessage;
  return { isStarting, isWorking, isAgentBusy, isFailed, needsRecovery };
}

export function useSessionState(sessionId: string | null) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);

  // Validate that active session belongs to the active task before using it.
  // This prevents showing messages from an unrelated session when navigating
  // to a task that has no sessions yet (activeSessionId may still hold the
  // old session from the previous task).
  const activeSessionData = useAppStore((state) =>
    activeSessionId ? (state.taskSessions.items[activeSessionId] ?? null) : null,
  );
  const validatedActiveSessionId =
    activeSessionData && activeSessionData.task_id === activeTaskId ? activeSessionId : null;

  const resolvedSessionId = sessionId ?? validatedActiveSessionId;

  const { session } = useSession(resolvedSessionId);
  const task = useTask(session?.task_id ?? null);
  const turns = useAppStore((state) =>
    resolvedSessionId ? state.turns.bySession[resolvedSessionId] : undefined,
  );

  const taskId = session?.task_id ?? null;
  const taskDescription = task?.description ?? null;
  const hasTurns = Boolean(turns && turns.length > 0);
  const flags = deriveSessionFlags(session?.state, session?.error_message, hasTurns);

  return {
    resolvedSessionId,
    session,
    task,
    taskId,
    taskDescription,
    ...flags,
  };
}
