"use client";

import { useEffect } from "react";
import { fetchGitLabStats } from "@/lib/api/domains/gitlab-api";
import { useAppStore } from "@/components/state-provider";

/**
 * useGitLabStats subscribes to the open-MRs / awaiting-review / open-issues
 * counts surfaced on the /gitlab page header.
 */
export function useGitLabStats() {
  const stats = useAppStore((state) => state.gitlabStats.data);
  const loading = useAppStore((state) => state.gitlabStats.loading);
  const setStats = useAppStore((state) => state.setGitLabStats);
  const setStatsLoading = useAppStore((state) => state.setGitLabStatsLoading);

  useEffect(() => {
    if (stats || loading) return;
    setStatsLoading(true);
    fetchGitLabStats()
      .then((res) => setStats(res ?? null))
      .catch(() => setStats(null))
      .finally(() => setStatsLoading(false));
  }, [stats, loading, setStats, setStatsLoading]);

  return { stats, loading };
}
