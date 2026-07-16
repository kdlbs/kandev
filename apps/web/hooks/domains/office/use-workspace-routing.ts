"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  getWorkspaceRouting,
  retryProvider,
  updateWorkspaceRouting,
} from "@/lib/api/domains/office-extended-api";
import type { ExecutionProfileSummary, WorkspaceRouting } from "@/lib/state/slices/office/types";

export type UseWorkspaceRoutingResult = {
  config: WorkspaceRouting | undefined;
  knownProviders: string[];
  executionProfiles: ExecutionProfileSummary[];
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  update: (cfg: WorkspaceRouting) => Promise<void>;
  retry: (providerId: string) => Promise<void>;
};

export function useWorkspaceRouting(workspaceName: string | null): UseWorkspaceRoutingResult {
  const config = useAppStore((s) =>
    workspaceName ? s.office.routing.byWorkspace[workspaceName] : undefined,
  );
  const knownProviders = useAppStore((s) => s.office.routing.knownProviders);
  const setWorkspaceRouting = useAppStore((s) => s.setWorkspaceRouting);
  const setKnownProviders = useAppStore((s) => s.setKnownProviders);
  const [isLoading, setIsLoading] = useState(false);
  const [executionProfiles, setExecutionProfiles] = useState<ExecutionProfileSummary[]>([]);
  const [profilesWorkspace, setProfilesWorkspace] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const requestVersion = useRef(0);

  const refresh = useCallback(async () => {
    if (!workspaceName) return;
    const version = ++requestVersion.current;
    setIsLoading(true);
    setError(null);
    try {
      const res = await getWorkspaceRouting(workspaceName);
      if (version !== requestVersion.current) return;
      if (res.config) setWorkspaceRouting(workspaceName, res.config);
      if (Array.isArray(res.known_providers)) setKnownProviders(res.known_providers);
      setExecutionProfiles(Array.isArray(res.execution_profiles) ? res.execution_profiles : []);
      setProfilesWorkspace(workspaceName);
    } catch (e) {
      if (version !== requestVersion.current) return;
      setError(e instanceof Error ? e.message : "Failed to load routing config");
    } finally {
      if (version === requestVersion.current) setIsLoading(false);
    }
  }, [workspaceName, setWorkspaceRouting, setKnownProviders]);

  useEffect(() => {
    if (!workspaceName) {
      requestVersion.current += 1;
      setExecutionProfiles([]);
      setProfilesWorkspace(null);
      setIsLoading(false);
      return;
    }
    if (profilesWorkspace === workspaceName) return;
    void refresh();
  }, [workspaceName, profilesWorkspace, refresh]);

  const update = useCallback(
    async (cfg: WorkspaceRouting) => {
      if (!workspaceName) return;
      await updateWorkspaceRouting(workspaceName, cfg);
      setWorkspaceRouting(workspaceName, cfg);
    },
    [workspaceName, setWorkspaceRouting],
  );

  const retry = useCallback(
    async (providerId: string) => {
      if (!workspaceName) return;
      await retryProvider(workspaceName, providerId);
    },
    [workspaceName],
  );

  return { config, knownProviders, executionProfiles, isLoading, error, refresh, update, retry };
}
