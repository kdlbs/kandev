"use client";

import { useEffect, useRef, useState } from "react";
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
export function usePRStatuses(prs: GitHubPR[]): Map<string, GitHubPRStatus> {
  const [statuses, setStatuses] = useState<Map<string, GitHubPRStatus>>(new Map());
  const requestedKey = useRef<string>("");

  const key = prs.map((p) => prStatusKey(p.repo_owner, p.repo_name, p.number)).join(",");

  // We deliberately depend only on `key`: `prs` gets a new array identity on
  // every render, and including it would cancel an in-flight request whose
  // content hasn't actually changed. The composed `key` string is the
  // authoritative content signal; when it matches we reuse the latest `prs`
  // closure for building request refs.
  useEffect(() => {
    if (key === "") return;
    if (requestedKey.current === key) return;
    requestedKey.current = key;
    const refs: PRStatusRef[] = prs.map((p) => ({
      owner: p.repo_owner,
      repo: p.repo_name,
      number: p.number,
    }));
    let cancelled = false;
    getPRStatusesBatch(refs)
      .then((resp) => {
        if (cancelled) return;
        setStatuses(new Map(Object.entries(resp.statuses ?? {})));
      })
      .catch(() => {
        if (cancelled) return;
        setStatuses(new Map());
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  return statuses;
}
