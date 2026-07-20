"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import { officeRoutingQueryOptions } from "@/lib/query/query-options/office";
import { retryProvider, updateWorkspaceRouting } from "@/lib/api/domains/office-extended-api";
import type { ExecutionProfileSummary, WorkspaceRouting } from "@/lib/state/slices/office/types";
import { queryErrorMessage } from "./query-error";

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
  const queryClient = useQueryClient();
  const query = useQuery(officeRoutingQueryOptions(workspaceName ?? ""));

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
        known_providers: query.data?.known_providers ?? [],
        execution_profiles: query.data?.execution_profiles ?? [],
      });
    },
    [query.data?.execution_profiles, query.data?.known_providers, queryClient, workspaceName],
  );

  const retry = useCallback(
    async (providerId: string) => {
      if (!workspaceName) return;
      await retryProvider(workspaceName, providerId);
    },
    [workspaceName],
  );

  const config = query.data?.config ?? undefined;
  const knownProviders = query.data?.known_providers ?? [];
  const executionProfiles = query.data?.execution_profiles ?? [];
  const error = queryErrorMessage(query.error);

  return {
    config,
    knownProviders,
    executionProfiles,
    isLoading: query.isPending,
    error,
    refresh,
    update,
    retry,
  };
}
