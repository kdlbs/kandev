import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { sessionAgentctlQueryOptions } from "@/lib/query/query-options";
import { getWebSocketClient } from "@/lib/ws/connection";
import { createDebugLogger } from "@/lib/debug/log";
import type { SessionAgentctlStatus } from "@/lib/state/slices/session/types";
import type { TaskSession, TaskSessionState } from "@/lib/types/http";

const debug = createDebugLogger("agentctl:status");

type AgentctlStatusLog = {
  errorMessage?: string | null;
  agentExecutionId?: string | null;
};

const WORKSPACE_READY_STATES: ReadonlySet<TaskSessionState> = new Set([
  "RUNNING",
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "CANCELLED",
]);

function resolveAgentctlStatusValue(
  status: SessionAgentctlStatus | null | undefined,
  session: TaskSession | null | undefined,
) {
  if (status?.status === "error" || status?.status === "ready") return status.status;
  if (session?.task_environment_id && WORKSPACE_READY_STATES.has(session.state)) return "ready";
  return status?.status ?? "missing";
}

function useAgentctlTransitionLog(
  sessionId: string | null,
  statusValue: string,
  status: AgentctlStatusLog | undefined,
  connectionStatus: string,
) {
  const lastLoggedRef = useRef<string | null>(null);
  const snapshot = `${sessionId ?? "none"}|${statusValue}|${status?.errorMessage ?? ""}|${status?.agentExecutionId ?? ""}`;
  useEffect(() => {
    if (!sessionId) return;
    if (lastLoggedRef.current === snapshot) return;
    debug("transition", {
      sessionId,
      from: lastLoggedRef.current ?? "init",
      status: statusValue,
      errorMessage: status?.errorMessage ?? null,
      agentExecutionId: status?.agentExecutionId ?? null,
      connectionStatus,
    });
    lastLoggedRef.current = snapshot;
  }, [sessionId, snapshot, statusValue, status, connectionStatus]);
}

export function useSessionAgentctl(sessionId: string | null) {
  const agentctlQuery = useQuery(sessionAgentctlQueryOptions(sessionId ?? ""));
  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] : undefined,
  );
  const storeStatus = useAppStore((state) =>
    sessionId ? state.sessionAgentctl.itemsBySessionId[sessionId] : undefined,
  );
  const status = agentctlQuery.data ?? storeStatus;
  const connectionStatus = useAppStore((state) => state.connection.status);

  useEffect(() => {
    if (!session?.id) return;
    if (connectionStatus !== "connected") return;
    const client = getWebSocketClient();
    if (!client) return;
    return client.subscribeSession(session.id);
  }, [session?.id, connectionStatus]);

  // Log status transitions only — re-rendering should not spam.
  const statusValue = resolveAgentctlStatusValue(status, session);
  useAgentctlTransitionLog(sessionId, statusValue, status, connectionStatus);

  return {
    status: statusValue === "missing" ? "starting" : statusValue,
    errorMessage: status?.errorMessage,
    agentExecutionId: status?.agentExecutionId,
    isReady: statusValue === "ready",
    isStarting: statusValue === "starting" || statusValue === "missing",
    isError: statusValue === "error",
  };
}
