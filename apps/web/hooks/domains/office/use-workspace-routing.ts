"use client";

import { useCallback, useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import { officeRoutingQueryOptions } from "@/lib/query/query-options/office";
import { retryProvider, updateWorkspaceRouting } from "@/lib/api/domains/office-extended-api";
import type { WorkspaceRouting } from "@/lib/state/slices/office/types";
import { queryErrorMessage } from "./query-error";

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
  const queryClient = useQueryClient();
  const storeConfig = useAppStore((s) =>
    workspaceName ? s.office.routing.byWorkspace[workspaceName] : undefined,
  );
  const storeKnownProviders = useAppStore((s) => s.office.routing.knownProviders);
  const setWorkspaceRouting = useAppStore((s) => s.setWorkspaceRouting);
  const setKnownProviders = useAppStore((s) => s.setKnownProviders);
  const query = useQuery(officeRoutingQueryOptions(workspaceName ?? ""));

  useEffect(() => {
    if (!workspaceName || !query.data) return;
    if (query.data.config) setWorkspaceRouting(workspaceName, query.data.config);
    if (Array.isArray(query.data.known_providers)) setKnownProviders(query.data.known_providers);
  }, [query.data, setKnownProviders, setWorkspaceRouting, workspaceName]);

  const refresh = useCallback(async () => {
    if (!workspaceName) return;
    await query.refetch();
  }, [query, workspaceName]);

  const update = useCallback(
    async (cfg: WorkspaceRouting) => {
      if (!workspaceName) return;
      await updateWorkspaceRouting(workspaceName, cfg);
      queryClient.setQueryData(qk.office.routing(workspaceName), {
        config: cfg,
        known_providers: storeKnownProviders,
      });
      setWorkspaceRouting(workspaceName, cfg);
    },
    [queryClient, storeKnownProviders, workspaceName, setWorkspaceRouting],
  );

  const retry = useCallback(
    async (providerId: string) => {
      if (!workspaceName) return;
      await retryProvider(workspaceName, providerId);
    },
    [workspaceName],
  );

  const config = query.data?.config ?? storeConfig;
  const knownProviders = query.data?.known_providers ?? storeKnownProviders;
  const error = queryErrorMessage(query.error);

  return { config, knownProviders, isLoading: query.isPending, error, refresh, update, retry };
}
