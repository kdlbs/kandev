"use client";

import { useEffect, useCallback } from "react";
import { fetchGitHubStatus } from "@/lib/api/domains/github-api";
import { useAppStore } from "@/components/state-provider";

export function useGitHubStatus() {
  const status = useAppStore((state) => state.githubStatus.status);
  const loaded = useAppStore((state) => state.githubStatus.loaded);
  const loading = useAppStore((state) => state.githubStatus.loading);
  const setGitHubStatus = useAppStore((state) => state.setGitHubStatus);
  const setGitHubStatusLoading = useAppStore((state) => state.setGitHubStatusLoading);
  const invalidateSystemHealth = useAppStore((state) => state.invalidateSystemHealth);

  const doFetch = useCallback(() => {
    setGitHubStatusLoading(true);
    fetchGitHubStatus({ cache: "no-store" })
      .then((response) => {
        setGitHubStatus(response ?? null);
      })
      .catch(() => {
        setGitHubStatus(null);
      })
      .finally(() => {
        setGitHubStatusLoading(false);
      });
  }, [setGitHubStatus, setGitHubStatusLoading]);

  useEffect(() => {
    if (loaded || loading) return;
    doFetch();
  }, [loaded, loading, doFetch]);

  const refresh = useCallback(() => {
    // Also invalidate system health so the header indicator refetches
    invalidateSystemHealth();
    doFetch();
  }, [doFetch, invalidateSystemHealth]);

  return { status, loaded, loading, refresh };
}
