"use client";

import { useEffect } from "react";
import { listPlugins } from "@/lib/api/domains/plugins-api";
import { useAppStore } from "@/components/state-provider";

/**
 * Loads the plugin registry into the store on mount. Mirrors
 * hooks/domains/settings/use-secrets.ts: the slice only holds
 * setPlugins/setPluginsLoading/setPluginsError — the fetch + dispatch lives
 * here, per the "Never fetch data directly in components" data-flow rule.
 */
export function usePlugins() {
  const items = useAppStore((state) => state.plugins.items);
  const loaded = useAppStore((state) => state.plugins.loaded);
  const loading = useAppStore((state) => state.plugins.loading);
  const error = useAppStore((state) => state.plugins.error);
  const setPlugins = useAppStore((state) => state.setPlugins);
  const setPluginsLoading = useAppStore((state) => state.setPluginsLoading);
  const setPluginsError = useAppStore((state) => state.setPluginsError);

  useEffect(() => {
    if (loading) return;
    setPluginsLoading(true);
    listPlugins({ cache: "no-store" })
      .then((response) => {
        setPlugins(response);
      })
      .catch((err) => {
        setPluginsError(err instanceof Error ? err.message : "Failed to load plugins");
      })
      .finally(() => {
        setPluginsLoading(false);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { items, loaded, loading, error };
}
