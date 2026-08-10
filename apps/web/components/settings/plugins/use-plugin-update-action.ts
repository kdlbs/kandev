"use client";

import { useCallback, useState } from "react";
import { t } from "@/lib/i18n";
import type { MarketplaceEntry } from "@/lib/types/plugins";

/**
 * Owns the manual "Update" button's busy/error state machine, independent of
 * the auto-updater. `runUpdate` clears any previous failure for that plugin
 * first (so a retry doesn't show a stale error while it's pending), installs
 * the newer package via the shared `marketplaceInstall` (same path
 * install/enable use — never disagrees with the auto-updater's own
 * `install_state` comparison), and always re-checks the catalog afterward so
 * a resolved update's row converges within one round-trip whether the
 * install succeeded or failed.
 */
export function usePluginUpdateAction(
  marketplaceInstall: (url: string) => Promise<{ ok: boolean; error?: string }>,
  reloadUpdates: () => void,
) {
  const [updatingId, setUpdatingId] = useState<string | null>(null);
  const [errorsById, setErrorsById] = useState<Map<string, string>>(new Map());

  const clearError = useCallback((id: string) => {
    setErrorsById((prev) => {
      if (!prev.has(id)) return prev;
      const next = new Map(prev);
      next.delete(id);
      return next;
    });
  }, []);

  const runUpdate = useCallback(
    async (entry: MarketplaceEntry) => {
      clearError(entry.id);
      setUpdatingId(entry.id);
      try {
        const result = await marketplaceInstall(entry.package_url);
        if (!result.ok) {
          const message = result.error ?? t("plugins:failedToUpdatePlugin", { name: entry.name });
          setErrorsById((prev) => new Map(prev).set(entry.id, message));
        }
      } finally {
        setUpdatingId(null);
        reloadUpdates();
      }
    },
    [marketplaceInstall, reloadUpdates, clearError],
  );

  return { updatingId, errorsById, runUpdate };
}
