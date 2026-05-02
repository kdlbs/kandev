import { useEffect, useCallback, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { SessionCommit } from "@/lib/state/slices/session-runtime/types";

const NOT_READY_RETRY_MS = 2000;

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
  // Retry timer for the not-ready case — agentctl recovers asynchronously
  // after a backend restart, so the first fetch may land before the workspace
  // execution has been ensured. Without a retry the store would be stuck on
  // an empty list and the COMMITS section would silently miss commits whose
  // commit_created notifications were already fired (or pushed and so
  // filtered out by the live watcher).
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const fetchCommits = useCallback(async () => {
    if (!sessionId) return;

    const client = getWebSocketClient();
    if (!client) return;

    if (retryTimerRef.current) {
      clearTimeout(retryTimerRef.current);
      retryTimerRef.current = null;
    }

    setSessionCommitsLoading(sessionId, true);
    try {
      const response = await client.request<{ commits?: SessionCommit[]; ready?: boolean }>(
        "session.git.commits",
        { session_id: sessionId },
      );

      // Backend signals ready:false with an empty commits array when the
      // workspace execution isn't available yet (e.g. agentctl still being
      // recovered after a backend restart, or a session in WAITING_FOR_INPUT
      // whose execution was never spawned). Don't overwrite the store with
      // [] — that would leave commits looking "loaded but empty" forever.
      // Schedule a retry so we eventually pick up the real list.
      if (response?.ready === false) {
        retryTimerRef.current = setTimeout(() => {
          retryTimerRef.current = null;
          fetchCommits();
        }, NOT_READY_RETRY_MS);
        return;
      }

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

  // Cancel any in-flight retry on unmount or when the session changes.
  useEffect(() => {
    return () => {
      if (retryTimerRef.current) {
        clearTimeout(retryTimerRef.current);
        retryTimerRef.current = null;
      }
    };
  }, [sessionId]);

  return {
    commits: commits ?? [],
    loading,
    refetch: fetchCommits,
  };
}
