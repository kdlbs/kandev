"use client";

import { useAppStore } from "@/components/state-provider";
import { useExecutionClock } from "@/hooks/use-execution-clock";
import { getExecutionStatus } from "@/lib/execution-status";
import type { ExecutionStatus } from "@/lib/types/execution";
import type { TaskSession } from "@/lib/types/http";

export function useSessionExecution(sessionId: string | null | undefined): {
  session: TaskSession | undefined;
  status: ExecutionStatus;
} {
  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] : undefined,
  );
  const connection = useAppStore((state) => state.connection.status);
  const observedStatus = session?.execution_observation?.status;
  const live =
    observedStatus === "active" || observedStatus === "starting" || observedStatus === "configured";
  const observation = session?.execution_observation;
  const checkedDeadline = Date.parse(observation?.checked_at ?? "") + 30_000;
  const deadline =
    observedStatus === "active"
      ? Math.min(checkedDeadline, Date.parse(observation?.valid_until ?? ""))
      : checkedDeadline;
  const now = useExecutionClock(connection === "connected" && live ? deadline : null);
  return {
    session,
    status: session ? getExecutionStatus(session, connection, now || Date.now()) : "unknown",
  };
}
