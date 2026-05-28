import { useEffect, useCallback, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { SessionCommit } from "@/lib/state/slices/session-runtime/types";

// Sentinel ref value: forces the trigger-bumped path to fire on first mount if
// the store already carries a non-zero refetchTrigger (e.g. a bump landed
// before the hook mounted). Any real trigger value the store can hold is > 0,
// so 0 reliably "looks bumped" when the store's first observed value isn't 0
// and "looks unbumped" when it is — there's no first-render flash either way.
const REFETCH_TRIGGER_INIT = 0;

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
  // Stale-while-revalidate trigger: bumped by commits_reset / branch_switched
  // WS events. We refetch on change without nulling the visible list, so the
  // Changes panel keeps showing the previous commits until the new ones land.
  const refetchTrigger = useAppStore((state) => {
    if (!sessionId) return 0;
    const envKey = state.environmentIdBySessionId[sessionId] ?? sessionId;
    return state.sessionCommits.refetchTrigger[envKey] ?? 0;
  });
  const setSessionCommits = useAppStore((state) => state.setSessionCommits);
  const setSessionCommitsLoading = useAppStore((state) => state.setSessionCommitsLoading);
  const connectionStatus = useAppStore((state) => state.connection.status);

  // Track the last refetch trigger we acted on, so a bump triggers exactly one
  // refetch rather than re-firing on every render. Initialise to a sentinel
  // (not `refetchTrigger`) so a bump that arrived before this hook mounted
  // still drives an initial refetch — otherwise prevRef would equal the live
  // value and `triggerBumped` would silently be false on first render.
  const prevRefetchTriggerRef = useRef<number>(REFETCH_TRIGGER_INIT);
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
      // Keep loading:true while a retry is scheduled — the operation isn't
      // really finished, and flipping the flag here would let consumers see
      // `{ loading: false, commits: [] }` for the whole retry window, which is
      // the same misleading state this hook is designed to avoid.
      if (!retryTimerRef.current) {
        setSessionCommitsLoading(sessionId, false);
      }
    }
  }, [sessionId, setSessionCommits, setSessionCommitsLoading]);

  // Fetch commits when:
  //  1. commits is undefined (initial load — clearSessionCommits is still
  //     called by callers that genuinely want to drop the list, e.g. session
  //     teardown).
  //  2. the refetch trigger was bumped (commits_reset / branch_switched).
  //
  // The trigger path replaces the old "clear then refetch from undefined"
  // pattern: it keeps the previous commits in the store so the Changes panel
  // doesn't flicker through its empty state while the refetch is in flight
  // (stale-while-revalidate, matching how useCumulativeDiff works).
  useEffect(() => {
    if (connectionStatus !== "connected") return;
    if (!sessionId) return;

    const needsInitialFetch = commits === undefined;
    const triggerBumped = refetchTrigger !== prevRefetchTriggerRef.current;

    if (needsInitialFetch || triggerBumped) {
      fetchCommits();
    }

    prevRefetchTriggerRef.current = refetchTrigger;
  }, [sessionId, commits, refetchTrigger, fetchCommits, connectionStatus]);

  // Cancel any in-flight retry on unmount, when the session changes, or when
  // the WS disconnects — a retry firing against a disconnected client would
  // either throw inside fetchCommits or hit getWebSocketClient()===null.
  useEffect(() => {
    return () => {
      if (retryTimerRef.current) {
        clearTimeout(retryTimerRef.current);
        retryTimerRef.current = null;
      }
    };
  }, [sessionId, connectionStatus]);

  return {
    commits: commits ?? [],
    loading,
    refetch: fetchCommits,
  };
}
