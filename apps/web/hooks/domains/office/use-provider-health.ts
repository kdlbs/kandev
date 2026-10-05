"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getProviderHealth } from "@/lib/api/domains/office-extended-api";
import type { ProviderHealth } from "@/lib/state/slices/office/types";
import { t } from "@/lib/i18n";

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
  const activeRefresh = useRef<(() => Promise<void>) | null>(null);
  const requestVersion = useRef(0);

  const refresh = useCallback(async (): Promise<void> => {
    if (!workspaceName || activeRefresh.current !== refresh) return;
    const version = ++requestVersion.current;
    const isCurrent = () => activeRefresh.current === refresh && requestVersion.current === version;
    setIsLoading(true);
    setError(null);
    try {
      const res = await getProviderHealth(workspaceName);
      if (!isCurrent()) return;
      setProviderHealth(workspaceName, res.health ?? []);
    } catch (e) {
      if (!isCurrent()) return;
      setError(e instanceof Error ? e.message : t("office:failedToLoadProviderHealth"));
    } finally {
      if (isCurrent()) setIsLoading(false);
    }
  }, [workspaceName, setProviderHealth]);

  useLayoutEffect(() => {
    activeRefresh.current = refresh;
    setIsLoading(false);
    setError(null);
    return () => {
      activeRefresh.current = null;
      requestVersion.current++;
    };
  }, [refresh]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { health, isLoading, error, refresh };
}
