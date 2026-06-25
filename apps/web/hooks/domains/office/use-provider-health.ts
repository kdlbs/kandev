"use client";

import { useCallback, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
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
  const health = useAppStore((s) =>
    workspaceName
      ? (s.office.providerHealth.byWorkspace[workspaceName] ?? EMPTY_HEALTH)
      : EMPTY_HEALTH,
  );
  const setProviderHealth = useAppStore((s) => s.setProviderHealth);
  const query = useQuery(officeProviderHealthQueryOptions(workspaceName ?? ""));

  const refresh = useCallback(async () => {
    if (!workspaceName) return;
    await query.refetch();
  }, [query, workspaceName]);

  useEffect(() => {
    if (!workspaceName || !query.data) return;
    setProviderHealth(workspaceName, query.data.health ?? []);
  }, [query.data, setProviderHealth, workspaceName]);

  const queryHealth = query.data?.health ?? health;
  const error = queryErrorMessage(query.error);

  return { health: queryHealth, isLoading: query.isPending, error, refresh };
}
