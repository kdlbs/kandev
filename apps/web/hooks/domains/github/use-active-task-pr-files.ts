"use client";

import { useMemo } from "react";
import { useQueries } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { qk } from "@/lib/query/keys";
import { useTaskPRs, useWorkspacePRs } from "./use-task-pr";
import type { PRDiffFile, TaskPR } from "@/lib/types/github";

type PRFilesByKey = Record<string, PRDiffFile[]>;

/**
 * Cache key for an in-flight fetch — owner/repo/PR + the last_synced_at hint
 * from the TaskPR row, so a server-side sync invalidates the cache and
 * triggers a refetch automatically.
 */
export function prFetchKey(pr: TaskPR): string {
  return `${pr.owner}/${pr.repo}/${pr.pr_number}/${pr.last_synced_at ?? ""}`;
}

async function fetchPRFiles(pr: TaskPR): Promise<PRDiffFile[]> {
  const client = getWebSocketClient();
  if (!client) return [];
  const response = await client.request<{ files?: PRDiffFile[] }>("github.pr_files.get", {
    owner: pr.owner,
    repo: pr.repo,
    number: pr.pr_number,
  });
  return response?.files ?? [];
}

/**
 * Returns one diff array per task PR, keyed by `${owner}/${repo}/${prNumber}/${last_synced_at}`.
 * Fans out one TQ query per PR using useQueries, so dedup and caching are handled by TQ.
 *
 * Designed for the changes panel's PR Changes section, which needs to render
 * one row per file across every per-repo PR (multi-repo tasks have one
 * PR per repo, not just one for the whole task).
 */
export function useActiveTaskPRsWithFiles(): {
  prs: TaskPR[];
  filesByPRKey: PRFilesByKey;
} {
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  // Fetch the workspace PR query so the PR Changes section works on surfaces
  // that don't mount the desktop board / sidebar (e.g. the mobile layout).
  // Without this the qk.github.prs(wsId) query is never created and useTaskPRs
  // returns empty. TQ dedupes when another consumer already created it.
  useWorkspacePRs(activeWorkspaceId);
  const prs = useTaskPRs(activeTaskId);

  const queries = useQueries({
    queries: prs.map((pr) => ({
      queryKey: qk.github.prFiles(pr.owner, pr.repo, pr.pr_number, pr.last_synced_at),
      queryFn: () => fetchPRFiles(pr),
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    })),
  });

  const filesByPRKey = useMemo(() => {
    const result: PRFilesByKey = {};
    for (let i = 0; i < prs.length; i++) {
      const key = prFetchKey(prs[i]);
      result[key] = queries[i]?.data ?? [];
    }
    return result;
  }, [prs, queries]);

  return { prs, filesByPRKey };
}
