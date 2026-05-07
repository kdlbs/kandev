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
  /** True while a fetch is in flight. Drives the popover progress bar. */
  isFetching: boolean;
  /** Wallclock ms when the cache entry was last updated. */
  lastUpdatedAt: number | null;
  /** Trigger a refetch immediately (used on hover-open and on WS update). */
  refetch: () => void;
};

/**
 * SWR-style hook for the CI popover: keeps a per-PR cache of PRFeedback, and
 * exposes refetch/loading state. The popover refetches on every open and
 * subscribes to `github.task_pr.updated` (already broadcast by the backend
 * poller / mock controller) while open so a check finishing mid-stare still
 * lands.
 *
 * Pass `enabled=false` to fully disable the hook (e.g. when no PR is
 * associated, or on touch devices where the popover is suppressed).
 */
export function usePRCIPopover(pr: TaskPR | null, enabled: boolean): Result {
  const key = pr ? prFeedbackKey(pr) : null;
  const cached = useAppStore((state) => (key ? (state.prFeedbackCache.byKey[key] ?? null) : null));
  const setEntry = useAppStore((state) => state.setPRFeedbackCacheEntry);
  const [isFetching, setIsFetching] = useState(false);
  const requestRef = useRef(0);

  const refetch = useCallback(() => {
    if (!pr || !enabled) return;
    const requestId = ++requestRef.current;
    setIsFetching(true);
    getPRFeedback(pr.owner, pr.repo, pr.pr_number, { cache: "no-store" })
      .then((response) => {
        if (requestRef.current !== requestId) return;
        if (response) setEntry(prFeedbackKey(pr), response);
      })
      .catch(() => {
        // Swallow errors — the popover keeps showing the stale cached value
        // (Q4: stale-while-revalidate). A future refetch may succeed.
      })
      .finally(() => {
        if (requestRef.current === requestId) setIsFetching(false);
      });
  }, [pr, enabled, setEntry]);

  // Refetch every time the popover gains its `enabled` flag (i.e. opens) or
  // when the underlying TaskPR's updated_at changes (the WS handler in
  // lib/ws/handlers/github.ts has already applied the change to the store
  // by the time this fires). queueMicrotask defers the setState (inside
  // refetch's setIsFetching) so the lint's "no setState in effect" rule
  // is satisfied — we're really subscribing to an external system.
  const lastSyncedAt = pr?.updated_at ?? null;
  const lastSyncedRef = useRef<string | null>(null);
  useEffect(() => {
    if (!enabled) return;
    if (lastSyncedRef.current === lastSyncedAt) return;
    lastSyncedRef.current = lastSyncedAt;
    queueMicrotask(refetch);
  }, [enabled, lastSyncedAt, refetch]);

  return {
    feedback: cached?.feedback ?? null,
    isFetching,
    lastUpdatedAt: cached?.lastUpdatedAt ?? null,
    refetch,
  };
}
