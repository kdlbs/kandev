"use client";

import { useEffect, useCallback, useRef, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { QueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { githubQueryOptions } from "@/lib/query/query-options/github";
import type { TaskPR } from "@/lib/types/github";

const SYNC_RETRY_DELAY = 5_000; // 5 seconds
const SYNC_MAX_RETRIES = 6; // Up to 30 seconds of retries

const EMPTY_PRS: TaskPR[] = [];

export function getPrimaryTaskPR(prs: TaskPR[] | undefined): TaskPR | null {
  return prs && prs.length > 0 ? prs[0] : null;
}

/**
 * Normalises the WS sync response into an array of TaskPR rows. Backend
 * returns `{prs: TaskPR[]}` (current shape) for multi-repo support, but we
 * accept the legacy bare-TaskPR shape too in case an older backend is
 * still running. Empty / null / unknown shapes return an empty array.
 */
function normalizeSyncResponse(result: { prs?: TaskPR[] } | TaskPR | null | undefined): TaskPR[] {
  if (!result) return [];
  const envelope = result as { prs?: TaskPR[] };
  if (Array.isArray(envelope.prs)) return envelope.prs;
  const single = result as TaskPR;
  if (single.task_id) return [single];
  return [];
}

/**
 * Upsert a PR row by (repository_id, pr_number) so multi-branch tasks can hold
 * N PRs on the same repo as siblings. For legacy rows without a repository_id,
 * match on the empty key + pr_number, preserving prior single-PR semantics.
 * (Mirrors the bridge's upsertTaskPR.)
 */
function upsertTaskPR(
  existing: Record<string, TaskPR[]> | undefined,
  pr: TaskPR,
): Record<string, TaskPR[]> {
  const byTaskId = existing ?? {};
  const current = byTaskId[pr.task_id];
  const list = Array.isArray(current) ? current : [];
  const repoKey = pr.repository_id ?? "";
  const idx = list.findIndex(
    (p) => (p.repository_id ?? "") === repoKey && p.pr_number === pr.pr_number,
  );
  const next = idx >= 0 ? list.map((p, i) => (i === idx ? pr : p)) : [...list, pr];
  return { ...byTaskId, [pr.task_id]: next };
}

/**
 * Upsert a PR into every cached workspace PR query. The PR event/sync carries
 * task_id but not workspace_id, so — like the bridge — we scan all cached
 * `["github", *, "prs"]` queries and update each one.
 */
export function upsertTaskPRIntoCaches(qc: QueryClient, pr: TaskPR): void {
  if (!pr.task_id) return;
  const queries = qc.getQueryCache().findAll({
    predicate: (q) => {
      const key = q.queryKey as unknown[];
      return key[0] === "github" && key[2] === "prs";
    },
  });
  for (const q of queries) {
    qc.setQueryData<{ task_prs: Record<string, TaskPR[]> }>(q.queryKey, (prev) => {
      if (!prev) return prev;
      return { ...prev, task_prs: upsertTaskPR(prev.task_prs, pr) };
    });
  }
}

/**
 * Read a task's PRs from the TQ cache without React (for imperative dockview
 * layout decisions). Scans every cached workspace PR query.
 */
export function readTaskPRsFromCache(qc: QueryClient, taskId: string): TaskPR[] {
  const queries = qc.getQueryCache().findAll({
    predicate: (q) => {
      const key = q.queryKey as unknown[];
      return key[0] === "github" && key[2] === "prs";
    },
  });
  for (const q of queries) {
    const data = qc.getQueryData<{ task_prs: Record<string, TaskPR[]> }>(q.queryKey);
    const prs = data?.task_prs?.[taskId];
    if (Array.isArray(prs) && prs.length > 0) return prs;
  }
  return EMPTY_PRS;
}

/** The active workspace id (client-only Zustand state, not part of this domain). */
function useActiveWorkspaceId(): string | null {
  return useAppStore((s) => s.workspaces.activeId);
}

/**
 * Fetch all PR associations for a workspace into the TQ cache. Side-effect
 * only — callers (kanban board, session sidebar) trigger the fetch; readers
 * observe the cache via `useTaskPRsByTaskId` / `useTaskPRs`.
 */
export function useWorkspacePRs(workspaceId: string | null) {
  useQuery(githubQueryOptions.workspacePRs(workspaceId ?? ""));
}

/**
 * The full taskId → PR[] map for a workspace, read from the TQ cache.
 * Defaults to the active workspace. Observe-only (does not fetch) — the data
 * is fetched by `useWorkspacePRs` on the board / sidebar.
 */
export function useTaskPRsByTaskId(workspaceId?: string | null): Record<string, TaskPR[]> {
  const activeId = useActiveWorkspaceId();
  const wsId = workspaceId === undefined ? activeId : workspaceId;
  const { data } = useQuery({
    ...githubQueryOptions.workspacePRs(wsId ?? ""),
    enabled: false,
  });
  return data?.task_prs ?? {};
}

/** The PR list for a single task, read from the active workspace TQ cache. */
export function useTaskPRs(taskId: string | null): TaskPR[] {
  const byTaskId = useTaskPRsByTaskId();
  return taskId ? (byTaskId[taskId] ?? EMPTY_PRS) : EMPTY_PRS;
}

/**
 * Fetch a single task's PR associations, with on-demand sync via WS.
 *
 * Reads from the TQ workspace-PR cache (populated by the bridge for live WS
 * events and by `useWorkspacePRs` for the initial fetch) and triggers a WS
 * sync request to backfill when a task has no PR cached yet.
 */
export function useTaskPR(taskId: string | null) {
  const qc = useQueryClient();
  // Ensure the workspace PR query exists and is fetched on any surface that
  // mounts this hook. On desktop the board / sidebar already call
  // useWorkspacePRs, but the mobile layout mounts neither — without this the
  // qk.github.prs(wsId) query is never created, so the bridge and WS sync
  // (both no-op when prev === undefined) have no cache to write into and the
  // chip / changes panel stay empty. TQ dedupes, so this is a no-op fetch
  // when another consumer already created the query.
  const activeWorkspaceId = useActiveWorkspaceId();
  useWorkspacePRs(activeWorkspaceId);
  const prs = useTaskPRs(taskId);
  const pr = getPrimaryTaskPR(prs);
  const retryRef = useRef(0);

  const refresh = useCallback(() => {
    if (!taskId) return;
    const client = getWebSocketClient();
    if (!client) return;

    client
      .request<{ prs?: TaskPR[] } | TaskPR | null>("github.task_pr.sync", {
        task_id: taskId,
      })
      .then((result) => {
        const list = normalizeSyncResponse(result);
        if (list.length === 0) return;
        for (const taskPR of list) {
          if (taskPR.task_id) upsertTaskPRIntoCaches(qc, taskPR);
        }
        retryRef.current = 0;
      })
      .catch(() => {
        // Ignore — sync may fail if no watch exists
      });
  }, [taskId, qc]);

  // Reset retry count when taskId changes.
  useEffect(() => {
    retryRef.current = 0;
  }, [taskId]);

  // Sync once when the task becomes active.
  useEffect(() => {
    if (!taskId) return;
    refresh();
  }, [taskId, refresh]);

  // Retry polling when no PR is in the cache yet.
  useEffect(() => {
    if (!taskId || pr) return;
    const interval = setInterval(() => {
      if (retryRef.current >= SYNC_MAX_RETRIES) {
        clearInterval(interval);
        return;
      }
      retryRef.current++;
      refresh();
    }, SYNC_RETRY_DELAY);
    return () => clearInterval(interval);
  }, [taskId, pr, refresh]);

  return useMemo(
    () => ({ pr, prs, refresh }) as { pr: TaskPR | null; prs: TaskPR[]; refresh: () => void },
    [pr, prs, refresh],
  );
}

/** Read the active task's primary PR from the TQ cache (no fetching). */
export function useActiveTaskPR(): TaskPR | null {
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const prs = useTaskPRs(activeTaskId);
  return getPrimaryTaskPR(prs);
}
