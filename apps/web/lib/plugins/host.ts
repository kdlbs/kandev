/**
 * Plugin host: the `window.registerKandevPlugin` global + the loader that
 * imports plugin bundles from the boot payload (docs/plans/plugins/PLUGIN-API.md).
 *
 * Loading sequence per bundle: inject `styleUrls` as `<link>` tags, dynamically
 * `import(/* @vite-ignore *\/ bundleUrl)` the bundle (module-level side effect
 * calls `window.registerKandevPlugin`), then call the registered plugin's
 * `initialize(registry, host)`. A bad plugin (throwing bundle, missing
 * registration, or throwing `initialize`) is logged and swallowed — it never
 * breaks boot or blocks other plugins.
 *
 * `registeredPlugins` is never cleared on disable — only on a fresh import
 * (see `resolveRegistration`). The browser's ES module cache means a repeat
 * `import(bundleUrl)` after disable would resolve without re-running the
 * bundle's top-level `registerKandevPlugin` call, so re-enabling in the same
 * tab must reuse the cached registration instead of relying on re-import.
 */
import { getBackendConfig } from "@/lib/config";
import { pluginRegistry } from "./registry";
import type { ActivePlugin, KandevPlugin, PluginHostApi } from "./types";

/** Builds the per-plugin `PluginHostApi` for a given pluginId. */
export type PluginHostFactory = (pluginId: string) => PluginHostApi;

/** Injectable bundle loader — defaults to a real dynamic import. Tests pass a fake. */
export type BundleImporter = (url: string) => Promise<unknown>;

const defaultImporter: BundleImporter = (url) => import(/* @vite-ignore */ url);

type PluginGlobalWindow = Window & {
  registerKandevPlugin?: (id: string, plugin: KandevPlugin) => void;
};

/** Bundles registered via `window.registerKandevPlugin`, keyed by pluginId. */
const registeredPlugins = new Map<string, KandevPlugin>();

/** Defines `window.registerKandevPlugin` before any bundle loads. Idempotent. */
export function installPluginGlobal(win: Window = window): void {
  (win as PluginGlobalWindow).registerKandevPlugin = (id, plugin) => {
    registeredPlugins.set(id, plugin);
  };
}

/**
 * Loads every plugin from the boot payload: injects styles, imports the
 * bundle, then runs `initialize(registry, host)`. Each plugin is isolated —
 * a failure anywhere in its load path is logged and does not affect the
 * others or the boot sequence.
 */
export async function loadPlugins(
  bootPlugins: ActivePlugin[],
  hostFactory: PluginHostFactory,
  importer: BundleImporter = defaultImporter,
  win: Window = window,
): Promise<void> {
  installPluginGlobal(win);
  for (const plugin of bootPlugins) {
    await loadPlugin(plugin, hostFactory, importer);
  }
}

async function loadPlugin(
  plugin: ActivePlugin,
  hostFactory: PluginHostFactory,
  importer: BundleImporter,
): Promise<void> {
  const { apiBaseUrl } = getBackendConfig();
  try {
    injectStyles(plugin.id, plugin.styleUrls, apiBaseUrl);
    const registered = await resolveRegistration(plugin, importer, apiBaseUrl);
    if (!registered) {
      console.error(`[plugins] "${plugin.id}" bundle did not call registerKandevPlugin`);
      return;
    }
    const host = hostFactory(plugin.id);
    const registry = pluginRegistry.forPlugin(plugin.id);
    await registered.initialize(registry, host);
  } catch (error) {
    console.error(`[plugins] failed to load plugin "${plugin.id}"`, error);
  }
}

/**
 * Returns the plugin's registration, importing the bundle only when it
 * isn't already cached from a prior load in this tab (see module doc for
 * why re-enable must not blindly re-import).
 */
async function resolveRegistration(
  plugin: ActivePlugin,
  importer: BundleImporter,
  apiBaseUrl: string,
): Promise<KandevPlugin | undefined> {
  const cached = registeredPlugins.get(plugin.id);
  if (cached) return cached;
  await importer(resolvePluginUrl(plugin.bundleUrl, apiBaseUrl));
  return registeredPlugins.get(plugin.id);
}

/**
 * Prefixes a root-relative plugin asset URL with the backend origin. Plain
 * root-relative URLs only resolve correctly when the SPA and the API share
 * an origin (same-origin production); split-origin dev and the Tauri
 * desktop shell need the explicit `apiBaseUrl`, same as `host.api.fetch`.
 */
function resolvePluginUrl(url: string, apiBaseUrl: string): string {
  if (!apiBaseUrl || !url.startsWith("/")) return url;
  return `${apiBaseUrl}${url}`;
}

function injectStyles(pluginId: string, styleUrls: string[] | undefined, apiBaseUrl: string): void {
  if (!styleUrls) return;
  for (const href of styleUrls) {
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = resolvePluginUrl(href, apiBaseUrl);
    link.dataset.pluginId = pluginId;
    document.head.appendChild(link);
  }
}

/** Removes every `<link>` this plugin injected via `injectStyles`. */
function removeStyles(pluginId: string): void {
  document.querySelectorAll(`link[data-plugin-id="${pluginId}"]`).forEach((link) => link.remove());
}

/**
 * Disables a plugin: calls `destroy?.()`, bulk-revokes its registry
 * registrations, and removes its injected stylesheets. Deliberately keeps
 * the `registeredPlugins` entry — see module doc — so a later re-enable in
 * the same tab can re-run `initialize` without depending on the browser
 * re-executing the bundle's module-eval side effect.
 */
export function unloadPlugin(id: string): void {
  const plugin = registeredPlugins.get(id);
  try {
    plugin?.destroy?.();
  } catch (error) {
    console.error(`[plugins] error destroying plugin "${id}"`, error);
  } finally {
    pluginRegistry.unregisterPlugin(id);
    removeStyles(id);
  }
}
