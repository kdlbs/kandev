"use client";

import { useState } from "react";
import { toast } from "@/lib/toast/sonner";
// Module-level `t`: these callbacks fire outside a render, so each message
// resolves when the action runs rather than at import.
import { t } from "@/lib/i18n";
import type { StoreApi } from "zustand";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  disablePlugin,
  enablePlugin,
  getPlugin,
  installPluginFromUrl,
  installPluginUpload,
  listPlugins,
  setPluginAutoUpdate,
  syncPlugins,
  uninstallPlugin,
} from "@/lib/api/domains/plugins-api";
import { toActivePlugin } from "@/lib/plugins/active-plugin";
import { buildHostApi } from "@/lib/plugins/host-api";
import { loadPlugins, unloadPlugin } from "@/lib/plugins/host";
import { summarizeSyncResult } from "@/lib/plugins/sync-summary";
import type { InstallResult } from "@/lib/api/domains/plugins-api";
import type { PluginRecord, PluginStatus, SyncError } from "@/lib/types/plugins";
import type { AppState } from "@/lib/state/store";

function withStatus(plugin: PluginRecord, status: PluginStatus): PluginRecord {
  if (status === "active") {
    return { ...plugin, status, last_error: undefined, last_error_at: undefined };
  }
  return { ...plugin, status };
}

/**
 * Loads a plugin's UI bundle into the running app, if it declares one.
 * Unloads any previous registrations first — a first-time load is a safe
 * no-op through `unloadPlugin` since nothing is registered yet.
 *
 * `evictCache` controls whether the cached bundle registration is dropped
 * before reloading:
 * - Install/update (`afterInstall`) passes `evictCache: true`: the backend
 *   install endpoint is also how an update is applied, so without eviction an
 *   update would leave the prior version's nav/route/slot registrations in
 *   place alongside the new ones (e.g. a duplicated top-bar widget), and
 *   `loadPlugins` would skip re-importing the bundle and re-run the old
 *   version's `initialize()` against the new registry entry.
 * - Plain enable (`handleEnable`) passes `evictCache: false`: the disable path
 *   intentionally keeps the cached registration (see `unloadPlugin`'s doc) so
 *   a same-tab disable→enable cycle can reuse it without depending on the
 *   browser re-executing the bundle's module-eval side effect — the
 *   `bundleUrl` is unchanged, so a forced re-import would silently return the
 *   already-evaluated module without calling `registerKandevPlugin` again.
 */
async function loadIfActive(
  record: PluginRecord,
  storeApi: StoreApi<AppState>,
  evictCache: boolean,
) {
  if (record.status !== "active") return;
  const active = toActivePlugin(record);
  if (!active) return;
  if (evictCache) {
    unloadPlugin(record.id, { evictCache: true, transition: "reload" });
  } else {
    unloadPlugin(record.id, { transition: "reload" });
  }
  await loadPlugins([active], (pluginId) => buildHostApi(pluginId, storeApi));
}

/**
 * Enable/disable action wiring, per task-20's output contract: enabling a
 * plugin with a UI bundle calls the task-18 runtime `loadPlugins` for just
 * that plugin (no full page reload); disabling calls `unloadPlugin` to
 * revoke its nav items/routes/slots immediately.
 */
function useEnableDisableActions(upsertPlugin: (p: PluginRecord) => void) {
  const storeApi = useAppStoreApi();
  const [busyId, setBusyId] = useState<string | null>(null);

  const handleEnable = async (plugin: PluginRecord) => {
    setBusyId(plugin.id);
    try {
      await enablePlugin(plugin.id);
      const updated = withStatus(plugin, "active");
      upsertPlugin(updated);
      await loadIfActive(updated, storeApi, false);
    } catch (err) {
      try {
        const refreshed = await getPlugin(plugin.id, { cache: "no-store" });
        upsertPlugin(refreshed);
      } catch {
        // Preserve the original Enable failure toast if the diagnostic refresh
        // itself cannot reach the backend.
      }
      toast.error(
        err instanceof Error
          ? err.message
          : t("plugins:failedToEnable", { name: plugin.display_name }),
      );
    } finally {
      setBusyId(null);
    }
  };

  const handleDisable = async (plugin: PluginRecord) => {
    setBusyId(plugin.id);
    try {
      await disablePlugin(plugin.id);
      unloadPlugin(plugin.id);
      upsertPlugin(withStatus(plugin, "disabled"));
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
          : t("plugins:failedToDisable", { name: plugin.display_name }),
      );
    } finally {
      setBusyId(null);
    }
  };

  return { busyId, handleEnable, handleDisable };
}

/**
 * Per-plugin auto-update override wiring: PUT /api/plugins/:id/auto-update,
 * then upsert the returned record so the row's toggle reflects the new
 * override. `value` is `true`/`false` to force the plugin on/off, or `null` to
 * clear the override and inherit the instance-wide default again.
 */
function useAutoUpdateAction(upsertPlugin: (p: PluginRecord) => void) {
  const [autoUpdateBusyId, setAutoUpdateBusyId] = useState<string | null>(null);

  const handleSetAutoUpdate = async (plugin: PluginRecord, value: boolean | null) => {
    setAutoUpdateBusyId(plugin.id);
    try {
      const updated = await setPluginAutoUpdate(plugin.id, value);
      upsertPlugin(updated);
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
          : t("plugins:failedToSetAutoUpdateFor", { name: plugin.display_name }),
      );
    } finally {
      setAutoUpdateBusyId(null);
    }
  };

  return { autoUpdateBusyId, handleSetAutoUpdate };
}

function useUninstallAction(removePlugin: (id: string) => void) {
  const [uninstallBusy, setUninstallBusy] = useState(false);

  const confirmUninstall = async (target: PluginRecord) => {
    setUninstallBusy(true);
    try {
      await uninstallPlugin(target.id);
      unloadPlugin(target.id);
      removePlugin(target.id);
      return true;
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
          : t("plugins:failedToUninstall", { name: target.display_name }),
      );
      return false;
    } finally {
      setUninstallBusy(false);
    }
  };

  return {
    uninstallBusy,
    confirmUninstall,
  };
}

/**
 * Install-plugin dialog wiring. Install-from-URL and upload share the same
 * open/busy/error state and post-install effect: upsert the record into the
 * store, and if the backend already brought it up `active` with a UI
 * bundle, hot-load it into the running app (no full page reload) — same as
 * the enable path.
 */
function useInstallAction(upsertPlugin: (p: PluginRecord) => void) {
  const storeApi = useAppStoreApi();
  const [installOpen, setInstallOpenState] = useState(false);
  const [installBusy, setInstallBusy] = useState(false);
  const [installError, setInstallError] = useState<string | null>(null);

  const closeInstallDialog = () => {
    setInstallOpenState(false);
    setInstallError(null);
  };

  // A partial-install warning (package installed but failed to spawn — the
  // backend leaves Plugin.Status "error") must not be masked by a green
  // "installed" toast, so it takes priority over the success toast.
  const afterInstall = async ({ plugin, warning }: InstallResult) => {
    upsertPlugin(plugin);
    await loadIfActive(plugin, storeApi, true);
    if (warning) {
      toast.warning(warning);
    } else {
      toast.success(t("plugins:pluginInstalled", { name: plugin.display_name }));
    }
    closeInstallDialog();
  };

  const runInstall = async (install: () => Promise<InstallResult>) => {
    setInstallBusy(true);
    setInstallError(null);
    try {
      const result = await install();
      await afterInstall(result);
    } catch (err) {
      setInstallError(err instanceof Error ? err.message : t("plugins:failedToInstallPlugin"));
    } finally {
      setInstallBusy(false);
    }
  };

  // marketplaceInstall runs the same post-install effect as the dialog
  // (upsert + hot-load if active + success toast), but surfaces failures as a
  // toast rather than the dialog-scoped installError region — the Browse tab
  // has no such region. It resolves even on failure (after toasting) so its
  // fire-and-forget onClick callers never leak an unhandled rejection; their
  // try/finally still clears per-entry busy state. The resolved
  // `{ ok, error, pluginId }` lets callers that need the outcome (the manual
  // update action and the Browse tab) reconcile local update state without
  // also duplicating the toast.
  const marketplaceInstall = async (
    url: string,
  ): Promise<{ ok: boolean; error?: string; pluginId?: string }> => {
    try {
      const result = await installPluginFromUrl(url);
      await afterInstall(result);
      return { ok: true, pluginId: result.plugin.id };
    } catch (err) {
      const message = err instanceof Error ? err.message : t("plugins:failedToInstallPlugin");
      toast.error(message);
      return { ok: false, error: message };
    }
  };

  return {
    installOpen,
    openInstall: () => setInstallOpenState(true),
    setInstallOpen: (open: boolean) => (open ? setInstallOpenState(true) : closeInstallDialog()),
    installBusy,
    installError,
    submitInstallUrl: (url: string) => runInstall(() => installPluginFromUrl(url)),
    submitInstallFile: (file: File) => runInstall(() => installPluginUpload(file)),
    marketplaceInstall,
    closeInstallDialog,
  };
}

/**
 * Sync-button wiring: POST /api/plugins/sync, then refresh the plugin list
 * via the same GET /api/plugins call usePlugins itself makes on mount, and
 * summarize the result as a toast. result.errors are kept in state for an
 * inline `plugins-sync-errors` region rather than only living in the toast,
 * so they stay visible without depending on toast auto-dismiss timing.
 *
 * A sync can bring a dropped tarball plugin all the way to `active` with a
 * UI bundle, but (unlike install/enable) this does not hot-load it — an
 * operator can re-enable it (or reload) to pick up the bundle; wiring a
 * silent hot-load here is out of scope for the sync button itself.
 *
 * `handleSync` resolves `{ ok }` (never throws) so a caller that chains a
 * marketplace update check after the sync (plugins-settings.tsx) can skip
 * that second request when the sync itself already failed — avoiding two
 * stacked error toasts for what's likely one unreachable-backend root cause.
 */
function useSyncAction(setPlugins: (plugins: PluginRecord[]) => void) {
  const [syncBusy, setSyncBusy] = useState(false);
  const [syncErrors, setSyncErrors] = useState<SyncError[]>([]);

  const handleSync = async (): Promise<{ ok: boolean }> => {
    setSyncBusy(true);
    try {
      const result = await syncPlugins();
      const refreshed = await listPlugins({ cache: "no-store" });
      setPlugins(refreshed);
      setSyncErrors(result.errors ?? []);
      toast.success(summarizeSyncResult(result));
      return { ok: true };
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("plugins:failedToSyncPlugins"));
      return { ok: false };
    } finally {
      setSyncBusy(false);
    }
  };

  return { syncBusy, syncErrors, handleSync };
}

export function usePluginActions() {
  const upsertPlugin = useAppStore((s) => s.upsertPlugin);
  const removePlugin = useAppStore((s) => s.removePlugin);
  const setPlugins = useAppStore((s) => s.setPlugins);

  const enableDisable = useEnableDisableActions(upsertPlugin);
  const autoUpdate = useAutoUpdateAction(upsertPlugin);
  const uninstall = useUninstallAction(removePlugin);
  const install = useInstallAction(upsertPlugin);
  const sync = useSyncAction(setPlugins);

  return { ...enableDisable, ...autoUpdate, ...uninstall, ...install, ...sync };
}
