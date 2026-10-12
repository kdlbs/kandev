"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { getPRStatusesBatch, type PRStatusRef } from "@/lib/api/domains/github-api";
import type { GitHubPR, GitHubPRStatus } from "@/lib/types/github";

export function prStatusKey(owner: string, repo: string, number: number): string {
  return `${owner}/${repo}#${number}`;
}

// usePRStatuses fetches PR review/checks/mergeable summaries in a single
// batch request instead of N per-row fetches. Results are keyed by
// prStatusKey(owner, repo, number). When the PR list changes (new page,
// different preset) we refetch; the backend caches per-PR so fast
// pagination stays cheap.
export function usePRStatuses(
  workspaceId: string | null,
  prs: GitHubPR[],
): Map<string, GitHubPRStatus> {
  const emptyStatuses = useMemo(() => new Map<string, GitHubPRStatus>(), []);
  const [result, setResult] = useState(() => ({ key: "", statuses: emptyStatuses }));
  // Only successful, uncancelled reads record a completed key. StrictMode's
  // cancelled first effect therefore cannot suppress the second effect's read.
  const completedKey = useRef<string>("");

  const key =
    workspaceId && prs.length > 0
      ? `${workspaceId}:${prs.map((p) => prStatusKey(p.repo_owner, p.repo_name, p.number)).join(",")}`
      : "";

  // We deliberately depend only on `key`: `prs` gets a new array identity on
  // every render, and including it would cancel an in-flight request whose
  // content hasn't actually changed. The composed `key` string is the
  // authoritative content signal; when it matches we reuse the latest `prs`
  // closure for building request refs.
  useEffect(() => {
    if (key === "") {
      completedKey.current = "";
      setResult({ key: "", statuses: emptyStatuses });
      return;
    }
    if (completedKey.current === key) return;
    const refs: PRStatusRef[] = prs.map((p) => ({
      owner: p.repo_owner,
      repo: p.repo_name,
      number: p.number,
    }));
    let cancelled = false;
    getPRStatusesBatch(workspaceId!, refs)
      .then((resp) => {
        if (cancelled) return;
        completedKey.current = key;
        setResult({ key, statuses: new Map(Object.entries(resp.statuses ?? {})) });
      })
      .catch(() => {
        if (cancelled) return;
        // Failure clears retained summaries without recording a completed key.
        setResult((prev) =>
          prev.statuses.size === 0 ? prev : { key: "", statuses: emptyStatuses },
        );
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, workspaceId]);

  return key !== "" && result.key === key ? result.statuses : emptyStatuses;
}
