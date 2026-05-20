"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  getWorkspaceRouting,
  retryProvider,
  updateWorkspaceRouting,
} from "@/lib/api/domains/office-extended-api";
import type { WorkspaceRouting } from "@/lib/state/slices/office/types";

export type UseWorkspaceRoutingResult = {
  config: WorkspaceRouting | undefined;
  knownProviders: string[];
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
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!workspaceName) return;
    setIsLoading(true);
    setError(null);
    try {
      const res = await getWorkspaceRouting(workspaceName);
      if (res.config) setWorkspaceRouting(workspaceName, res.config);
      if (Array.isArray(res.known_providers)) setKnownProviders(res.known_providers);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load routing config");
    } finally {
      setIsLoading(false);
    }
  }, [workspaceName, setWorkspaceRouting, setKnownProviders]);

  useEffect(() => {
    if (!workspaceName) return;
    if (config !== undefined) return;
    void refresh();
  }, [workspaceName, config, refresh]);

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

  return { config, knownProviders, isLoading, error, refresh, update, retry };
}
