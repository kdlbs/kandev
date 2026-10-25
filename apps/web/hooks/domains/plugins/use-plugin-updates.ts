"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { getMarketplaceCatalog, refreshMarketplace } from "@/lib/api/domains/marketplace-api";
import { t } from "@/lib/i18n";
import type { MarketplaceCatalog, MarketplaceEntry } from "@/lib/types/plugins";

/**
 * The message shown when a check couldn't complete: the thrown error's own text
 * when there is one, otherwise the generic fallback. A function, not a module
 * constant — `t()` at module scope would freeze at the boot locale.
 */
function checkFailedMessage(err?: unknown): string {
  return err instanceof Error ? err.message : t("plugins:updateCheckFailed");
}

async function runMarketplaceCheck(
  check: number,
  checkGeneration: { current: number },
  applyCatalog: (catalog: MarketplaceCatalog) => void,
  setChecking: (checking: boolean) => void,
  setError: (error: string | null) => void,
): Promise<boolean> {
  try {
    await refreshMarketplace();
    if (check !== checkGeneration.current) return false;
    const catalog = await getMarketplaceCatalog();
    if (check !== checkGeneration.current) return false;
    applyCatalog(catalog);
    return true;
  } catch (err) {
    if (check === checkGeneration.current) setError(checkFailedMessage(err));
    return false;
  } finally {
    if (check === checkGeneration.current) setChecking(false);
  }
}

async function waitForMarketplaceCheck(checkInFlight: {
  current: Promise<boolean> | null;
}): Promise<boolean> {
  while (checkInFlight.current) {
    const inFlight = checkInFlight.current;
    const succeeded = await inFlight;
    if (!succeeded && (checkInFlight.current === inFlight || checkInFlight.current === null))
      return false;
  }
  return true;
}

function markEntryInstalled(
  entries: Map<string, MarketplaceEntry>,
  pluginId: string,
): Map<string, MarketplaceEntry> {
  const current = entries.get(pluginId);
  if (!current || current.install_state !== "update_available") return entries;
  return new Map(entries).set(pluginId, { ...current, install_state: "installed" });
}

function trackMarketplaceCheck(
  run: Promise<boolean>,
  checkInFlight: { current: Promise<boolean> | null },
  lastCheckFailed: { current: boolean },
) {
  void run.then(
    (succeeded) => {
      if (checkInFlight.current !== run) return;
      checkInFlight.current = null;
      lastCheckFailed.current = !succeeded;
    },
    () => {
      if (checkInFlight.current !== run) return;
      checkInFlight.current = null;
      lastCheckFailed.current = true;
    },
  );
}

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
  const [sourcesDegraded, setSourcesDegraded] = useState(false);
  const requestGeneration = useRef(0);
  const checkGeneration = useRef(0);
  const checkInFlight = useRef<Promise<boolean> | null>(null);
  const lastCheckFailed = useRef(false);

  const applyCatalog = useCallback((catalog: MarketplaceCatalog) => {
    const enabledSources = catalog.sources.filter((source) => source.enabled);
    const unhealthy = enabledSources.filter((source) => source.healthy === false);
    // A degraded source may not carry an error string; fall back to the generic
    // message so the strip never renders an empty explanation.
    const reason = () => unhealthy.find((source) => source.error)?.error ?? checkFailedMessage();
    if (enabledSources.length > 0 && unhealthy.length === enabledSources.length) {
      setSourcesDegraded(true);
      setError(reason());
      return;
    }

    const next = new Map<string, MarketplaceEntry>();
    for (const plugin of catalog.plugins) {
      if (plugin.install_state === "installed" || plugin.install_state === "update_available") {
        next.set(plugin.id, plugin);
      }
    }
    setLatestById(next);
    setChecked(true);
    setLastCheckedAt(new Date().toISOString());

    // With no enabled source there was nothing to query, so this response is
    // not evidence that anything was delisted — it is evidence of nothing at
    // all. Flag it degraded (rows stay silent instead of claiming removal) and
    // say so, since unlike an unreachable source this is something the
    // operator turned off and can turn back on.
    if (enabledSources.length === 0) {
      setSourcesDegraded(true);
      setError(t("plugins:noEnabledSources"));
      return;
    }

    // A partially degraded catalog is a success for the sources that answered
    // and a non-answer for the ones that didn't. Report the degraded source so
    // the operator knows the picture is incomplete, and set `sourcesDegraded`
    // so a row missing from this response is shown as unknown rather than as
    // "not in the marketplace" — the backend omits a failing source's entries
    // entirely, which is indistinguishable from delisting without this flag.
    setSourcesDegraded(unhealthy.length > 0);
    setError(unhealthy.length > 0 ? reason() : null);
  }, []);

  const markUpdated = useCallback(
    (pluginId: string) => setLatestById((previous) => markEntryInstalled(previous, pluginId)),
    [],
  );

  const reload = useCallback(
    async (installedPluginId?: string) => {
      const checkSucceeded = await waitForMarketplaceCheck(checkInFlight);
      if (!checkSucceeded || lastCheckFailed.current) {
        if (installedPluginId) markUpdated(installedPluginId);
        return;
      }

      const generation = ++requestGeneration.current;
      try {
        const catalog = await getMarketplaceCatalog();
        if (generation === requestGeneration.current) applyCatalog(catalog);
      } catch (err) {
        if (generation === requestGeneration.current) setError(checkFailedMessage(err));
      }
    },
    [applyCatalog, markUpdated],
  );

  useEffect(() => {
    void reload();
    return () => {
      requestGeneration.current += 1;
      checkGeneration.current += 1;
    };
  }, [reload]);

  const checkForUpdates = useCallback(() => {
    // Invalidate any older reload. Reloads that begin while this pair is in
    // flight wait for it, so they cannot steal the post-refresh GET.
    ++requestGeneration.current;
    const check = ++checkGeneration.current;
    lastCheckFailed.current = false;
    setChecking(true);
    const run = runMarketplaceCheck(check, checkGeneration, applyCatalog, setChecking, setError);
    checkInFlight.current = run;
    trackMarketplaceCheck(run, checkInFlight, lastCheckFailed);
    return run;
  }, [applyCatalog]);

  const updates = useMemo(() => {
    const next = new Map<string, MarketplaceEntry>();
    for (const [id, plugin] of latestById) {
      if (plugin.install_state === "update_available") next.set(id, plugin);
    }
    return next;
  }, [latestById]);

  return {
    latestById,
    updates,
    checking,
    checked,
    sourcesDegraded,
    lastCheckedAt,
    error,
    reload,
    checkForUpdates,
    markUpdated,
  };
}
