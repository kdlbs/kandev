"use client";

import { useAppStore } from "@/components/state-provider";
import { useExecutionClock } from "@/hooks/use-execution-clock";
import { getExecutionStatus } from "@/lib/execution-status";
import { getExecutionFacts, executionObservationDeadline } from "@/lib/execution-facts";
import type { ExecutionFacts, ExecutionStatus } from "@/lib/types/execution";
import type { TaskSession } from "@/lib/types/http";

export function useSessionExecution(sessionId: string | null | undefined): {
  session: TaskSession | undefined;
  status: ExecutionStatus;
  facts: ExecutionFacts | null;
} {
  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] : undefined,
  );
  const connection = useAppStore((state) => state.connection.status);
  const deadline = connection === "connected" ? executionObservationDeadline(session) : null;
  const now = Math.max(useExecutionClock(deadline), Date.now());
  return {
    session,
    facts: session ? getExecutionFacts(session, connection, now) : null,
    status: session ? getExecutionStatus(session, connection, now) : "unknown",
  };
}
