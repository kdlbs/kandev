"use client";

import { useCallback, useEffect, useState } from "react";
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
 * install succeeded or failed. A successful install also marks the catalog
 * entry current before that re-check, so a failed catalog request cannot leave
 * an already-installed version looking updateable.
 *
 * `installedIds` is the id set of what is currently installed. A failure is a
 * fact about one installed copy of a plugin, so it is dropped as soon as that
 * plugin leaves the list — otherwise uninstalling a plugin whose update failed
 * and installing it again (same id, no page reload) would surface the previous
 * copy's error on the new, perfectly healthy row.
 */
export function usePluginUpdateAction(
  marketplaceInstall: (url: string) => Promise<{ ok: boolean; error?: string }>,
  reloadUpdates: () => Promise<void>,
  installedIds: ReadonlySet<string>,
  markUpdated?: (pluginId: string) => void,
) {
  // A set, not a single id: rows update independently, so two installs can be
  // in flight at once. With one slot the first to settle cleared the marker for
  // the other, dropping a still-installing row's spinner and re-enabling its
  // Update/Uninstall controls mid-install.
  const [updatingIds, setUpdatingIds] = useState<ReadonlySet<string>>(new Set());
  const [errorsById, setErrorsById] = useState<Map<string, string>>(new Map());

  useEffect(() => {
    setErrorsById((prev) => {
      if (prev.size === 0) return prev;
      const next = new Map(prev);
      for (const id of prev.keys()) {
        if (!installedIds.has(id)) next.delete(id);
      }
      return next.size === prev.size ? prev : next;
    });
  }, [installedIds]);

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
      setUpdatingIds((prev) => new Set(prev).add(entry.id));
      try {
        const result = await marketplaceInstall(entry.package_url);
        if (!result.ok) {
          const message = result.error ?? t("plugins:failedToUpdatePlugin", { name: entry.name });
          setErrorsById((prev) => new Map(prev).set(entry.id, message));
        } else {
          markUpdated?.(entry.id);
        }
      } finally {
        try {
          await reloadUpdates();
        } finally {
          setUpdatingIds((prev) => {
            if (!prev.has(entry.id)) return prev;
            const next = new Set(prev);
            next.delete(entry.id);
            return next;
          });
        }
      }
    },
    [marketplaceInstall, reloadUpdates, clearError, markUpdated],
  );

  return { updatingIds, errorsById, runUpdate };
}
