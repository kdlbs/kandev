"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
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

function healthKey(row: ProviderHealth): string {
  return JSON.stringify([row.provider_id, row.scope, row.scope_value]);
}

function reconcileHealth(
  snapshot: ProviderHealth[],
  atStart: ProviderHealth[],
  current: ProviderHealth[],
): ProviderHealth[] {
  const previous = new Map(atStart.map((row) => [healthKey(row), row]));
  const changed = new Map(
    current
      .filter((row) => row !== previous.get(healthKey(row)))
      .map((row) => [healthKey(row), row]),
  );
  const keys = new Set(snapshot.map(healthKey));
  return [
    ...snapshot.map((row) => changed.get(healthKey(row)) ?? row),
    ...current.filter((row) => changed.has(healthKey(row)) && !keys.has(healthKey(row))),
  ];
}

export function useProviderHealth(workspaceName: string | null): UseProviderHealthResult {
  const store = useAppStoreApi();
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
    const atStart =
      store.getState().office.providerHealth.byWorkspace[workspaceName] ?? EMPTY_HEALTH;
    setIsLoading(true);
    setError(null);
    try {
      const res = await getProviderHealth(workspaceName);
      if (!isCurrent()) return;
      const current =
        store.getState().office.providerHealth.byWorkspace[workspaceName] ?? EMPTY_HEALTH;
      setProviderHealth(workspaceName, reconcileHealth(res.health ?? [], atStart, current));
    } catch (e) {
      if (!isCurrent()) return;
      setError(e instanceof Error ? e.message : t("office:failedToLoadProviderHealth"));
    } finally {
      if (isCurrent()) setIsLoading(false);
    }
  }, [workspaceName, setProviderHealth, store]);

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
