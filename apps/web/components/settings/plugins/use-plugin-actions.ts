"use client";

import { useState } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { StoreApi } from "zustand";
import { useAppStoreApi } from "@/components/state-provider";
import { useTheme } from "@/components/theme/app-theme";
import {
  disablePlugin,
  enablePlugin,
  installPluginFromUrl,
  installPluginUpload,
  listPlugins,
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
import { qk } from "@/lib/query/keys";

function upsertPlugin(queryClient: QueryClient, plugin: PluginRecord) {
  queryClient.setQueryData<PluginRecord[]>(qk.plugins.all(), (current = []) => {
    const index = current.findIndex((item) => item.id === plugin.id);
    if (index < 0) return [...current, plugin];
    return current.map((item, itemIndex) => (itemIndex === index ? plugin : item));
  });
}

function removePlugin(queryClient: QueryClient, pluginId: string) {
  queryClient.setQueryData<PluginRecord[]>(qk.plugins.all(), (current = []) =>
    current.filter((plugin) => plugin.id !== pluginId),
  );
}

function withStatus(plugin: PluginRecord, status: PluginStatus): PluginRecord {
  return { ...plugin, status };
}

/** Loads a plugin's UI bundle into the running app, if it declares one. */
async function loadIfActive(
  record: PluginRecord,
  storeApi: StoreApi<AppState>,
  theme: "light" | "dark",
) {
  if (record.status !== "active") return;
  const active = toActivePlugin(record);
  if (!active) return;
  await loadPlugins([active], (pluginId) => buildHostApi(pluginId, storeApi, theme));
}

/**
 * Enable/disable action wiring, per task-20's output contract: enabling a
 * plugin with a UI bundle calls the task-18 runtime `loadPlugins` for just
 * that plugin (no full page reload); disabling calls `unloadPlugin` to
 * revoke its nav items/routes/slots immediately.
 */
function useEnableDisableActions(queryClient: QueryClient) {
  const storeApi = useAppStoreApi();
  const { resolvedTheme } = useTheme();
  const [busyId, setBusyId] = useState<string | null>(null);

  const handleEnable = async (plugin: PluginRecord) => {
    setBusyId(plugin.id);
    try {
      await enablePlugin(plugin.id);
      const updated = withStatus(plugin, "active");
      upsertPlugin(queryClient, updated);
      await loadIfActive(updated, storeApi, resolvedTheme);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : `Failed to enable ${plugin.display_name}`);
    } finally {
      setBusyId(null);
    }
  };

  const handleDisable = async (plugin: PluginRecord) => {
    setBusyId(plugin.id);
    try {
      await disablePlugin(plugin.id);
      unloadPlugin(plugin.id);
      upsertPlugin(queryClient, withStatus(plugin, "disabled"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : `Failed to disable ${plugin.display_name}`);
    } finally {
      setBusyId(null);
    }
  };

  return { busyId, handleEnable, handleDisable };
}

function useUninstallAction(queryClient: QueryClient) {
  const [uninstallTarget, setUninstallTarget] = useState<PluginRecord | null>(null);
  const [uninstallBusy, setUninstallBusy] = useState(false);

  const confirmUninstall = async () => {
    if (!uninstallTarget) return;
    const target = uninstallTarget;
    setUninstallBusy(true);
    try {
      await uninstallPlugin(target.id);
      unloadPlugin(target.id);
      removePlugin(queryClient, target.id);
      setUninstallTarget(null);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : `Failed to uninstall ${target.display_name}`,
      );
    } finally {
      setUninstallBusy(false);
    }
  };

  return {
    uninstallTarget,
    uninstallBusy,
    openUninstall: setUninstallTarget,
    closeUninstall: () => setUninstallTarget(null),
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
function useInstallAction(queryClient: QueryClient) {
  const storeApi = useAppStoreApi();
  const { resolvedTheme } = useTheme();
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
    upsertPlugin(queryClient, plugin);
    await loadIfActive(plugin, storeApi, resolvedTheme);
    if (warning) {
      toast.warning(warning);
    } else {
      toast.success(`${plugin.display_name} installed`);
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
      setInstallError(err instanceof Error ? err.message : "Failed to install plugin");
    } finally {
      setInstallBusy(false);
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
 */
function useSyncAction(queryClient: QueryClient) {
  const [syncBusy, setSyncBusy] = useState(false);
  const [syncErrors, setSyncErrors] = useState<SyncError[]>([]);

  const handleSync = async () => {
    setSyncBusy(true);
    try {
      const result = await syncPlugins();
      const refreshed = await listPlugins({ cache: "no-store" });
      queryClient.setQueryData(qk.plugins.all(), refreshed);
      setSyncErrors(result.errors ?? []);
      toast.success(summarizeSyncResult(result));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to sync plugins");
    } finally {
      setSyncBusy(false);
    }
  };

  return { syncBusy, syncErrors, handleSync };
}

export function usePluginActions() {
  const queryClient = useQueryClient();
  const enableDisable = useEnableDisableActions(queryClient);
  const uninstall = useUninstallAction(queryClient);
  const install = useInstallAction(queryClient);
  const sync = useSyncAction(queryClient);

  return { ...enableDisable, ...uninstall, ...install, ...sync };
}
