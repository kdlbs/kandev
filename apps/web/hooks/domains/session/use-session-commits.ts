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
  // Tracks which sessionId we've already run an authoritative snapshot fetch
  // for. Without this, the initial-fetch gate fires only when `commits` is
  // undefined — but commits can be populated by a live `commit_created`
  // event that arrived before mount, and a replayed/raced event can carry
  // zero stats. The bad stats then stick because the snapshot (which would
  // overwrite with correct stats from `git log --shortstat`) never runs.
  // Anchoring to sessionId guarantees a snapshot per session regardless of
  // how `commits` got populated.
  const fetchedSessionRef = useRef<string | null>(null);
  // Retry timer for the not-ready case — agentctl recovers asynchronously
  // after a backend restart, so the first fetch may land before the workspace
  // execution has been ensured. Without a retry the store would be stuck on
  // an empty list and the COMMITS section would silently miss commits whose
  // commit_created notifications were already fired (or pushed and so
  // filtered out by the live watcher).
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Monotonic request version. Captured at fetch start; the response is only
  // applied if the version still matches. Without this, two trigger bumps in
  // quick succession (e.g. branch_switched → user reverts) could see the
  // older in-flight response land after the newer one and clobber the panel
  // with stale data. Mirrors the pattern in useCumulativeDiff.
  const requestVersionRef = useRef(0);

  // `allowEmpty` is threaded into setSessionCommits's guard. Trigger-bump
  // refetches (commits_reset / branch_switched) can legitimately return [] —
  // e.g. a `git reset` stripped every commit back to base — and the store
  // must accept that authoritatively. Initial fetches keep the default
  // guard so a stale empty response can't race the addSessionCommit path.
  const fetchCommits = useCallback(
    async (opts?: { allowEmpty?: boolean }) => {
      if (!sessionId) return;

      const client = getWebSocketClient();
      if (!client) return;

      if (retryTimerRef.current) {
        clearTimeout(retryTimerRef.current);
        retryTimerRef.current = null;
      }

      const version = ++requestVersionRef.current;
      setSessionCommitsLoading(sessionId, true);
      try {
        const response = await client.request<{ commits?: SessionCommit[]; ready?: boolean }>(
          "session.git.commits",
          { session_id: sessionId },
        );

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
        if (response?.ready === false) {
          retryTimerRef.current = setTimeout(() => {
            retryTimerRef.current = null;
            fetchCommits(opts);
          }, NOT_READY_RETRY_MS);
          return;
        }

        if (response?.commits) {
          setSessionCommits(sessionId, response.commits, opts);
        }
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
    [sessionId, setSessionCommits, setSessionCommitsLoading],
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
    if (connectionStatus !== "connected") return;
    if (!sessionId) return;

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
