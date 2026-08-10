"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getPRFeedback } from "@/lib/api/domains/github-api";
import { useAppStore } from "@/components/state-provider";
import { useMinVisibleDuration } from "@/hooks/use-min-visible-duration";
import type { PRFeedback, TaskPR } from "@/lib/types/github";

/**
 * How long the "Updating…" footer stays up once a refresh starts. Long enough
 * to read, short enough that it never feels like the popover is stuck.
 */
export const PR_REFRESH_INDICATOR_MIN_MS = 450;

export function prFeedbackKey(pr: { owner: string; repo: string; pr_number: number }): string {
  return `${pr.owner}/${pr.repo}#${pr.pr_number}`;
}

type Result = {
  /** Last cached PRFeedback (may be stale while a refetch is in flight). */
  feedback: PRFeedback | null;
  /** True while a fetch is in flight. Drives skeleton loading in PRCheckGroup. */
  isFetching: boolean;
  /**
   * True while *any* part of the popover's own refresh cycle is in flight —
   * the PRFeedback fetch and the TaskPR summary sync, which land separately.
   * Held for a minimum duration so it can actually be read. Drives the footer.
   */
  isRefreshing: boolean;
  /** Wallclock ms when the cache entry was last updated. */
  lastUpdatedAt: number | null;
  /** Trigger a refetch immediately (used as a hover-open safety net). */
  refetch: () => void;
};

/**
 * Internal: fetch + cache one PR's feedback. Used by the background-sync hook
 * (always-on, mounted at the top-bar button) and by the popover hook (gated
 * on hover-open). Keeping the fetch logic shared means the request-counter
 * dedup is preserved across both call sites.
 */
function useFeedbackFetch(workspaceId: string | null, pr: TaskPR | null) {
  const setEntry = useAppStore((state) => state.setPRFeedbackCacheEntry);
  const [isFetching, setIsFetching] = useState(false);
  const requestRef = useRef(0);
  const refetch = useCallback(() => {
    if (!workspaceId || !pr) return;
    const requestId = ++requestRef.current;
    setIsFetching(true);
    getPRFeedback(workspaceId, pr.owner, pr.repo, pr.pr_number, { cache: "no-store" })
      .then((response) => {
        if (requestRef.current !== requestId) return;
        if (response) setEntry(prFeedbackKey(pr), response);
      })
      .catch(() => {
        // Swallow errors — the popover keeps showing the stale cached value
        // (stale-while-revalidate). A future refetch may succeed.
      })
      .finally(() => {
        if (requestRef.current === requestId) setIsFetching(false);
      });
  }, [workspaceId, pr, setEntry]);
  return { refetch, isFetching };
}

/**
 * Always-on background sync for the active task's PR. Mounted at the
 * PRTopbarButton so the popover cache stays fresh at the same cadence as
 * the button icon: every time `pr.updated_at` changes (the WS push that
 * already drives the icon color), refetch PRFeedback into the cache.
 *
 * Without this, hover-open had to wait for the on-demand fetch to land
 * before showing fresh data — the user sees a stale popover for ~150ms
 * + network latency.
 */
export function usePRFeedbackBackgroundSync(workspaceId: string | null, pr: TaskPR | null): void {
  const { refetch } = useFeedbackFetch(workspaceId, pr);
  // Compound the cache key with the timestamp so that switching the active
  // task to a different PR (different key) always refetches even when the
  // two PRs happen to share the same updated_at string. Tracking
  // updated_at alone would silently skip the new PR's first fetch.
  const syncKey = pr ? `${prFeedbackKey(pr)}@${pr.updated_at}` : null;
  const lastSyncedRef = useRef<string | null>(null);
  useEffect(() => {
    if (syncKey == null) return;
    if (lastSyncedRef.current === syncKey) return;
    lastSyncedRef.current = syncKey;
    queueMicrotask(refetch);
  }, [syncKey, refetch]);
}

/**
 * Popover-side reader: returns cached feedback, then refreshes both that
 * feedback and the TaskPR summary whenever the popover opens. Refreshing both
 * sources together keeps the trigger's status icon aligned with the checks
 * shown inside the popover.
 */
export function usePRCIPopover(
  workspaceId: string | null,
  pr: TaskPR | null,
  enabled: boolean,
  refreshTaskPR?: () => void | Promise<void>,
): Result {
  const key = pr ? prFeedbackKey(pr) : null;
  const cached = useAppStore((state) => (key ? (state.prFeedbackCache.byKey[key] ?? null) : null));
  const { refetch, isFetching } = useFeedbackFetch(workspaceId, pr);
  const { isSyncing, trackSync } = useTaskPRSyncTracker();
  const isRefreshing = useMinVisibleDuration(isFetching || isSyncing, PR_REFRESH_INDICATOR_MIN_MS);

  const wasEnabledRef = useRef(false);
  useEffect(() => {
    const opened = enabled && !wasEnabledRef.current;
    wasEnabledRef.current = enabled;
    if (!opened) return;
    queueMicrotask(() => {
      refetch();
      trackSync(refreshTaskPR?.());
    });
  }, [enabled, refetch, refreshTaskPR, trackSync]);

  return {
    feedback: cached?.feedback ?? null,
    isFetching,
    isRefreshing,
    lastUpdatedAt: cached?.lastUpdatedAt ?? null,
    refetch,
  };
}

/**
 * Tracks whether the TaskPR summary sync is still in flight. That sync is owned
 * by `useTaskPR` and reaches the popover as an opaque callback, so the only
 * handle on it is the promise it returns — older call sites return `void`, in
 * which case there is nothing to wait for and the tracker stays idle.
 */
function useTaskPRSyncTracker(): {
  isSyncing: boolean;
  trackSync: (result: void | Promise<void>) => void;
} {
  const [isSyncing, setIsSyncing] = useState(false);
  // Only the newest sync may settle the flag: reopening the popover mid-flight
  // starts a second one, and the first landing must not clear the second's.
  const syncRef = useRef(0);
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const trackSync = useCallback((result: void | Promise<void>) => {
    if (!result) return;
    const syncId = ++syncRef.current;
    setIsSyncing(true);
    const settle = () => {
      if (!mountedRef.current || syncRef.current !== syncId) return;
      setIsSyncing(false);
    };
    // `then(settle, settle)` rather than `finally`, which would re-throw a
    // rejection into an unhandled promise. A failed sync is not this hook's
    // to report — it just means the summary stayed stale.
    void result.then(settle, settle);
  }, []);

  return { isSyncing, trackSync };
}
