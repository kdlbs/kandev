"use client";

import { useQuery } from "@tanstack/react-query";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import type { AgentRoutePreview } from "@/lib/state/slices/office/types";

export type UseRoutingPreviewResult = {
  agents: AgentRoutePreview[];
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
};

const EMPTY_PREVIEW: AgentRoutePreview[] = [];

export function useRoutingPreview(workspaceName: string | null): UseRoutingPreviewResult {
  const { data, isLoading, error, refetch } = useQuery({
    ...officeQueryOptions.routingPreview(workspaceName ?? ""),
    enabled: !!workspaceName,
  });

  function toErrorMessage(e: unknown): string | null {
    if (!e) return null;
    return e instanceof Error ? e.message : "Failed to load routing preview";
  }

  return {
    agents: data ?? EMPTY_PREVIEW,
    isLoading,
    error: toErrorMessage(error),
    refresh: async () => {
      await refetch();
    },
  };
}
