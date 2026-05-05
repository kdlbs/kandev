import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";

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

  const isReady = status?.status === "ready";

  return {
    status: status?.status ?? "starting",
    errorMessage: status?.errorMessage,
    agentExecutionId: status?.agentExecutionId,
    isReady,
    isStarting: status?.status === "starting" || !status,
    isError: status?.status === "error",
  };
}
