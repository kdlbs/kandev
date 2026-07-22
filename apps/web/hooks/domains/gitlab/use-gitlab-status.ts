"use client";

import { useCallback, useEffect, useRef } from "react";
import { fetchGitLabStatus } from "@/lib/api/domains/gitlab-api";
import { useAppStore } from "@/components/state-provider";
import { subscribeIntegrationAvailability } from "@/lib/integrations/integration-availability-events";

/**
 * useGitLabStatus subscribes the slice to the latest GitLab connection status.
 * Fetches on mount, retries are caller-driven via the returned `refresh`.
 *
 * Effect-driven and imperative fetches share one generation guard so a stale
 * workspace response cannot overwrite the currently selected workspace.
 */
export function useGitLabStatus() {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const statusSnapshot = useAppStore((state) => state.gitlabStatus);
  const ownsSnapshot = statusSnapshot.workspaceId === workspaceId;
  const status = ownsSnapshot ? statusSnapshot.data : null;
  const loading = ownsSnapshot ? statusSnapshot.loading : Boolean(workspaceId);
  const setStatus = useAppStore((state) => state.setGitLabStatus);
  const setStatusLoading = useAppStore((state) => state.setGitLabStatusLoading);
  const requestGeneration = useRef(0);
  const currentWorkspaceId = useRef(workspaceId);
  currentWorkspaceId.current = workspaceId;

  const loadStatus = useCallback(
    async (requestedWorkspaceId: string) => {
      const generation = ++requestGeneration.current;
      const isCurrentRequest = () =>
        generation === requestGeneration.current &&
        requestedWorkspaceId === currentWorkspaceId.current;

      setStatusLoading(requestedWorkspaceId, true);
      try {
        const res = await fetchGitLabStatus({
          cache: "no-store",
          workspaceId: requestedWorkspaceId,
        });
        if (isCurrentRequest()) setStatus(requestedWorkspaceId, res ?? null);
      } catch {
        if (isCurrentRequest()) setStatus(requestedWorkspaceId, null);
      } finally {
        if (isCurrentRequest()) setStatusLoading(requestedWorkspaceId, false);
      }
    },
    [setStatus, setStatusLoading],
  );

  useEffect(() => {
    if (!workspaceId) {
      requestGeneration.current++;
      setStatus(null, null);
      setStatusLoading(null, false);
      return;
    }
    setStatus(workspaceId, null);
    void loadStatus(workspaceId);
    return () => {
      requestGeneration.current++;
    };
  }, [workspaceId, loadStatus, setStatus, setStatusLoading]);

  const refresh = useCallback(async () => {
    const requestedWorkspaceId = currentWorkspaceId.current;
    if (!requestedWorkspaceId) {
      requestGeneration.current++;
      setStatus(null, null);
      setStatusLoading(null, false);
      return;
    }
    await loadStatus(requestedWorkspaceId);
  }, [loadStatus, setStatus, setStatusLoading]);

  useEffect(() => subscribeIntegrationAvailability(() => void refresh()), [refresh]);

  return { status, loading, refresh };
}
