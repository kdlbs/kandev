import { useEffect, useCallback, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { SessionCommit } from "@/lib/state/slices/session-runtime/types";

/**
 * Hook to fetch and manage commits for a session.
 * Commits are keyed by environmentId so sessions sharing the same environment
 * share the same commit list and don't duplicate fetches.
 */
export function useSessionCommits(sessionId: string | null) {
  const commits = useAppStore((state) => {
    if (!sessionId) return undefined;
    const envKey = state.environmentIdBySessionId[sessionId] ?? sessionId;
    return state.sessionCommits.byEnvironmentId[envKey];
  });
  const loading = useAppStore((state) => {
    if (!sessionId) return false;
    const envKey = state.environmentIdBySessionId[sessionId] ?? sessionId;
    return state.sessionCommits.loading[envKey] ?? false;
  });
  const setSessionCommits = useAppStore((state) => state.setSessionCommits);
  const setSessionCommitsLoading = useAppStore((state) => state.setSessionCommitsLoading);
  const connectionStatus = useAppStore((state) => state.connection.status);

  // Track whether we had commits before (to detect clears)
  const prevCommitsRef = useRef<SessionCommit[] | undefined>(undefined);

  const fetchCommits = useCallback(async () => {
    if (!sessionId) return;

    const client = getWebSocketClient();
    if (!client) return;

    setSessionCommitsLoading(sessionId, true);
    try {
      const response = await client.request<{ commits?: SessionCommit[] }>("session.git.commits", {
        session_id: sessionId,
      });

      if (response?.commits) {
        setSessionCommits(sessionId, response.commits);
      }
    } catch (error) {
      console.error("Failed to fetch session commits:", error);
    } finally {
      setSessionCommitsLoading(sessionId, false);
    }
  }, [sessionId, setSessionCommits, setSessionCommitsLoading]);

  // Fetch commits on mount or when commits are cleared (e.g., after reset)
  useEffect(() => {
    if (connectionStatus !== "connected") return;
    if (!sessionId) return;

    // Fetch if:
    // 1. commits is undefined (initial load or after clear)
    // 2. commits was previously set but is now undefined (reset scenario)
    const wasCleared = prevCommitsRef.current !== undefined && commits === undefined;
    const needsInitialFetch = commits === undefined;

    if (needsInitialFetch || wasCleared) {
      fetchCommits();
    }

    prevCommitsRef.current = commits;
  }, [sessionId, commits, fetchCommits, connectionStatus]);

  return {
    commits: commits ?? [],
    loading,
    refetch: fetchCommits,
  };
}
