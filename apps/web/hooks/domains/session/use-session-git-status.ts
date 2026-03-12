import { useEffect } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";

/** Resolve the environment key for workspace state (git, files) given a sessionId. */
function resolveEnvKey(
  state: { environmentIdBySessionId: Record<string, string>; taskSessions: { items: Record<string, { task_environment_id?: string }> } },
  sessionId: string,
): string {
  return state.environmentIdBySessionId[sessionId]
    ?? state.taskSessions.items[sessionId]?.task_environment_id
    ?? sessionId;
}

/**
 * Hook to get the current git status for a session.
 * Git status is keyed by environment ID so sessions sharing an environment share git state.
 */
export function useSessionGitStatus(sessionId: string | null) {
  const gitStatus = useAppStore(
    useShallow((state) => {
      if (!sessionId) return undefined;
      const envKey = resolveEnvKey(state, sessionId);
      return state.gitStatus.byEnvironmentId[envKey];
    }),
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
