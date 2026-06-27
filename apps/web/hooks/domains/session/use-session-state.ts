import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { useSession } from "@/hooks/domains/session/use-session";
import { useTask } from "@/hooks/use-task";
import { prepareProgressQueryOptions } from "@/lib/query/query-options";
import type { TaskSession } from "@/lib/types/http";

function deriveSessionFlags(state: TaskSession["state"] | undefined, errorMessage?: string) {
  const isStarting = state === "STARTING";
  const isAgentBusy = state === "RUNNING";
  const isWorking = isStarting || isAgentBusy;
  const isFailed = state === "FAILED" || state === "CANCELLED";
  const needsRecovery = state === "WAITING_FOR_INPUT" && !!errorMessage;
  return { isStarting, isWorking, isAgentBusy, isFailed, needsRecovery };
}

type UseSessionStateOptions = {
  taskIdHint?: string | null;
};

function useValidatedActiveSessionId() {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSessionData = useAppStore((state) =>
    activeSessionId ? (state.taskSessions.items[activeSessionId] ?? null) : null,
  );

  return activeSessionData && activeSessionData.task_id === activeTaskId ? activeSessionId : null;
}

function usePrepareStatus(resolvedSessionId: string | null) {
  const prepareQuery = useQuery(prepareProgressQueryOptions(resolvedSessionId ?? ""));
  const storePrepareStatus = useAppStore((state) =>
    resolvedSessionId ? state.prepareProgress.bySessionId[resolvedSessionId]?.status : undefined,
  );
  return prepareQuery.data?.status ?? storePrepareStatus;
}

export function useSessionState(sessionId: string | null, options: UseSessionStateOptions = {}) {
  const { taskIdHint = null } = options;
  const validatedActiveSessionId = useValidatedActiveSessionId();
  const resolvedSessionId = sessionId ?? validatedActiveSessionId;

  const { session } = useSession(resolvedSessionId);
  const taskId = session?.task_id ?? taskIdHint ?? null;
  const task = useTask(taskId);
  const prepareStatus = usePrepareStatus(resolvedSessionId);

  const taskDescription = task?.description ?? null;
  const flags = deriveSessionFlags(session?.state, session?.error_message);
  const isPreparingEnvironment = prepareStatus === "preparing";

  return {
    resolvedSessionId,
    session,
    task,
    taskId,
    taskDescription,
    ...flags,
    isStarting: flags.isStarting || isPreparingEnvironment,
    isWorking: flags.isWorking || isPreparingEnvironment,
    // Exposed separately so consumers that gate on "is the executor still
    // bootstrapping" (e.g. the chat input "agent is still being set up"
    // tooltip) can distinguish a brief STARTING transition — which every
    // session passes through, including local quick-chat — from an active
    // Docker / Sprites prepare-environment phase.
    isPreparingEnvironment,
  };
}
