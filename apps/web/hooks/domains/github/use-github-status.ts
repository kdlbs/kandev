"use client";

import { useCallback, useEffect } from "react";
import { fetchGitHubStatus } from "@/lib/api/domains/github-api";
import { useAppStore } from "@/components/state-provider";
import type { GitHubStatus, GitHubStatusResponse } from "@/lib/types/github";
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

export function normalizeGitHubStatus(response: GitHubStatusResponse): GitHubStatus {
  const automation = response.automation ?? null;
  const personal = response.personal ?? null;
  const active = response.authenticated;
  const username = response.authenticated ? (response.effective_personal_actor?.login ?? "") : "";
  return {
    ...response,
    automation,
    personal,
    app_available: response.app_available ?? response.github_app_available,
    authenticated: active,
    username,
    auth_method: active ? response.auth_method : "none",
    token_configured: response.token_configured,
    required_scopes: response.required_scopes,
  };
}

export function useGitHubStatus(requestedWorkspaceId?: string | null) {
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceId = requestedWorkspaceId ?? activeWorkspaceId;
  const statusState = useAppStore((state) =>
    workspaceId ? state.githubStatus.byWorkspaceId[workspaceId] : undefined,
  );
  const setGitHubStatus = useAppStore((state) => state.setGitHubStatus);
  const setGitHubStatusLoading = useAppStore((state) => state.setGitHubStatusLoading);
  const resetGitHubStatus = useAppStore((state) => state.resetGitHubStatus);
  const invalidateSystemHealth = useAppStore((state) => state.invalidateSystemHealth);

  const doFetch = useCallback(() => {
    if (!workspaceId) return;
    const version = nextRequestVersion(workspaceId);
    setGitHubStatusLoading(workspaceId, true);
    fetchGitHubStatus(workspaceId, { cache: "no-store" })
      .then((response) => {
        if (isCurrentRequest(workspaceId, version)) {
          setGitHubStatus(workspaceId, normalizeGitHubStatus(response));
        }
      })
      .catch(() => {
        if (isCurrentRequest(workspaceId, version)) setGitHubStatus(workspaceId, null);
      })
      .finally(() => {
        if (isCurrentRequest(workspaceId, version)) {
          setGitHubStatusLoading(workspaceId, false);
        }
      });
  }, [setGitHubStatus, setGitHubStatusLoading, workspaceId]);

  useEffect(() => {
    if (!workspaceId) return;
    if (!statusState) {
      resetGitHubStatus(workspaceId);
      doFetch();
      return;
    }
    if (!statusState.loaded && !statusState.loading) doFetch();
  }, [doFetch, resetGitHubStatus, statusState?.loaded, statusState?.loading, workspaceId]);

  useEffect(() => subscribeIntegrationAvailability(doFetch), [doFetch]);

  const refresh = useCallback(() => {
    invalidateSystemHealth();
    if (!workspaceId) return;
    resetGitHubStatus(workspaceId);
    doFetch();
  }, [doFetch, invalidateSystemHealth, resetGitHubStatus, workspaceId]);

  return {
    workspaceId,
    status: statusState?.status ?? null,
    loaded: statusState?.loaded ?? false,
    loading: statusState?.loading ?? false,
    refresh,
  };
}
