"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { updateWorkspaceRouting, retryProvider } from "@/lib/api/domains/office-routing-api";
import { officeQueryOptions } from "@/lib/query/query-options/office";
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
  const qc = useQueryClient();
  const routingKey = ["office", workspaceName ?? "", "routing"] as const;

  const { data, isLoading, error, refetch } = useQuery({
    ...officeQueryOptions.workspaceRouting(workspaceName ?? ""),
    enabled: !!workspaceName,
  });

  const updateMutation = useMutation({
    mutationFn: (cfg: WorkspaceRouting) => updateWorkspaceRouting(workspaceName!, cfg),
    onSuccess: (_result, cfg) => {
      // Optimistically patch the cache so the UI updates instantly.
      qc.setQueryData(routingKey, (prev: typeof data) =>
        prev ? { ...prev, config: cfg } : { config: cfg, known_providers: [] },
      );
    },
  });

  const retryMutation = useMutation({
    mutationFn: (providerId: string) => retryProvider(workspaceName!, providerId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: routingKey });
    },
  });

  function toErrorMessage(e: unknown): string | null {
    if (!e) return null;
    return e instanceof Error ? e.message : "Failed to load routing config";
  }

  return {
    config: data?.config ?? undefined,
    knownProviders: data?.known_providers ?? [],
    isLoading,
    error: toErrorMessage(error),
    refresh: async () => {
      await refetch();
    },
    update: async (cfg: WorkspaceRouting) => {
      await updateMutation.mutateAsync(cfg);
    },
    retry: async (providerId: string) => {
      await retryMutation.mutateAsync(providerId);
    },
  };
}
