"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { PRDiffFile, TaskPR } from "@/lib/types/github";

type PRFilesByKey = Record<string, PRDiffFile[]>;
type WorkspacePRFiles = { workspaceId: string | null; files: PRFilesByKey };

// Stable empty array so the Zustand selector returns the same reference
// for tasks with zero PRs. A fresh `[]` per render would re-trigger the
// selector subscriber and cascade through every effect that depends on
// `prs`.
const EMPTY_PRS: TaskPR[] = [];
const EMPTY_FILES: PRFilesByKey = {};

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
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const prs = useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return EMPTY_PRS;
    return s.taskPRs.byTaskId[taskId] ?? EMPTY_PRS;
  });

  const [fileCache, setFileCache] = useState<WorkspacePRFiles>({
    workspaceId,
    files: EMPTY_FILES,
  });
  // Refs so we can synchronously skip duplicate fetches without extra
  // state updates (the lint rule rightly objects to setState-in-effect).
  // Reset whenever the desired key set changes — a new last_synced_at
  // counts as a brand-new fetch.
  const inFlightRef = useRef<Set<string>>(new Set());
  const fetchedRef = useRef<Set<string>>(new Set());
  const workspaceIdRef = useRef(workspaceId);
  workspaceIdRef.current = workspaceId;

  // The set of keys we *want* to have results for. Drives the diff between
  // current state and what needs fetching, and lets us GC stale entries
  // (e.g. when a PR is deleted upstream or last_synced_at advances).
  const desiredKeys = useMemo(() => prs.map(fetchKey), [prs]);
  const desiredTrackingKeys = useMemo(
    () => desiredKeys.map((key) => `${workspaceId ?? ""}/${key}`),
    [desiredKeys, workspaceId],
  );

  // Drop cached results / tracking refs whose key is no longer desired.
  // Without this, switching tasks would leak stale PR file lists forever.
  // The setState is the GC step for an external (Zustand) state change —
  // pruneByKeySet returns the same reference when nothing changed, so this
  // does not cause cascading renders.
  useEffect(() => {
    const desiredSet = new Set(desiredKeys);
    const desiredTrackingSet = new Set(desiredTrackingKeys);
    for (const k of inFlightRef.current) {
      if (!desiredTrackingSet.has(k)) inFlightRef.current.delete(k);
    }
    for (const k of fetchedRef.current) {
      if (!desiredTrackingSet.has(k)) fetchedRef.current.delete(k);
    }
    // eslint-disable-next-line react-hooks/set-state-in-effect -- GC for external store change; no-op when nothing was pruned.
    setFileCache((prev) => {
      if (prev.workspaceId !== workspaceId) return { workspaceId, files: EMPTY_FILES };
      const files = pruneByKeySet(prev.files, desiredSet);
      return files === prev.files ? prev : { workspaceId, files };
    });
  }, [desiredKeys, desiredTrackingKeys, workspaceId]);

  // Issue one fetch per PR that hasn't been fetched yet under its current key.
  useEffect(() => {
    const client = getWebSocketClient();
    if (!client || !workspaceId) return;
    for (const pr of prs) {
      const key = fetchKey(pr);
      const trackingKey = `${workspaceId}/${key}`;
      if (fetchedRef.current.has(trackingKey) || inFlightRef.current.has(trackingKey)) continue;
      inFlightRef.current.add(trackingKey);
      void client
        .request<{ files?: PRDiffFile[] }>("github.pr_files.get", {
          workspace_id: workspaceId,
          owner: pr.owner,
          repo: pr.repo,
          number: pr.pr_number,
        })
        .then((response) => {
          inFlightRef.current.delete(trackingKey);
          if (workspaceIdRef.current !== workspaceId) return;
          fetchedRef.current.add(trackingKey);
          setFileCache((prev) => ({
            workspaceId,
            files: {
              ...(prev.workspaceId === workspaceId ? prev.files : EMPTY_FILES),
              [key]: response?.files ?? [],
            },
          }));
        })
        .catch(() => {
          inFlightRef.current.delete(trackingKey);
          if (workspaceIdRef.current !== workspaceId) return;
          fetchedRef.current.add(trackingKey);
          setFileCache((prev) => ({
            workspaceId,
            files: {
              ...(prev.workspaceId === workspaceId ? prev.files : EMPTY_FILES),
              [key]: [],
            },
          }));
        });
    }
    // No cleanup-time cancellation: the per-key dedup via inFlightRef +
    // fetchedRef already prevents duplicate requests, and the response
    // handlers use functional setState so they're safe to land after the
    // effect re-runs. Adding `cancelled = true` here used to drop responses
    // from the previous effect instance — and since the next effect's
    // early-continue saw the key still in inFlightRef, no fresh request
    // was issued either, leaving files permanently empty.
  }, [prs, workspaceId]);

  return {
    prs,
    filesByPRKey: fileCache.workspaceId === workspaceId ? fileCache.files : EMPTY_FILES,
  };
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
