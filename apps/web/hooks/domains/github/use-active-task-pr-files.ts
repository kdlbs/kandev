"use client";

import { useMemo } from "react";
import { useQueries } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { qk } from "@/lib/query/keys";
import type { PRDiffFile, TaskPR } from "@/lib/types/github";

type PRFilesByKey = Record<string, PRDiffFile[]>;

// Stable empty array so the selector returns the same reference
// for tasks with zero PRs, preventing unnecessary re-renders.
const EMPTY_PRS: TaskPR[] = [];

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
  const prs = useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return EMPTY_PRS;
    return s.taskPRs.byTaskId[taskId] ?? EMPTY_PRS;
  });

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
