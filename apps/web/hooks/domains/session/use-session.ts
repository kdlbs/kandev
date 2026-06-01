import { useEffect, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { useTaskSessionById } from "@/hooks/domains/session/use-task-session-by-id";
import { sessionAgentctlQueryOptions } from "@/lib/query/query-options/session-runtime";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { TaskSession } from "@/lib/types/http";

type UseSessionResult = {
  session: TaskSession | null;
  isActive: boolean;
  isFailed: boolean;
  errorMessage: string | undefined;
};

export function useSession(sessionId: string | null): UseSessionResult {
  const session = useTaskSessionById(sessionId);
  const connectionStatus = useAppStore((state) => state.connection.status);
  const agentctlReady =
    useQuery(sessionAgentctlQueryOptions(sessionId ?? "")).data?.status === "ready";

  const isActive = useMemo(() => {
    if (!session?.state) return false;
    if (session.state === "RUNNING" || session.state === "WAITING_FOR_INPUT") return true;
    // Workspace infrastructure (agentctl) is ready even though the agent CLI hasn't started
    if (session.state === "CREATED" && agentctlReady) return true;
    return false;
  }, [session?.state, agentctlReady]);

  const isFailed = useMemo(() => {
    return session?.state === "FAILED";
  }, [session?.state]);

  useEffect(() => {
    if (connectionStatus !== "connected") return;
    if (!session?.id) return;
    const client = getWebSocketClient();
    if (!client) return;
    const unsubscribe = client.subscribeSession(session.id);
    return () => {
      unsubscribe();
    };
  }, [session?.id, connectionStatus]);

  return { session, isActive, isFailed, errorMessage: session?.error_message };
}
