import { useEffect, useCallback, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import type { CumulativeDiff } from "@/lib/state/slices/session-runtime/types";

const debug = createDebugLogger("review:cumulative");

const cumulativeDiffCache: Record<string, CumulativeDiff | null> = {};
const loadingState: Record<string, boolean> = {};
// Invalidations that arrived while a fetch was in flight. The in-flight
// request can't be guaranteed to capture the working-tree state it was
// triggered by (git diff runs once on the server, edits may land after),
// so we always run one follow-up fetch after the in-flight one settles.
const pendingInvalidationByEnvKey: Record<string, boolean> = {};

// Listener events: "invalidated" triggers a refetch (bumps invalidationCount);
// "populated" tells subscribers to pick up the fresh cache value WITHOUT
// triggering another fetch. Without the populated signal, only the subscriber
// that "wins" the fetch race calls setDiff — the others stay stale because
// useState only reads the module cache at initial mount.
type ListenerEvent = { envKey: string; kind: "invalidated" | "populated" };
const listeners = new Set<(event: ListenerEvent) => void>();

// Trailing-edge debounce for the refetch-triggering listener fanout. A large
// rebase fires a burst of git WS events (one status_update per ~2 s poll tick,
// one commit_created per rebased commit, plus commits_reset/branch_switched),
// each of which would otherwise kick a fresh multi-MB cumulative-diff fetch.
// Coalescing the fanout collapses an N-event burst into ~1 trailing refetch.
// Per-envKey so unrelated environments don't block each other.
const COALESCE_WINDOW_MS = 200;
const invalidationTimers: Record<string, ReturnType<typeof setTimeout>> = {};

function clearInvalidationTimer(envKey: string) {
  const timer = invalidationTimers[envKey];
  if (timer) {
    clearTimeout(timer);
    delete invalidationTimers[envKey];
  }
}

/**
 * Invalidate the cumulative diff cache for the given environment key.
 * Callers should resolve sessionId → envKey before calling this.
 */
export function invalidateCumulativeDiffCache(envKey: string) {
  // Cache deletion stays SYNCHRONOUS: components reading
  // `cumulativeDiffCache[envKey]` on render (e.g. the useState initializer)
  // must observe the miss immediately. Only the listener fanout — which is
  // what actually triggers the refetch — is debounced.
  delete cumulativeDiffCache[envKey];
  // If a fetch is in flight, mark a pending refetch — the in-flight git diff
  // may have run before the worktree edit that triggered this invalidation,
  // so we need to refetch once it settles. Drained in fetchCumulativeDiff's
  // finally block. Only `invalidateCumulativeDiffCache` sets this flag (not
  // the in-flight skip path in the fetch itself), so duplicate React
  // subscribers don't create phantom invalidations that loop forever.
  if (loadingState[envKey]) {
    pendingInvalidationByEnvKey[envKey] = true;
  }
  debug("cache.invalidated", { envKey });
  // Coalesce the refetch trigger across a burst.
  clearInvalidationTimer(envKey);
  invalidationTimers[envKey] = setTimeout(() => {
    delete invalidationTimers[envKey];
    debug("cache.invalidated.coalesced", { envKey });
    listeners.forEach((fn) => fn({ envKey, kind: "invalidated" }));
  }, COALESCE_WINDOW_MS);
}

function commitFetchedDiff(
  envKey: string,
  sessionId: string,
  diff: CumulativeDiff | null,
  setDiff: (d: CumulativeDiff | null) => void,
) {
  cumulativeDiffCache[envKey] = diff;
  setDiff(diff);
  if (isDebug()) {
    debug("fetch.success", {
      sessionId,
      envKey,
      fileCount: diff ? Object.keys(diff.files ?? {}).length : 0,
      empty: !diff,
    });
  }
  // Broadcast the populated cache to other subscribers so they pick up the
  // fresh value. Without this, only the subscriber that "won" the fetch race
  // calls setDiff — others stay stale on the value they had at mount time.
  listeners.forEach((fn) => fn({ envKey, kind: "populated" }));
}

export function useCumulativeDiff(sessionId: string | null) {
  // Resolve to environment key so sessions sharing the same environment share the cache.
  const envKey = useAppStore((state) => {
    if (!sessionId) return null;
    return state.environmentIdBySessionId[sessionId] ?? sessionId;
  });

  // Guard against stale responses after an environment switch.
  const requestVersionRef = useRef(0);

  const [diff, setDiff] = useState<CumulativeDiff | null>(
    envKey ? (cumulativeDiffCache[envKey] ?? null) : null,
  );
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [invalidationCount, setInvalidationCount] = useState(0);

  const fetchCumulativeDiff = useCallback(async () => {
    if (!sessionId || !envKey) return;
    if (loadingState[envKey]) {
      // Another subscriber already kicked off the fetch. Do NOT set the
      // pending flag here — only `invalidateCumulativeDiffCache` does that,
      // so we don't loop on duplicate subscriber calls. When the in-flight
      // fetch completes it broadcasts a "populated" event that updates every
      // subscriber from the shared cache.
      debug("fetch.skip.in-flight", { sessionId, envKey });
      return;
    }

    const client = getWebSocketClient();
    if (!client) return;

    const version = ++requestVersionRef.current;

    setLoading(true);
    loadingState[envKey] = true;
    setError(null);
    debug("fetch.start", { sessionId, envKey });

    try {
      // Backend routes by session_id, but we cache by envKey
      const response = await client.request<{ cumulative_diff?: CumulativeDiff }>(
        "session.cumulative_diff",
        { session_id: sessionId },
      );

      // Discard if the environment changed while the request was in flight
      if (version !== requestVersionRef.current) return;

      commitFetchedDiff(envKey, sessionId, response?.cumulative_diff ?? null, setDiff);
    } catch (err) {
      if (version !== requestVersionRef.current) return;
      console.error("Failed to fetch cumulative diff:", err);
      const message = err instanceof Error ? err.message : "Failed to fetch cumulative diff";
      setError(message);
      debug("fetch.error", { sessionId, envKey, error: message });
    } finally {
      if (version === requestVersionRef.current) {
        setLoading(false);
      }
      loadingState[envKey] = false;
      // Drain any invalidation that arrived mid-flight by re-notifying
      // listeners, which bumps `invalidationCount` and re-runs the fetch
      // effect. Cleared first so the follow-up fetch can record its own
      // pending state if needed.
      if (pendingInvalidationByEnvKey[envKey]) {
        delete pendingInvalidationByEnvKey[envKey];
        debug("fetch.drain.pending", { sessionId, envKey });
        listeners.forEach((fn) => fn({ envKey, kind: "invalidated" }));
      }
    }
  }, [sessionId, envKey]);

  // Sync cached state when envKey changes.  Must run BEFORE the fetch effect
  // so that fetchCumulativeDiff's setLoading(true) wins the React 18 batch.
  useEffect(() => {
    // Bump version so any in-flight fetch for the previous envKey is discarded.
    // Clear the per-key loading flag so the fetch effect isn't blocked on re-entry
    // (e.g. A→B→A where A's original fetch is still in-flight).
    requestVersionRef.current++;
    if (envKey) {
      loadingState[envKey] = false;
      delete pendingInvalidationByEnvKey[envKey];
      setDiff(cumulativeDiffCache[envKey] ?? null);
    } else {
      setDiff(null);
    }
    setLoading(false);
    // Drop any pending coalesced refetch for this envKey on unmount / env
    // change, so navigating away doesn't fire a stale fetch.
    return () => {
      if (envKey) clearInvalidationTimer(envKey);
    };
  }, [envKey]);

  // Fetch on mount and when cache is invalidated
  useEffect(() => {
    if (!envKey) return;
    fetchCumulativeDiff();
  }, [envKey, invalidationCount, fetchCumulativeDiff]);

  // Subscribe to cache events from WS handlers and from other subscribers'
  // successful fetches.
  useEffect(() => {
    if (!envKey) return;
    const handler = (event: ListenerEvent) => {
      if (event.envKey !== envKey) return;
      if (event.kind === "populated") {
        // Pick up the freshly-cached value without triggering a refetch.
        setDiff(cumulativeDiffCache[envKey] ?? null);
      } else {
        setInvalidationCount((c) => c + 1);
      }
    };
    listeners.add(handler);
    return () => {
      listeners.delete(handler);
    };
  }, [envKey]);

  return {
    diff,
    loading,
    error,
    refetch: fetchCumulativeDiff,
  };
}
