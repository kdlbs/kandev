/**
 * Boots the plugin host from the app's boot payload
 * (docs/plans/plugins/PLUGIN-API.md). Called once from the app
 * bootstrap once the store + boot payload are both available.
 */
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BootPayload } from "@/src/boot-payload";
import { buildHostApi } from "./host-api";
import { installPluginGlobal, loadPlugins } from "./host";

/** Store instances that have already had `loadPlugins` triggered — boot is a one-shot per store. */
const bootedStores = new WeakSet<StoreApi<AppState>>();

/** Reads the resolved light/dark theme applied to the document by AppThemeProvider. */
function readResolvedTheme(win: Window = window): "light" | "dark" {
  return win.document.documentElement.classList.contains("dark") ? "dark" : "light";
}

/**
 * Installs `window.registerKandevPlugin` and loads every plugin in
 * `bootPayload.plugins`, building each one's `PluginHostApi` from `storeApi`.
 *
 * No-op when `feature` (the `plugins` feature flag) is off, or when the boot
 * payload carries no plugins. Idempotent per `storeApi` instance — repeated
 * calls (re-renders, StrictMode double-invoke, HMR remounts of the calling
 * component) never re-install the global or re-import a bundle for a store
 * that has already booted.
 */
export function bootPlugins(
  bootPayload: Pick<BootPayload, "plugins">,
  storeApi: StoreApi<AppState>,
  feature: boolean,
): void {
  if (!feature) return;
  if (!bootPayload.plugins || bootPayload.plugins.length === 0) return;
  if (bootedStores.has(storeApi)) return;
  bootedStores.add(storeApi);

  installPluginGlobal();
  const theme = readResolvedTheme();
  void loadPlugins(bootPayload.plugins, (pluginId) => buildHostApi(pluginId, storeApi, theme));
}
