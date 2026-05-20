"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getProviderHealth } from "@/lib/api/domains/office-extended-api";
import type { ProviderHealth } from "@/lib/state/slices/office/types";

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
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fetched, setFetched] = useState(false);

  const refresh = useCallback(async () => {
    if (!workspaceName) return;
    setIsLoading(true);
    setError(null);
    try {
      const res = await getProviderHealth(workspaceName);
      setProviderHealth(workspaceName, res.health ?? []);
      setFetched(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load provider health");
    } finally {
      setIsLoading(false);
    }
  }, [workspaceName, setProviderHealth]);

  useEffect(() => {
    if (!workspaceName || fetched) return;
    void refresh();
  }, [workspaceName, fetched, refresh]);

  return { health, isLoading, error, refresh };
}
