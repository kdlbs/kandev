"use client";

import { useEffect } from "react";
import { fetchGitHubStatus } from "@/lib/api/domains/github-api";
import { useAppStore } from "@/components/state-provider";

export function useGitHubStatus() {
  const status = useAppStore((state) => state.githubStatus.status);
  const loaded = useAppStore((state) => state.githubStatus.loaded);
  const loading = useAppStore((state) => state.githubStatus.loading);
  const setGitHubStatus = useAppStore((state) => state.setGitHubStatus);
  const setGitHubStatusLoading = useAppStore((state) => state.setGitHubStatusLoading);

  useEffect(() => {
    if (loaded || loading) return;
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
  }, [loaded, loading, setGitHubStatus, setGitHubStatusLoading]);

  return { status, loaded, loading };
}
