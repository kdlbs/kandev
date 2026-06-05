"use client";

import { useEffect, useRef } from "react";
import { fetchGitLabStatus } from "@/lib/api/domains/gitlab-api";
import { useAppStore } from "@/components/state-provider";

/**
 * useGitLabStatus subscribes the slice to the latest GitLab connection status.
 * Fetches on mount, retries are caller-driven via the returned `refresh`.
 *
 * Guards against an infinite re-fetch loop when GitLab is unreachable: a
 * fetch failure leaves `status` null, so without a per-mount attempted flag
 * the effect would re-run every render and hammer the backend.
 */
export function useGitLabStatus() {
  const status = useAppStore((state) => state.gitlabStatus.data);
  const loading = useAppStore((state) => state.gitlabStatus.loading);
  const loadedAt = useAppStore((state) => state.gitlabStatus.loadedAt);
  const setStatus = useAppStore((state) => state.setGitLabStatus);
  const setStatusLoading = useAppStore((state) => state.setGitLabStatusLoading);
  const attemptedRef = useRef(false);

  useEffect(() => {
    if (loading || loadedAt !== null || attemptedRef.current) return;
    attemptedRef.current = true;
    setStatusLoading(true);
    fetchGitLabStatus({ cache: "no-store" })
      .then((res) => setStatus(res ?? null))
      .catch(() => setStatus(null))
      .finally(() => setStatusLoading(false));
  }, [loading, loadedAt, setStatus, setStatusLoading]);

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
