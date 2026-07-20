"use client";

import { useCallback, useEffect, useState } from "react";

import {
  deleteDeploymentAppRegistration,
  fetchDeploymentAppRegistration,
  startDeploymentAppRegistration,
} from "@/lib/api/domains/github-api";
import type {
  DeploymentGitHubAppStatus,
  StartDeploymentGitHubAppRequest,
} from "@/lib/types/github";

export function useDeploymentAppRegistration() {
  const [status, setStatus] = useState<DeploymentGitHubAppStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [mutating, setMutating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (signal?: AbortSignal): Promise<boolean> => {
    setLoading(true);
    setError(null);
    try {
      const nextStatus = await fetchDeploymentAppRegistration({ init: { signal } });
      setStatus(nextStatus);
      return true;
    } catch (loadError) {
      if (signal?.aborted) return false;
      setStatus(null);
      setError(loadError instanceof Error ? loadError.message : "GitHub App status is unavailable");
      return false;
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const start = useCallback(async (request: StartDeploymentGitHubAppRequest) => {
    setMutating(true);
    try {
      const result = await startDeploymentAppRegistration(request);
      setStatus((current) => (current ? { ...current, state: "registering" } : current));
      return result;
    } finally {
      setMutating(false);
    }
  }, []);

  const remove = useCallback(async () => {
    setMutating(true);
    try {
      const result = await deleteDeploymentAppRegistration();
      setStatus(null);
      setError(null);
      const refreshed = await load();
      return { deleted: result.deleted, refreshed };
    } finally {
      setMutating(false);
    }
  }, [load]);

  return { status, loading, mutating, error, reload: load, start, remove } as const;
}
