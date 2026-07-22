"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  deleteGitHubAppRegistration,
  fetchGitHubAppRegistrations,
  importGitHubAppRegistration,
  prepareGitHubAppImport,
  renameGitHubAppRegistration,
  startGitHubAppInstall,
  startGitHubAppManifest,
} from "@/lib/api/domains/github-api";
import type {
  GitHubCallbackResult,
  ImportGitHubAppRegistrationRequest,
  PrepareGitHubAppImportRequest,
  StartGitHubAppManifestRequest,
} from "@/lib/types/github";

const requestVersions = new Map<string, number>();

function nextRequestVersion(workspaceId: string) {
  const version = (requestVersions.get(workspaceId) ?? 0) + 1;
  requestVersions.set(workspaceId, version);
  return version;
}

function isCurrentRequest(workspaceId: string, version: number) {
  return requestVersions.get(workspaceId) === version;
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "GitHub App registrations are unavailable";
}

export function parseGitHubCallbackResult(
  search: URLSearchParams,
  workspaceId: string,
): GitHubCallbackResult | null {
  const code = search.get("github_result")?.trim();
  if (!code) return null;
  const callbackWorkspaceId = search.get("workspace_id")?.trim();
  if (callbackWorkspaceId && callbackWorkspaceId !== workspaceId) return null;
  return { code, ...(callbackWorkspaceId ? { workspace_id: callbackWorkspaceId } : {}) };
}

export function useGitHubAppRegistrations(workspaceId: string) {
  const entry = useAppStore((state) => state.githubAppRegistrations.byWorkspaceId[workspaceId]);
  const reset = useAppStore((state) => state.resetGitHubAppRegistrations);
  const setCatalog = useAppStore((state) => state.setGitHubAppRegistrations);
  const setLoading = useAppStore((state) => state.setGitHubAppRegistrationsLoading);
  const [mutating, setMutating] = useState(false);
  const workspaceVersion = useRef(0);

  const load = useCallback(
    async (signal?: AbortSignal) => {
      const version = nextRequestVersion(workspaceId);
      setLoading(workspaceId, true);
      try {
        const catalog = await fetchGitHubAppRegistrations(workspaceId, {
          init: signal ? { signal } : undefined,
        });
        if (!signal?.aborted && isCurrentRequest(workspaceId, version)) {
          setCatalog(workspaceId, catalog);
        }
      } catch (error) {
        if (!signal?.aborted && isCurrentRequest(workspaceId, version)) {
          setCatalog(workspaceId, null, errorMessage(error));
        }
      } finally {
        if (isCurrentRequest(workspaceId, version)) setLoading(workspaceId, false);
      }
    },
    [setCatalog, setLoading, workspaceId],
  );

  useEffect(() => {
    workspaceVersion.current += 1;
    setMutating(false);
    reset(workspaceId);
    void load();
  }, [load, reset, workspaceId]);

  const refresh = useCallback(async () => {
    reset(workspaceId);
    await load();
  }, [load, reset, workspaceId]);

  const mutate = useCallback(
    async <T>(operation: () => Promise<T>, refreshAfter: boolean): Promise<T> => {
      const version = workspaceVersion.current;
      setMutating(true);
      try {
        const result = await operation();
        if (refreshAfter) await load();
        return result;
      } finally {
        if (workspaceVersion.current === version) setMutating(false);
      }
    },
    [load],
  );

  const startManifest = useCallback(
    (request: Omit<StartGitHubAppManifestRequest, "workspace_id">) =>
      mutate(() => startGitHubAppManifest({ ...request, workspace_id: workspaceId }), false),
    [mutate, workspaceId],
  );
  const importRegistration = useCallback(
    (request: Omit<ImportGitHubAppRegistrationRequest, "workspace_id">) =>
      mutate(() => importGitHubAppRegistration({ ...request, workspace_id: workspaceId }), true),
    [mutate, workspaceId],
  );
  const prepareImport = useCallback(
    (request: Omit<PrepareGitHubAppImportRequest, "workspace_id">) =>
      mutate(() => prepareGitHubAppImport({ ...request, workspace_id: workspaceId }), false),
    [mutate, workspaceId],
  );
  const rename = useCallback(
    (registrationId: string, displayName: string) =>
      mutate(() => renameGitHubAppRegistration(registrationId, displayName), true),
    [mutate],
  );
  const remove = useCallback(
    (registrationId: string) => mutate(() => deleteGitHubAppRegistration(registrationId), true),
    [mutate],
  );
  const startInstall = useCallback(
    (registrationId: string) =>
      mutate(() => startGitHubAppInstall(workspaceId, registrationId), false),
    [mutate, workspaceId],
  );

  const catalog = entry?.catalog ?? null;
  return {
    workspaceId,
    catalog,
    registrations: catalog?.registrations ?? [],
    selected: catalog?.registrations.find((registration) => registration.selected) ?? null,
    loaded: entry?.loaded ?? false,
    loading: entry?.loading ?? true,
    error: entry?.error ?? null,
    mutating,
    refresh,
    startManifest,
    prepareImport,
    importRegistration,
    rename,
    remove,
    startInstall,
  } as const;
}
