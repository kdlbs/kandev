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
import { pluginModalManager } from "./modal-manager";
import { pluginRegistry } from "./registry";
import type { ActivePlugin, KandevPlugin, PluginHostApi } from "./types";

/** Builds the per-plugin `PluginHostApi` for a given pluginId. */
export type PluginHostFactory = (pluginId: string) => PluginHostApi;

/** Injectable bundle loader — defaults to a real dynamic import. Tests pass a fake. */
export type BundleImporter = (url: string) => Promise<unknown>;

const defaultImporter: BundleImporter = (url) => import(/* @vite-ignore */ url);

/**
 * How long `loadPlugin` waits for a single plugin's `initialize(registry, host)`
 * to settle before giving up on it and moving on to the next plugin in the
 * boot list. A plugin whose `initialize()` never resolves must not be able to
 * stall every plugin queued behind it.
 */
const DEFAULT_INITIALIZE_TIMEOUT_MS = 10_000;

/**
 * Races `promise` against a `timeoutMs` timer. Resolves with `promise`'s
 * value (or rejects with its error) if it settles first; otherwise calls
 * `onTimeout` and resolves with `undefined` once the timer fires — a timeout
 * is deliberately not a rejection, so the caller's loop can continue to the
 * next plugin instead of routing a hang through the same error-handling path
 * as a thrown/rejected `initialize()`. The original promise is not
 * cancelled; if it eventually settles nothing observes it.
 */
function raceTimeout<T>(
  promise: Promise<T>,
  timeoutMs: number,
  onTimeout: () => void,
): Promise<T | void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      onTimeout();
      resolve();
    }, timeoutMs);
    promise.then(
      (value) => {
        clearTimeout(timer);
        resolve(value);
      },
      (error: unknown) => {
        clearTimeout(timer);
        reject(error as Error);
      },
    );
  });
}

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
 * others or the boot sequence. `initTimeoutMs` (default
 * `DEFAULT_INITIALIZE_TIMEOUT_MS`) bounds how long a single plugin's
 * `initialize()` can block the (sequential) loop before the loader gives up
 * on it and moves on — tests inject a short value instead of waiting out the
 * real default.
 */
export async function loadPlugins(
  bootPlugins: ActivePlugin[],
  hostFactory: PluginHostFactory,
  importer: BundleImporter = defaultImporter,
  win: Window = window,
  initTimeoutMs: number = DEFAULT_INITIALIZE_TIMEOUT_MS,
): Promise<void> {
  installPluginGlobal(win);
  for (const plugin of bootPlugins) {
    await loadPlugin(plugin, hostFactory, importer, initTimeoutMs);
  }
}

async function loadPlugin(
  plugin: ActivePlugin,
  hostFactory: PluginHostFactory,
  importer: BundleImporter,
  initTimeoutMs: number,
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
    const registry = pluginRegistry.forPlugin(plugin.id, plugin.name);
    await raceTimeout(Promise.resolve(registered.initialize(registry, host)), initTimeoutMs, () => {
      console.warn(
        `[plugins] "${plugin.id}" initialize() timed out after ${initTimeoutMs}ms; continuing without it`,
      );
    });
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
 * registrations, and removes its injected stylesheets. By default this
 * keeps the `registeredPlugins` entry — see module doc — so a later
 * re-enable in the same tab can re-run `initialize` without depending on the
 * browser re-executing the bundle's module-eval side effect.
 *
 * Pass `evictCache: true` for the install/update path specifically: an
 * updated package can ship new bundle code under the same `bundleUrl`, so
 * the cached registration (and, with it, the stale `initialize`/`destroy`
 * closures from the previous version) must be dropped so the next
 * `loadPlugin` re-imports it. This is opt-in — unconditional eviction here
 * would break the plain disable/re-enable cycle's cached-registration reuse.
 */
export function unloadPlugin(id: string, options?: { evictCache?: boolean }): void {
  const plugin = registeredPlugins.get(id);
  try {
    plugin?.destroy?.();
  } catch (error) {
    console.error(`[plugins] error destroying plugin "${id}"`, error);
  } finally {
    pluginRegistry.unregisterPlugin(id);
    pluginModalManager.closeAllForPlugin(id);
    removeStyles(id);
    if (options?.evictCache) {
      registeredPlugins.delete(id);
    }
  }
}
