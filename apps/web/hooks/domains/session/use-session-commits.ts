import { useEffect, useCallback, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import { fetchSessionCommitsSnapshot, sessionCommitsQueryOptions } from "@/lib/query/query-options";
import type { SessionCommit } from "@/lib/state/slices/session-runtime/types";

// Sentinel ref value: forces the trigger-bumped path to fire on first mount if
// the store already carries a non-zero refetchTrigger (e.g. a bump landed
// before the hook mounted). Any real trigger value the store can hold is > 0,
// so 0 reliably "looks bumped" when the store's first observed value isn't 0
// and "looks unbumped" when it is — there's no first-render flash either way.
const REFETCH_TRIGGER_INIT = 0;

const NOT_READY_RETRY_MS = 2000;

function useSessionCommitStoreState(sessionId: string | null) {
  const envKey = useAppStore((state) =>
    sessionId ? (state.environmentIdBySessionId[sessionId] ?? sessionId) : "",
  );
  const storeCommits = useAppStore((state) => {
    if (!sessionId) return undefined;
    const key = state.environmentIdBySessionId[sessionId] ?? sessionId;
    return state.sessionCommits.byEnvironmentId[key];
  });
  const storeLoading = useAppStore((state) => {
    if (!sessionId) return false;
    const key = state.environmentIdBySessionId[sessionId] ?? sessionId;
    return state.sessionCommits.loading[key] ?? false;
  });
  const refetchTrigger = useAppStore((state) => {
    if (!sessionId) return 0;
    const envKey = state.environmentIdBySessionId[sessionId] ?? sessionId;
    return state.sessionCommits.refetchTrigger[envKey] ?? 0;
  });
  return { envKey, storeCommits, storeLoading, refetchTrigger };
}

/**
 * Hook to fetch and manage commits for a session.
 * Commits are keyed by environmentId so sessions sharing the same environment
 * share the same commit list and don't duplicate fetches.
 */
export function useSessionCommits(sessionId: string | null) {
  const queryClient = useQueryClient();
  const { envKey, storeCommits, storeLoading, refetchTrigger } =
    useSessionCommitStoreState(sessionId);
  const commitsQuery = useQuery({
    ...sessionCommitsQueryOptions(envKey, sessionId ?? ""),
    enabled: false,
  });
  const commits = commitsQuery.data ?? storeCommits;
  const loading = commitsQuery.isFetching || storeLoading;
  const setSessionCommits = useAppStore((state) => state.setSessionCommits);
  const setSessionCommitsLoading = useAppStore((state) => state.setSessionCommitsLoading);
  const connectionStatus = useAppStore((state) => state.connection.status);

  const prevRefetchTriggerRef = useRef<number>(REFETCH_TRIGGER_INIT);
  const fetchedSessionRef = useRef<string | null>(null);
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const requestVersionRef = useRef(0);

  const fetchCommits = useCallback(
    async (opts?: { allowEmpty?: boolean }) => {
      if (!sessionId || !envKey) return;

      if (retryTimerRef.current) {
        clearTimeout(retryTimerRef.current);
        retryTimerRef.current = null;
      }

      const version = ++requestVersionRef.current;
      setSessionCommitsLoading(sessionId, true);
      try {
        const response = await fetchSessionCommitsSnapshot(sessionId);

        // Drop stale callbacks: another fetch (e.g. a later trigger bump)
        // already started, so this response is for an older state and must
        // not overwrite the newer one. Skips both the retry-scheduling and
        // the setSessionCommits write so the in-flight winner stays in
        // control of both.
        if (version !== requestVersionRef.current) return;

        // Backend signals ready:false with an empty commits array when the
        // workspace execution isn't available yet (e.g. agentctl still being
        // recovered after a backend restart, or a session in WAITING_FOR_INPUT
        // whose execution was never spawned). Don't overwrite the store with
        // [] — that would leave commits looking "loaded but empty" forever.
        // Schedule a retry so we eventually pick up the real list. Preserve
        // `opts` across retries so a trigger-bump fetch that gets ready:false
        // still applies its authoritative empty response when it succeeds.
        if (response.ready === false) {
          retryTimerRef.current = setTimeout(() => {
            retryTimerRef.current = null;
            fetchCommits(opts);
          }, NOT_READY_RETRY_MS);
          return;
        }

        const nextCommits = response.commits;
        queryClient.setQueryData(qk.sessionRuntime.commits(envKey), (current: unknown) => {
          const existing = Array.isArray(current)
            ? (current as SessionCommit[])
            : (storeCommits ?? []);
          if (!opts?.allowEmpty && nextCommits.length === 0 && existing.length > 0) {
            return existing;
          }
          return nextCommits;
        });
        setSessionCommits(sessionId, nextCommits, opts);
      } catch (error) {
        console.error("Failed to fetch session commits:", error);
      } finally {
        // Same version guard as above: a stale fetch must not flip loading
        // off — only the current in-flight call owns the loading flag.
        if (version === requestVersionRef.current && !retryTimerRef.current) {
          setSessionCommitsLoading(sessionId, false);
        }
      }
    },
    [envKey, queryClient, sessionId, setSessionCommits, setSessionCommitsLoading, storeCommits],
  );

  // Fetch commits when:
  //  1. The hook mounts (or sessionId changes) — runs an authoritative
  //     snapshot once per session so live `commit_created` events that
  //     populated the store with stale/zero stats get overwritten by the
  //     real `git log --shortstat` data.
  //  2. The refetch trigger was bumped (commits_reset / branch_switched).
  //
  // The trigger path keeps the previous commits in the store while the
  // refetch is in flight (stale-while-revalidate, matching how
  // useCumulativeDiff works).
  useEffect(() => {
    // !sessionId clears the ref BEFORE the connection check so a teardown
    // during a disconnect still resets the gate — otherwise the ref would
    // retain the old id, and a same-id reselect after reconnect would skip
    // the snapshot fetch ("session teardown during disconnect can still
    // block the next snapshot fetch for the same session").
    if (!sessionId) {
      fetchedSessionRef.current = null;
      return;
    }
    if (connectionStatus !== "connected") return;

    const triggerBumped = refetchTrigger !== prevRefetchTriggerRef.current;
    const sessionChanged = fetchedSessionRef.current !== sessionId;

    if (triggerBumped) {
      // Trigger-bump path: the backend already mutated state (reset / branch
      // switch). An empty response is authoritative — bypass the default
      // anti-race guard so the panel reflects the actual post-reset state.
      fetchCommits({ allowEmpty: true });
      fetchedSessionRef.current = sessionId;
    } else if (sessionChanged) {
      fetchCommits();
      fetchedSessionRef.current = sessionId;
    }

    prevRefetchTriggerRef.current = refetchTrigger;
  }, [sessionId, refetchTrigger, fetchCommits, connectionStatus]);

  // Cancel any in-flight retry on unmount, when the session changes, or when
  // the WS disconnects — a retry firing against a disconnected client would
  // just fail the snapshot request and leave loading stuck until the next run.
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
