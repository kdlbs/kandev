"use client";

import { useCallback, useEffect, useState } from "react";
import { getMarketplaceCatalog } from "@/lib/api/domains/marketplace-api";
import type { MarketplaceEntry } from "@/lib/types/plugins";

/**
 * Cross-references installed plugins against the marketplace catalog to find
 * which have a newer version available, keyed by plugin id. Used by the
 * Installed tab to show an Update button on rows with a pending update.
 *
 * The catalog already computes install_state per entry (the backend joins the
 * catalog against installed records by id+version), so this hook just keeps the
 * `update_available` entries. Failures are swallowed — a missing/offline
 * marketplace must never break installed-plugin management; it just means no
 * update badges are shown.
 */
export function usePluginUpdates() {
  const [updates, setUpdates] = useState<Map<string, MarketplaceEntry>>(new Map());
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    getMarketplaceCatalog()
      .then((catalog) => {
        if (cancelled) return;
        const next = new Map<string, MarketplaceEntry>();
        for (const entry of catalog.plugins) {
          if (entry.install_state === "update_available") next.set(entry.id, entry);
        }
        setUpdates(next);
      })
      .catch(() => {
        if (!cancelled) setUpdates(new Map());
      });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const reload = useCallback(() => setReloadKey((key) => key + 1), []);

  return { updates, reload };
}
