"use client";

import { useEffect, useRef } from "react";
import { fetchGitLabStats } from "@/lib/api/domains/gitlab-api";
import { useAppStore } from "@/components/state-provider";

/**
 * useGitLabStats subscribes to the open-MRs / awaiting-review / open-issues
 * counts surfaced on the /gitlab page header. Per-mount attempted flag
 * prevents an infinite re-fetch loop when GitLab is unreachable.
 */
export function useGitLabStats(workspaceId: string | null) {
  const stats = useAppStore((state) => state.gitlabStats.data);
  const loading = useAppStore((state) => state.gitlabStats.loading);
  const loadedAt = useAppStore((state) => state.gitlabStats.loadedAt);
  const setStats = useAppStore((state) => state.setGitLabStats);
  const setStatsLoading = useAppStore((state) => state.setGitLabStatsLoading);
  const attemptedRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (!workspaceId || loading || loadedAt !== null || attemptedRef.current.has(workspaceId))
      return;
    attemptedRef.current.add(workspaceId);
    setStatsLoading(true);
    fetchGitLabStats(workspaceId)
      .then((res) => setStats(res ?? null))
      .catch(() => setStats(null))
      .finally(() => setStatsLoading(false));
  }, [loading, loadedAt, setStats, setStatsLoading, workspaceId]);

  return { stats, loading };
}
