"use client";

import { useCallback, useEffect, useRef } from "react";
import { fetchGitLabStatus } from "@/lib/api/domains/gitlab-api";
import { useAppStore } from "@/components/state-provider";
import { subscribeIntegrationAvailability } from "@/lib/integrations/integration-availability-events";

const requestVersions = new Map<string, number>();

function nextRequestVersion(workspaceId: string) {
  const version = (requestVersions.get(workspaceId) ?? 0) + 1;
  requestVersions.set(workspaceId, version);
  return version;
}

function isCurrentRequest(workspaceId: string, version: number) {
  return requestVersions.get(workspaceId) === version;
}

/**
 * useGitLabStatus subscribes the slice to the latest GitLab connection status.
 * Fetches on mount, retries are caller-driven via the returned `refresh`.
 *
 * Requests are shared by workspace so a consumer unmount cannot strand a
 * status request that another consumer relies on.
 */
export function useGitLabStatus(requestedWorkspaceId?: string | null) {
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceId = requestedWorkspaceId ?? activeWorkspaceId;
  const statusEntry = useAppStore((state) =>
    workspaceId ? state.gitlabStatus.byWorkspaceId[workspaceId] : undefined,
  );
  const status = statusEntry?.data ?? null;
  const loading = statusEntry?.loading ?? Boolean(workspaceId);
  const setStatus = useAppStore((state) => state.setGitLabStatus);
  const setStatusLoading = useAppStore((state) => state.setGitLabStatusLoading);
  const resetStatus = useAppStore((state) => state.resetGitLabStatus);
  const currentWorkspaceId = useRef(workspaceId);
  currentWorkspaceId.current = workspaceId;

  const loadStatus = useCallback(
    async (requestedWorkspaceId: string) => {
      const version = nextRequestVersion(requestedWorkspaceId);
      const isCurrentConsumerRequest = () =>
        isCurrentRequest(requestedWorkspaceId, version) &&
        currentWorkspaceId.current === requestedWorkspaceId;

      setStatusLoading(requestedWorkspaceId, true);
      try {
        const res = await fetchGitLabStatus({
          cache: "no-store",
          workspaceId: requestedWorkspaceId,
        });
        if (isCurrentConsumerRequest()) {
          setStatus(requestedWorkspaceId, res ?? null);
        }
      } catch {
        if (isCurrentConsumerRequest()) {
          setStatus(requestedWorkspaceId, null);
        }
      } finally {
        if (isCurrentConsumerRequest()) {
          setStatusLoading(requestedWorkspaceId, false);
        }
      }
    },
    [setStatus, setStatusLoading],
  );

  useEffect(() => {
    if (!workspaceId) return;
    if (!statusEntry) {
      resetStatus(workspaceId);
      void loadStatus(workspaceId);
      return;
    }
    if (statusEntry.loadedAt === null && !statusEntry.loading) void loadStatus(workspaceId);
  }, [loadStatus, resetStatus, statusEntry?.loadedAt, statusEntry?.loading, workspaceId]);

  const refresh = useCallback(async () => {
    const requestedWorkspaceId = currentWorkspaceId.current;
    if (!requestedWorkspaceId) return;
    await loadStatus(requestedWorkspaceId);
  }, [loadStatus]);

  useEffect(() => subscribeIntegrationAvailability(() => void refresh()), [refresh]);

  return { status, loading, refresh };
}
