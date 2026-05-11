"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getPRFeedback } from "@/lib/api/domains/github-api";
import { useAppStore } from "@/components/state-provider";
import type { PRFeedback, TaskPR } from "@/lib/types/github";

export function prFeedbackKey(pr: { owner: string; repo: string; pr_number: number }): string {
  return `${pr.owner}/${pr.repo}#${pr.pr_number}`;
}

type Result = {
  /** Last cached PRFeedback (may be stale while a refetch is in flight). */
  feedback: PRFeedback | null;
  /** True while a fetch is in flight. Drives skeleton loading in PRCheckGroup. */
  isFetching: boolean;
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
function useFeedbackFetch(pr: TaskPR | null) {
  const setEntry = useAppStore((state) => state.setPRFeedbackCacheEntry);
  const [isFetching, setIsFetching] = useState(false);
  const requestRef = useRef(0);
  const refetch = useCallback(() => {
    if (!pr) return;
    const requestId = ++requestRef.current;
    setIsFetching(true);
    getPRFeedback(pr.owner, pr.repo, pr.pr_number, { cache: "no-store" })
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
  }, [pr, setEntry]);
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
export function usePRFeedbackBackgroundSync(pr: TaskPR | null): void {
  const { refetch } = useFeedbackFetch(pr);
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
 * Popover-side reader: returns the cached feedback + fires an on-demand
 * refetch whenever the popover transitions from closed to open. The
 * background-sync hook keeps the cache fresh in the meantime, so this
 * mostly serves as a safety net for the very first hover (before any
 * sync has fired).
 */
export function usePRCIPopover(pr: TaskPR | null, enabled: boolean): Result {
  const key = pr ? prFeedbackKey(pr) : null;
  const cached = useAppStore((state) => (key ? (state.prFeedbackCache.byKey[key] ?? null) : null));
  const { refetch, isFetching } = useFeedbackFetch(pr);

  const wasEnabledRef = useRef(false);
  useEffect(() => {
    const opened = enabled && !wasEnabledRef.current;
    wasEnabledRef.current = enabled;
    if (opened) queueMicrotask(refetch);
  }, [enabled, refetch]);

  return {
    feedback: cached?.feedback ?? null,
    isFetching,
    lastUpdatedAt: cached?.lastUpdatedAt ?? null,
    refetch,
  };
}
