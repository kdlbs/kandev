"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { PRDiffFile, TaskPR } from "@/lib/types/github";

type PRFilesByKey = Record<string, PRDiffFile[]>;

// Stable empty array so the Zustand selector returns the same reference
// for tasks with zero PRs. A fresh `[]` per render would re-trigger the
// selector subscriber and cascade through every effect that depends on
// `prs`.
const EMPTY_PRS: TaskPR[] = [];

/**
 * Cache key for an in-flight fetch — owner/repo/PR + the last_synced_at hint
 * from the TaskPR row, so a server-side sync invalidates the cache and
 * triggers a refetch automatically.
 */
function fetchKey(pr: TaskPR): string {
  return `${pr.owner}/${pr.repo}/${pr.pr_number}/${pr.last_synced_at ?? ""}`;
}

/**
 * Returns one diff array per task PR, keyed by `${owner}/${repo}/${prNumber}/${last_synced_at}`.
 * Internally fans out one WS request per PR and tracks them in local state —
 * we can't use `usePRDiff` directly because hooks can't be called in a loop.
 *
 * Designed for the changes panel's PR Changes section, which needs to render
 * one row per file across every per-repo PR (multi-repo tasks now have one
 * PR per repo, not just one for the whole task).
 */
export function useActiveTaskPRsWithFiles(): {
  prs: TaskPR[];
  filesByPRKey: PRFilesByKey;
} {
  const prs = useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return EMPTY_PRS;
    return s.taskPRs.byTaskId[taskId] ?? EMPTY_PRS;
  });

  const [filesByPRKey, setFilesByPRKey] = useState<PRFilesByKey>({});
  // Refs so we can synchronously skip duplicate fetches without extra
  // state updates (the lint rule rightly objects to setState-in-effect).
  // Reset whenever the desired key set changes — a new last_synced_at
  // counts as a brand-new fetch.
  const inFlightRef = useRef<Set<string>>(new Set());
  const fetchedRef = useRef<Set<string>>(new Set());

  // The set of keys we *want* to have results for. Drives the diff between
  // current state and what needs fetching, and lets us GC stale entries
  // (e.g. when a PR is deleted upstream or last_synced_at advances).
  const desiredKeys = useMemo(() => prs.map(fetchKey), [prs]);

  // Drop cached results / tracking refs whose key is no longer desired.
  // Without this, switching tasks would leak stale PR file lists forever.
  // The setState is the GC step for an external (Zustand) state change —
  // pruneByKeySet returns the same reference when nothing changed, so this
  // does not cause cascading renders.
  useEffect(() => {
    const desiredSet = new Set(desiredKeys);
    for (const k of inFlightRef.current) {
      if (!desiredSet.has(k)) inFlightRef.current.delete(k);
    }
    for (const k of fetchedRef.current) {
      if (!desiredSet.has(k)) fetchedRef.current.delete(k);
    }
    // eslint-disable-next-line react-hooks/set-state-in-effect -- GC for external store change; no-op when nothing was pruned.
    setFilesByPRKey((prev) => pruneByKeySet(prev, desiredSet));
  }, [desiredKeys]);

  // Issue one fetch per PR that hasn't been fetched yet under its current key.
  useEffect(() => {
    const client = getWebSocketClient();
    if (!client) return;
    for (const pr of prs) {
      const key = fetchKey(pr);
      if (fetchedRef.current.has(key) || inFlightRef.current.has(key)) continue;
      inFlightRef.current.add(key);
      void client
        .request<{ files?: PRDiffFile[] }>("github.pr_files.get", {
          owner: pr.owner,
          repo: pr.repo,
          number: pr.pr_number,
        })
        .then((response) => {
          inFlightRef.current.delete(key);
          fetchedRef.current.add(key);
          setFilesByPRKey((prev) => ({ ...prev, [key]: response?.files ?? [] }));
        })
        .catch(() => {
          inFlightRef.current.delete(key);
          fetchedRef.current.add(key);
          setFilesByPRKey((prev) => ({ ...prev, [key]: [] }));
        });
    }
    // No cleanup-time cancellation: the per-key dedup via inFlightRef +
    // fetchedRef already prevents duplicate requests, and the response
    // handlers use functional setState so they're safe to land after the
    // effect re-runs. Adding `cancelled = true` here used to drop responses
    // from the previous effect instance — and since the next effect's
    // early-continue saw the key still in inFlightRef, no fresh request
    // was issued either, leaving files permanently empty.
  }, [prs]);

  return { prs, filesByPRKey };
}

function pruneByKeySet<V>(prev: Record<string, V>, desiredSet: Set<string>): Record<string, V> {
  let changed = false;
  const next: Record<string, V> = {};
  for (const k of Object.keys(prev)) {
    if (desiredSet.has(k)) {
      next[k] = prev[k];
    } else {
      changed = true;
    }
  }
  return changed ? next : prev;
}

export { fetchKey as prFetchKey };
