import { useEffect } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";

/**
 * Hook to get the current git status for a session.
 * Git status is populated via WebSocket from workspace stream updates.
 * The workspace stream sends current status immediately on subscription.
 */
export function useSessionGitStatus(sessionId: string | null) {
  // Use shallow comparison to prevent re-renders when object reference changes but values are the same
  const gitStatus = useAppStore(
    useShallow((state) => (sessionId ? state.gitStatus.bySessionId[sessionId] : undefined)),
  );
  const connectionStatus = useAppStore((state) => state.connection.status);

  // Subscribe to session updates to receive git status via WebSocket
  // The workspace stream sends current git status immediately on subscription
  useEffect(() => {
    if (!sessionId) return;

    // Wait for WebSocket to be connected before subscribing
    if (connectionStatus !== "connected") return;

    const client = getWebSocketClient();
    if (client) {
      const unsubscribe = client.subscribeSession(sessionId);
      return () => {
        unsubscribe();
        // Don't clear git status on cleanup - keep it cached for when user switches back
      };
    }
  }, [sessionId, connectionStatus]);

  return gitStatus;
}
