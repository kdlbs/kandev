import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { useTaskSessionById } from "@/hooks/domains/session/use-task-session-by-id";
import {
  sessionAgentctlQueryOptions,
  type SessionAgentctlStatus,
} from "@/lib/query/query-options/session-runtime";
import { getWebSocketClient } from "@/lib/ws/connection";
import { createDebugLogger } from "@/lib/debug/log";

const debug = createDebugLogger("agentctl:status");

/** Log agentctl status transitions only (re-rendering should not spam). */
function useLogAgentctlTransitions(
  sessionId: string | null,
  status: SessionAgentctlStatus | undefined,
  statusValue: string,
  connectionStatus: string,
): void {
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
  const session = useTaskSessionById(sessionId);
  // Observe-only: the agentctl status is fed into the TQ cache by the
  // session-state bridge (qk.session.agentctl). enabled:false means we never
  // run a queryFn — we just subscribe to whatever the bridge has written.
  const status = useQuery(sessionAgentctlQueryOptions(sessionId ?? "")).data ?? undefined;
  const connectionStatus = useAppStore((state) => state.connection.status);

  useEffect(() => {
    if (!session?.id) return;
    if (connectionStatus !== "connected") return;
    const client = getWebSocketClient();
    if (!client) return;
    return client.subscribeSession(session.id);
  }, [session?.id, connectionStatus]);

  const statusValue = status?.status ?? "missing";
  useLogAgentctlTransitions(sessionId, status, statusValue, connectionStatus);

  return {
    status: status?.status ?? "starting",
    errorMessage: status?.errorMessage,
    agentExecutionId: status?.agentExecutionId,
    isReady: statusValue === "ready",
    isStarting: statusValue === "starting" || !status,
    isError: statusValue === "error",
  };
}
