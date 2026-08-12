"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { PRDiffFile, TaskPR } from "@/lib/types/github";

type PRFilesByKey = Record<string, PRDiffFile[]>;
type ScopedPRFiles = {
  workspaceId: string | null;
  taskId: string | null;
  files: PRFilesByKey;
};

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

function prIdentityKey(pr: TaskPR): string {
  return `${pr.owner}/${pr.repo}/${pr.pr_number}`;
}

function fetchKeyIdentity(key: string): string {
  return key.slice(0, key.lastIndexOf("/"));
}

function isCurrentScope(
  scope: { workspaceId: string | null; taskId: string | null },
  workspaceId: string,
  taskId: string | null,
): boolean {
  return scope.workspaceId === workspaceId && scope.taskId === taskId;
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
export function useActiveTaskPRsWithFiles(scopedPRs?: TaskPR[]): {
  prs: TaskPR[];
  filesByPRKey: PRFilesByKey;
} {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const taskPRs = useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return EMPTY_PRS;
    return s.taskPRs.byTaskId[taskId] ?? EMPTY_PRS;
  });
  const prs = scopedPRs ?? taskPRs;

  const [fileCache, setFileCache] = useState<ScopedPRFiles>({
    workspaceId,
    taskId: activeTaskId,
    files: EMPTY_FILES,
  });
  // Refs so we can synchronously skip duplicate fetches without extra
  // state updates (the lint rule rightly objects to setState-in-effect).
  // Reset whenever the desired key set changes — a new last_synced_at
  // counts as a brand-new fetch.
  const inFlightRef = useRef<Set<string>>(new Set());
  const fetchedRef = useRef<Set<string>>(new Set());
  const scopeRef = useRef({ workspaceId, taskId: activeTaskId });
  scopeRef.current = { workspaceId, taskId: activeTaskId };

  // The set of keys we *want* to have results for. Drives the diff between
  // current state and what needs fetching, and lets us GC stale entries
  // (e.g. when a PR is deleted upstream or last_synced_at advances).
  const desiredKeys = useMemo(() => prs.map(fetchKey), [prs]);
  const desiredIdentities = useMemo(() => prs.map(prIdentityKey), [prs]);
  const desiredTrackingKeys = useMemo(
    () => desiredKeys.map((key) => `${workspaceId ?? ""}/${activeTaskId ?? ""}/${key}`),
    [activeTaskId, desiredKeys, workspaceId],
  );

  // Drop cached results / tracking refs whose PR identity is no longer desired.
  // Without this, switching tasks would leak stale PR file lists forever.
  // The setState is the GC step for an external (Zustand) state change —
  // pruneByIdentitySet returns the same reference when nothing changed, so this
  // does not cause cascading renders.
  useEffect(() => {
    const desiredIdentitySet = new Set(desiredIdentities);
    const desiredTrackingSet = new Set(desiredTrackingKeys);
    for (const k of inFlightRef.current) {
      if (!desiredTrackingSet.has(k)) inFlightRef.current.delete(k);
    }
    for (const k of fetchedRef.current) {
      if (!desiredTrackingSet.has(k)) fetchedRef.current.delete(k);
    }
    // eslint-disable-next-line react-hooks/set-state-in-effect -- GC for external store change; no-op when nothing was pruned.
    setFileCache((prev) => {
      if (prev.workspaceId !== workspaceId || prev.taskId !== activeTaskId) {
        return { workspaceId, taskId: activeTaskId, files: EMPTY_FILES };
      }
      const files = pruneByIdentitySet(prev.files, desiredIdentitySet);
      return files === prev.files ? prev : { workspaceId, taskId: activeTaskId, files };
    });
  }, [activeTaskId, desiredIdentities, desiredTrackingKeys, workspaceId]);

  // Issue one fetch per PR that hasn't been fetched yet under its current key.
  useEffect(() => {
    const client = getWebSocketClient();
    if (!client || !workspaceId) return;
    for (const pr of prs) {
      const key = fetchKey(pr);
      const trackingKey = `${workspaceId}/${activeTaskId ?? ""}/${key}`;
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
          if (!isCurrentScope(scopeRef.current, workspaceId, activeTaskId)) return;
          fetchedRef.current.add(trackingKey);
          setFileCache((prev) =>
            replacePRFiles(prev, workspaceId, activeTaskId, key, response?.files ?? []),
          );
        })
        .catch(() => {
          inFlightRef.current.delete(trackingKey);
          if (!isCurrentScope(scopeRef.current, workspaceId, activeTaskId)) return;
          fetchedRef.current.add(trackingKey);
          setFileCache((prev) => retainOrSetEmptyPRFiles(prev, workspaceId, activeTaskId, key));
        });
    }
    // No cleanup-time cancellation: the per-key dedup via inFlightRef +
    // fetchedRef already prevents duplicate requests, and the response
    // handlers use functional setState so they're safe to land after the
    // effect re-runs. Adding `cancelled = true` here used to drop responses
    // from the previous effect instance — and since the next effect's
    // early-continue saw the key still in inFlightRef, no fresh request
    // was issued either, leaving files permanently empty.
  }, [activeTaskId, prs, workspaceId]);

  const filesByPRKey = useMemo(() => {
    if (fileCache.workspaceId !== workspaceId || fileCache.taskId !== activeTaskId) {
      return EMPTY_FILES;
    }
    const projected: PRFilesByKey = {};
    for (const pr of prs) {
      const key = fetchKey(pr);
      const files =
        fileCache.files[key] ??
        Object.entries(fileCache.files).find(
          ([cachedKey]) => fetchKeyIdentity(cachedKey) === prIdentityKey(pr),
        )?.[1];
      if (files) projected[key] = files;
    }
    return Object.keys(projected).length > 0 ? projected : EMPTY_FILES;
  }, [activeTaskId, fileCache, prs, workspaceId]);

  return {
    prs,
    filesByPRKey,
  };
}

function pruneByIdentitySet<V>(
  prev: Record<string, V>,
  desiredIdentitySet: Set<string>,
): Record<string, V> {
  let changed = false;
  const next: Record<string, V> = {};
  for (const k of Object.keys(prev)) {
    if (desiredIdentitySet.has(fetchKeyIdentity(k))) {
      next[k] = prev[k];
    } else {
      changed = true;
    }
  }
  return changed ? next : prev;
}

function replacePRFiles(
  prev: ScopedPRFiles,
  workspaceId: string,
  taskId: string | null,
  key: string,
  files: PRDiffFile[],
): ScopedPRFiles {
  const previousFiles =
    prev.workspaceId === workspaceId && prev.taskId === taskId ? prev.files : EMPTY_FILES;
  const identity = fetchKeyIdentity(key);
  const next = Object.fromEntries(
    Object.entries(previousFiles).filter(([cachedKey]) => fetchKeyIdentity(cachedKey) !== identity),
  );
  next[key] = files;
  return { workspaceId, taskId, files: next };
}

function retainOrSetEmptyPRFiles(
  prev: ScopedPRFiles,
  workspaceId: string,
  taskId: string | null,
  key: string,
): ScopedPRFiles {
  if (prev.workspaceId === workspaceId && prev.taskId === taskId) {
    const identity = fetchKeyIdentity(key);
    if (Object.keys(prev.files).some((cachedKey) => fetchKeyIdentity(cachedKey) === identity)) {
      return prev;
    }
  }
  return replacePRFiles(prev, workspaceId, taskId, key, []);
}

export { fetchKey as prFetchKey };
