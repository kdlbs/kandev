"use client";

import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { officeProviderHealthQueryOptions } from "@/lib/query/query-options/office";
import type { ProviderHealth } from "@/lib/state/slices/office/types";
import { queryErrorMessage } from "./query-error";

export type UseProviderHealthResult = {
  health: ProviderHealth[];
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
};

const EMPTY_HEALTH: ProviderHealth[] = [];

export function useProviderHealth(workspaceName: string | null): UseProviderHealthResult {
  const query = useQuery(officeProviderHealthQueryOptions(workspaceName ?? ""));

  const refresh = useCallback(async () => {
    if (!workspaceName) return;
    await query.refetch();
  }, [query, workspaceName]);

  const queryHealth = query.data?.health ?? EMPTY_HEALTH;
  const error = queryErrorMessage(query.error);

  return { health: queryHealth, isLoading: query.isPending, error, refresh };
}
