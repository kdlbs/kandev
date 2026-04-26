import { useEffect, useMemo } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";

/**
 * Hook to get the current git status for a session.
 * Git status is keyed by environment ID so sessions sharing an environment share git state.
 *
 * For multi-repo workspaces this returns whichever repo's status arrived last;
 * use useSessionGitStatusByRepo when the caller needs all repos at once.
 */
export function useSessionGitStatus(sessionId: string | null) {
  const gitStatus = useAppStore(
    useShallow((state) => {
      if (!sessionId) return undefined;
      const envKey = state.environmentIdBySessionId[sessionId] ?? sessionId;
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

/**
 * Hook to get per-repository git statuses for a multi-repo session.
 * Returns an array of { repository_name, status } sorted by repo name.
 *
 * For single-repo workspaces returns a single-element array (or empty when
 * no status has arrived yet). The Changes panel uses this to merge files
 * from all repos and tag each with its repository, so the file tree's
 * existing per-repo grouping (Phase 6) kicks in automatically.
 */
export function useSessionGitStatusByRepo(
  sessionId: string | null,
): Array<{ repository_name: string; status: GitStatusEntry }> {
  const map = useAppStore(
    useShallow((state) => {
      if (!sessionId) return undefined;
      const envKey = state.environmentIdBySessionId[sessionId] ?? sessionId;
      return state.gitStatus.byEnvironmentRepo[envKey];
    }),
  );
  return useMemo(() => {
    if (!map) return [];
    return Object.entries(map)
      .map(([name, status]) => ({ repository_name: name, status }))
      .sort((a, b) => a.repository_name.localeCompare(b.repository_name));
  }, [map]);
}
