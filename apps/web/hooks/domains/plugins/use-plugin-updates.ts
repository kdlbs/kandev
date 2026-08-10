"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { getMarketplaceCatalog, refreshMarketplace } from "@/lib/api/domains/marketplace-api";
import { t } from "@/lib/i18n";
import type { MarketplaceCatalog, MarketplaceEntry } from "@/lib/types/plugins";

/**
 * Cross-references installed plugins against the marketplace catalog: which
 * ones have a catalog entry at all (`latestById`), and which of those are
 * strictly newer than the installed version (`updates`, the `latestById`
 * subset with `install_state === "update_available"`). Used by the Installed
 * tab to show each row's latest-known version and, where relevant, an Update
 * button.
 *
 * The catalog already computes `install_state` per entry server-side (the
 * backend joins the catalog against installed records by id+version), so this
 * hook only needs to bucket entries, never compare versions itself.
 *
 * A missing/offline marketplace must never break installed-plugin management:
 * a failed check sets `error` (surfaced by the caller) but never throws and
 * never clears previously-known `latestById` data. `checked` only flips to
 * `true` after the *first* successful check, so callers can tell "haven't
 * checked yet" apart from "checked and this plugin isn't in any catalog".
 */
export function usePluginUpdates() {
  const [latestById, setLatestById] = useState<Map<string, MarketplaceEntry>>(new Map());
  const [checking, setChecking] = useState(false);
  const [checked, setChecked] = useState(false);
  const [lastCheckedAt, setLastCheckedAt] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  const applyCatalog = useCallback((catalog: MarketplaceCatalog) => {
    const enabledSources = catalog.sources.filter((source) => source.enabled);
    const allUnhealthy =
      enabledSources.length > 0 && enabledSources.every((source) => source.healthy === false);
    if (allUnhealthy) {
      const reason = enabledSources.find((source) => source.error)?.error;
      setError(reason ?? t("plugins:updateCheckFailed"));
      return;
    }

    const next = new Map<string, MarketplaceEntry>();
    for (const plugin of catalog.plugins) {
      if (plugin.install_state === "installed" || plugin.install_state === "update_available") {
        next.set(plugin.id, plugin);
      }
    }
    setLatestById(next);
    setError(null);
    setChecked(true);
    setLastCheckedAt(new Date().toISOString());
  }, []);

  useEffect(() => {
    let cancelled = false;
    getMarketplaceCatalog()
      .then((catalog) => {
        if (!cancelled) applyCatalog(catalog);
      })
      .catch((err) => {
        if (!cancelled)
          setError(err instanceof Error ? err.message : t("plugins:updateCheckFailed"));
      });
    return () => {
      cancelled = true;
    };
  }, [reloadKey, applyCatalog]);

  const reload = useCallback(() => setReloadKey((key) => key + 1), []);

  const checkForUpdates = useCallback(async () => {
    setChecking(true);
    try {
      await refreshMarketplace().catch(() => undefined);
      const catalog = await getMarketplaceCatalog();
      applyCatalog(catalog);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("plugins:updateCheckFailed"));
    } finally {
      setChecking(false);
    }
  }, [applyCatalog]);

  const updates = useMemo(() => {
    const next = new Map<string, MarketplaceEntry>();
    for (const [id, plugin] of latestById) {
      if (plugin.install_state === "update_available") next.set(id, plugin);
    }
    return next;
  }, [latestById]);

  return { latestById, updates, checking, checked, lastCheckedAt, error, reload, checkForUpdates };
}
