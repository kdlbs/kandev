"use client";

import { useCallback, useEffect, useState } from "react";
import {
  getMarketplaceCatalog,
  refreshMarketplace,
  type CatalogQuery,
} from "@/lib/api/domains/marketplace-api";
import type { MarketplaceCatalog } from "@/lib/types/plugins";

const EMPTY_CATALOG: MarketplaceCatalog = { plugins: [], sources: [] };

/**
 * Fetches the marketplace catalog for the given query, re-fetching whenever
 * the query changes. `reload` force-refreshes the backend source cache before
 * re-fetching (use after adding/removing a source); `softReload` re-fetches
 * without dropping the cache (use after an install, when only install_state
 * changed). The catalog is a transient browse-time cache — it lives in local
 * hook state, not the global store.
 */
export function useMarketplace(query: CatalogQuery) {
  const [catalog, setCatalog] = useState<MarketplaceCatalog>(EMPTY_CATALOG);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  const { q, category, sort } = query;

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    getMarketplaceCatalog({ q, category, sort })
      .then((result) => {
        if (cancelled) return;
        setCatalog(result);
        setError(null);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "Failed to load marketplace");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [q, category, sort, reloadKey]);

  const softReload = useCallback(() => setReloadKey((key) => key + 1), []);

  const reload = useCallback(async () => {
    await refreshMarketplace().catch(() => undefined);
    setReloadKey((key) => key + 1);
  }, []);

  return { catalog, loading, error, reload, softReload };
}
