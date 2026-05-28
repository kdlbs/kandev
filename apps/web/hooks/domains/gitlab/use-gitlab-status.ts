"use client";

import { useEffect } from "react";
import { fetchGitLabStatus } from "@/lib/api/domains/gitlab-api";
import { useAppStore } from "@/components/state-provider";

/**
 * useGitLabStatus subscribes the slice to the latest GitLab connection status.
 * Fetches on mount and on demand via the returned `refresh` function.
 */
export function useGitLabStatus() {
  const status = useAppStore((state) => state.gitlabStatus.data);
  const loading = useAppStore((state) => state.gitlabStatus.loading);
  const setStatus = useAppStore((state) => state.setGitLabStatus);
  const setStatusLoading = useAppStore((state) => state.setGitLabStatusLoading);

  useEffect(() => {
    if (status || loading) return;
    setStatusLoading(true);
    fetchGitLabStatus({ cache: "no-store" })
      .then((res) => setStatus(res ?? null))
      .catch(() => setStatus(null))
      .finally(() => setStatusLoading(false));
  }, [status, loading, setStatus, setStatusLoading]);

  const refresh = async () => {
    setStatusLoading(true);
    try {
      const res = await fetchGitLabStatus({ cache: "no-store" });
      setStatus(res ?? null);
    } catch {
      setStatus(null);
    } finally {
      setStatusLoading(false);
    }
  };

  return { status, loading, refresh };
}
