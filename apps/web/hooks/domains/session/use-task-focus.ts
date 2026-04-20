import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";

/**
 * Marks a session as actively focused by the user — the task details page or
 * the task panel modal. The backend uses this signal (separate from
 * subscription) to lift the workspace's git polling into fast mode.
 *
 * Subscriptions remain unchanged: they say "I want updates"; focus says
 * "I'm currently looking at this". Sidebar cards subscribe but never focus,
 * so they get slow polling. The page that actually shows the task focuses,
 * lifting that workspace to fast.
 *
 * Multiple components can call useTaskFocus(sameSessionId) safely — the WS
 * client ref-counts focus calls so only one focus signal is sent backend-side.
 */
export function useTaskFocus(sessionId: string | null | undefined) {
  const connectionStatus = useAppStore((state) => state.connection.status);

  useEffect(() => {
    if (!sessionId) return;
    if (connectionStatus !== "connected") return;

    const client = getWebSocketClient();
    if (!client) return;

    return client.focusSession(sessionId);
  }, [sessionId, connectionStatus]);
}
