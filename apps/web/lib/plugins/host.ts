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
 */
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
  try {
    injectStyles(plugin.styleUrls);
    await importer(plugin.bundleUrl);
    const registered = registeredPlugins.get(plugin.id);
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

function injectStyles(styleUrls: string[] | undefined): void {
  if (!styleUrls) return;
  for (const href of styleUrls) {
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = href;
    document.head.appendChild(link);
  }
}

/** Disables a plugin: calls `destroy?.()` then bulk-revokes its registrations. */
export function unloadPlugin(id: string): void {
  const plugin = registeredPlugins.get(id);
  try {
    plugin?.destroy?.();
  } catch (error) {
    console.error(`[plugins] error destroying plugin "${id}"`, error);
  } finally {
    pluginRegistry.unregisterPlugin(id);
    registeredPlugins.delete(id);
  }
}
