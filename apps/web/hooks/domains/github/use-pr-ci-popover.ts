"use client";

import { useEffect, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { githubQueryOptions } from "@/lib/query/query-options/github";
import { qk } from "@/lib/query/keys";
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
  const qc = useQueryClient();
  // Compound the cache key with the timestamp so that switching the active
  // task to a different PR (different key) always refetches even when the
  // two PRs happen to share the same updated_at string.
  const syncKey = pr ? `${prFeedbackKey(pr)}@${pr.updated_at}` : null;
  const lastSyncedRef = useRef<string | null>(null);

  useEffect(() => {
    if (syncKey == null || !pr) return;
    if (lastSyncedRef.current === syncKey) return;
    lastSyncedRef.current = syncKey;
    queueMicrotask(() => {
      void qc.refetchQueries({
        queryKey: qk.github.prFeedback(pr.owner, pr.repo, pr.pr_number),
      });
    });
  }, [syncKey, pr, qc]);
}

/**
 * Popover-side reader: returns the cached feedback + fires an on-demand
 * refetch whenever the popover transitions from closed to open. The
 * background-sync hook keeps the cache fresh in the meantime, so this
 * mostly serves as a safety net for the very first hover (before any
 * sync has fired).
 */
export function usePRCIPopover(pr: TaskPR | null, enabled: boolean): Result {
  const qc = useQueryClient();
  const {
    data: feedback,
    isFetching,
    dataUpdatedAt,
  } = useQuery({
    ...githubQueryOptions.prFeedback(pr?.owner ?? null, pr?.repo ?? null, pr?.pr_number ?? null),
    // Never auto-refetch; callers drive freshness.
    refetchOnWindowFocus: false,
    refetchOnMount: false,
  });

  const wasEnabledRef = useRef(false);
  useEffect(() => {
    const opened = enabled && !wasEnabledRef.current;
    wasEnabledRef.current = enabled;
    if (opened && pr) {
      queueMicrotask(() => {
        void qc.refetchQueries({
          queryKey: qk.github.prFeedback(pr.owner, pr.repo, pr.pr_number),
        });
      });
    }
  }, [enabled, pr, qc]);

  function refetch() {
    if (!pr) return;
    void qc.refetchQueries({
      queryKey: qk.github.prFeedback(pr.owner, pr.repo, pr.pr_number),
    });
  }

  return {
    feedback: feedback ?? null,
    isFetching,
    lastUpdatedAt: dataUpdatedAt > 0 ? dataUpdatedAt : null,
    refetch,
  };
}
