import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { createDebugLogger } from "@/lib/debug/log";

const debug = createDebugLogger("agentctl:status");

export function useSessionAgentctl(sessionId: string | null) {
  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] : undefined,
  );
  const status = useAppStore((state) =>
    sessionId ? state.sessionAgentctl.itemsBySessionId[sessionId] : undefined,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);

  useEffect(() => {
    if (!session?.id) return;
    if (connectionStatus !== "connected") return;
    const client = getWebSocketClient();
    if (!client) return;
    return client.subscribeSession(session.id);
  }, [session?.id, connectionStatus]);

  // Log status transitions only — re-rendering should not spam.
  const lastLoggedRef = useRef<string | null>(null);
  const statusValue = status?.status ?? "missing";
  const snapshot = `${statusValue}|${status?.errorMessage ?? ""}|${status?.agentExecutionId ?? ""}`;
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

  return {
    status: status?.status ?? "starting",
    errorMessage: status?.errorMessage,
    agentExecutionId: status?.agentExecutionId,
    isReady: statusValue === "ready",
    isStarting: statusValue === "starting" || !status,
    isError: statusValue === "error",
  };
}
