"use client";

import { useQuery } from "@tanstack/react-query";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import type { ProviderHealth } from "@/lib/state/slices/office/types";

export type UseProviderHealthResult = {
  health: ProviderHealth[];
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
};

const EMPTY_HEALTH: ProviderHealth[] = [];

export function useProviderHealth(workspaceName: string | null): UseProviderHealthResult {
  const { data, isLoading, error, refetch } = useQuery({
    ...officeQueryOptions.providerHealth(workspaceName ?? ""),
    enabled: !!workspaceName,
  });

  function toErrorMessage(e: unknown): string | null {
    if (!e) return null;
    return e instanceof Error ? e.message : "Failed to load provider health";
  }

  return {
    health: data ?? EMPTY_HEALTH,
    isLoading,
    error: toErrorMessage(error),
    refresh: async () => {
      await refetch();
    },
  };
}
