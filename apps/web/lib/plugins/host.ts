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
 * `registeredPlugins` is retained by bundle identity across disable/re-enable
 * cycles, but a changed bundle URL always gets a fresh module registration.
 * The global callback is accepted only while the matching bundle import has an
 * open registration stage, preventing late or foreign bundles from poisoning
 * the cache.
 */
import { getBackendConfig } from "@/lib/config";
import { pluginModalManager } from "./modal-manager";
import { pluginRegistry } from "./registry";
import { generationFencedHost, PluginLoadResources } from "./host-runtime-resources";
import type { ActivePlugin, KandevPlugin, PluginHostApi, PluginRegistry } from "./types";

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
 * Races `promise` against a `timeoutMs` timer. Resolves with the settled value
 * (or rejects with its error) if it settles first; otherwise calls `onTimeout`
 * and resolves with `timedOut: true` — a timeout is deliberately not a
 * rejection, so the caller's loop can continue to the next plugin instead of
 * routing a hang through the same error-handling path as a thrown/rejected
 * `initialize()`. The original promise is not cancelled; if it eventually
 * settles nothing observes it.
 */
function raceTimeout<T>(
  promise: Promise<T>,
  timeoutMs: number,
  onTimeout: () => void,
): Promise<{ value?: T; timedOut: boolean }> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      onTimeout();
      resolve({ timedOut: true });
    }, timeoutMs);
    promise.then(
      (value) => {
        clearTimeout(timer);
        resolve({ value, timedOut: false });
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

type RegistrationCacheEntry = {
  bundleUrl: string;
  plugin: KandevPlugin;
};

const registeredPlugins = new Map<string, RegistrationCacheEntry>();

type RegistrationStage = {
  pluginId: string;
  bundleUrl: string;
  plugin?: KandevPlugin;
  open: boolean;
};

let registrationStage: RegistrationStage | undefined;
let registrationQueue = Promise.resolve();

/**
 * Latest load "generation" claimed per pluginId. `loadPlugin` claims a fresh
 * generation the moment it starts (before it awaits the dynamic import), so a
 * later-initiated load always outranks an earlier one for the same plugin.
 * Registry mutations are then fenced on this: a load that has been superseded
 * — an older boot import resolving after a newer install/update already loaded
 * (bootPlugins fires loadPlugins without awaiting it, and the settings update
 * path calls loadPlugins independently) — must not revoke the successor's
 * registrations or append its own stale ones. See `loadPlugin`.
 */
const loadGenerations = new Map<string, number>();

type ActivePluginRuntime = {
  generation: number;
  plugin: KandevPlugin;
  resources: PluginLoadResources;
};

/** Currently initialized (or initializing) runtime per plugin id. */
const activePluginRuntimes = new Map<string, ActivePluginRuntime>();

/** Generation currently published to consumers; staged replacements do not change it. */
const publishedGenerations = new Map<string, number>();

/** Claims and returns a new, strictly-increasing load generation for `id`. */
function claimLoadGeneration(id: string): number {
  const next = (loadGenerations.get(id) ?? 0) + 1;
  loadGenerations.set(id, next);
  return next;
}

/** True while `generation` is still the newest claimed load for `id`. */
function isCurrentLoad(id: string, generation: number): boolean {
  return loadGenerations.get(id) === generation;
}

function stagedGenerationRegistry(
  registry: PluginRegistry,
  isCurrent: () => boolean,
): { registry: PluginRegistry; commit: () => void } {
  const pending: Array<() => void> = [];
  const staged: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(registry)) {
    staged[key] =
      typeof value === "function"
        ? (...args: unknown[]) => {
            if (isCurrent()) {
              pending.push(() => (value as (...callArgs: unknown[]) => unknown)(...args));
            }
          }
        : value;
  }
  return {
    registry: staged as unknown as PluginRegistry,
    commit: () => pending.forEach((registration) => registration()),
  };
}

function isCurrentPublished(id: string, generation: number): boolean {
  return publishedGenerations.get(id) === generation;
}

function failPluginGeneration(
  pluginId: string,
  generation: number,
  preservePublished: boolean,
  restorePublishedMetadata?: () => void,
) {
  pluginRegistry.runAtomicMutation(() => {
    if (preservePublished) {
      restorePublishedMetadata?.();
      pluginRegistry.markPluginReady(pluginId, generation);
    } else {
      pluginRegistry.unregisterPlugin(pluginId);
      restorePublishedMetadata?.();
      pluginRegistry.markPluginFailed(pluginId, generation);
    }
  });
}

function setPluginDeclarations(plugin: ActivePlugin): void {
  if (plugin.repositoryProviderIds) {
    pluginRegistry.setDeclaredRepositoryProviderIds(plugin.id, plugin.repositoryProviderIds);
  } else {
    pluginRegistry.clearDeclaredRepositoryProviderIds(plugin.id);
  }
}

/** Defines `window.registerKandevPlugin` before any bundle loads. Idempotent. */
export function installPluginGlobal(win: Window = window): void {
  (win as PluginGlobalWindow).registerKandevPlugin = (id, plugin) => {
    const stage = registrationStage;
    if (!stage || !stage.open || stage.pluginId !== id || stage.plugin) return;
    stage.plugin = plugin;
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

// eslint-disable-next-line max-lines-per-function -- Plugin loading keeps the guarded lifecycle in one transaction.
async function loadPlugin(
  plugin: ActivePlugin,
  hostFactory: PluginHostFactory,
  importer: BundleImporter,
  initTimeoutMs: number,
): Promise<void> {
  const { apiBaseUrl } = getBackendConfig();
  const generation = claimLoadGeneration(plugin.id);
  const previousRuntime = activePluginRuntimes.get(plugin.id);
  const previousPluginName = pluginRegistry.getPluginName(plugin.id);
  const previousProviderIds = pluginRegistry.getDeclaredRepositoryProviderIds(plugin.id);
  const restorePublishedMetadata = () => {
    pluginRegistry.restorePluginName(plugin.id, previousPluginName);
    if (previousProviderIds) {
      pluginRegistry.setDeclaredRepositoryProviderIds(plugin.id, previousProviderIds);
    } else {
      pluginRegistry.clearDeclaredRepositoryProviderIds(plugin.id);
    }
  };
  pluginRegistry.markPluginLoading(plugin.id, generation);
  let generationOpen = true;
  let resources: PluginLoadResources | undefined;
  let registeredPlugin: KandevPlugin | undefined;
  const isActiveGeneration = () => generationOpen && isCurrentLoad(plugin.id, generation);
  try {
    injectStyles(plugin.id, plugin.styleUrls, apiBaseUrl, generation);
    registeredPlugin = await resolveRegistration(
      plugin,
      importer,
      apiBaseUrl,
      previousRuntime?.plugin,
    );
    if (!registeredPlugin) {
      console.error(`[plugins] "${plugin.id}" bundle did not call registerKandevPlugin`);
      generationOpen = false;
      if (isCurrentLoad(plugin.id, generation)) {
        failPluginGeneration(
          plugin.id,
          generation,
          Boolean(previousRuntime),
          restorePublishedMetadata,
        );
        revokeFailedLoad(plugin.id, generation);
      }
      return;
    }
    if (!isCurrentLoad(plugin.id, generation)) {
      generationOpen = false;
      removeStyles(plugin.id, generation);
      return;
    }
    resources = new PluginLoadResources(plugin.id);
    const host = generationFencedHost(
      hostFactory(plugin.id),
      () => isActiveGeneration() || isCurrentPublished(plugin.id, generation),
      resources,
    );
    const scopedRegistry = pluginRegistry.forPlugin(plugin.id, plugin.name);
    if (isCurrentLoad(plugin.id, generation)) {
      pluginRegistry.runAtomicMutation(() => setPluginDeclarations(plugin));
    }
    const staged = stagedGenerationRegistry(scopedRegistry, isActiveGeneration);
    const result = await raceTimeout(
      Promise.resolve(registeredPlugin.initialize(staged.registry, host)),
      initTimeoutMs,
      () => {
        console.warn(
          `[plugins] "${plugin.id}" initialize() timed out after ${initTimeoutMs}ms; continuing without it`,
        );
      },
    );
    generationOpen = false;
    if (!isCurrentLoad(plugin.id, generation)) {
      revokeFailedLoad(plugin.id, generation, resources, registeredPlugin);
      return;
    }
    if (result.timedOut) {
      failPluginGeneration(
        plugin.id,
        generation,
        Boolean(previousRuntime),
        restorePublishedMetadata,
      );
      revokeFailedLoad(plugin.id, generation, resources, registeredPlugin);
      return;
    }
    publishPluginGeneration(plugin, generation, staged, registeredPlugin, resources);
    if (previousRuntime) {
      retireRuntime(plugin.id, previousRuntime);
      removeStyles(plugin.id, previousRuntime.generation);
    }
  } catch (error) {
    generationOpen = false;
    if (isCurrentLoad(plugin.id, generation)) {
      failPluginGeneration(
        plugin.id,
        generation,
        Boolean(previousRuntime),
        restorePublishedMetadata,
      );
    }
    console.error(`[plugins] failed to load plugin "${plugin.id}"`, error);
    revokeFailedLoad(plugin.id, generation, resources, registeredPlugin);
  }
}

/**
 * Publishes one plugin generation under the registry's atomic mutation: the
 * previous registration set is revoked and the staged registrations commit as
 * one unit, so a partial failure rolls the whole generation back.
 */
function publishPluginGeneration(
  plugin: ActivePlugin,
  generation: number,
  staged: { commit: () => void },
  registeredPlugin: KandevPlugin,
  resources: PluginLoadResources,
): void {
  pluginRegistry.runAtomicMutation(() => {
    pluginRegistry.unregisterPlugin(plugin.id);
    pluginRegistry.restorePluginName(plugin.id, plugin.name);
    setPluginDeclarations(plugin);
    staged.commit();
    activePluginRuntimes.set(plugin.id, {
      generation,
      plugin: registeredPlugin,
      resources,
    });
    publishedGenerations.set(plugin.id, generation);
    pluginRegistry.markPluginReady(plugin.id, generation);
  });
}

/** Removes a failed generation without touching the published runtime. */
function revokeFailedLoad(
  pluginId: string,
  generation: number,
  resources?: PluginLoadResources,
  plugin?: KandevPlugin,
): void {
  const runtime = activePluginRuntimes.get(pluginId);
  if (runtime?.generation === generation) {
    activePluginRuntimes.delete(pluginId);
    retireRuntime(pluginId, runtime);
  } else if (resources) {
    resources.revoke();
    try {
      plugin?.destroy?.();
    } catch (error) {
      console.error(`[plugins] error destroying plugin "${pluginId}"`, error);
    }
    pluginModalManager.closeAllForPlugin(pluginId);
  }
  removeStyles(pluginId, generation);
}

function retireRuntime(pluginId: string, runtime: ActivePluginRuntime): void {
  runtime.resources.revoke();
  try {
    runtime.plugin.destroy?.();
  } catch (error) {
    console.error(`[plugins] error destroying plugin "${pluginId}"`, error);
  }
  pluginModalManager.closeAllForPlugin(pluginId);
}

/** Revokes one published runtime and its contributions. */
function deactivatePluginRuntime(pluginId: string): void {
  const runtime = activePluginRuntimes.get(pluginId);
  if (runtime) {
    activePluginRuntimes.delete(pluginId);
    publishedGenerations.delete(pluginId);
    retireRuntime(pluginId, runtime);
  }
  pluginRegistry.unregisterPlugin(pluginId);
  pluginModalManager.closeAllForPlugin(pluginId);
  removeStyles(pluginId);
}

/**
 * Returns the plugin's registration, importing the bundle only when the
 * resolved bundle URL is not already cached in this tab.
 */
async function resolveRegistration(
  plugin: ActivePlugin,
  importer: BundleImporter,
  apiBaseUrl: string,
  previousRuntimePlugin?: KandevPlugin,
): Promise<KandevPlugin | undefined> {
  const bundleUrl = resolvePluginUrl(plugin.bundleUrl, apiBaseUrl);
  const cachedRegistration = () => {
    const cached = registeredPlugins.get(plugin.id);
    if (cached?.bundleUrl !== bundleUrl) return undefined;
    return cached.plugin === previousRuntimePlugin ? { ...cached.plugin } : cached.plugin;
  };
  const cached = cachedRegistration();
  if (cached) return cached;

  const previousRegistration = registrationQueue;
  let releaseRegistration!: () => void;
  const registrationTurn = new Promise<void>((resolve) => {
    releaseRegistration = resolve;
  });
  const queuedRegistration = previousRegistration.then(() => registrationTurn);
  registrationQueue = queuedRegistration;
  try {
    await previousRegistration;
    const queuedCached = cachedRegistration();
    if (queuedCached) return queuedCached;

    const stage: RegistrationStage = { pluginId: plugin.id, bundleUrl, open: true };
    registrationStage = stage;
    try {
      await importer(bundleUrl);
      if (stage.plugin) {
        registeredPlugins.set(plugin.id, { bundleUrl, plugin: stage.plugin });
      }
      return stage.plugin;
    } finally {
      stage.open = false;
      if (registrationStage === stage) registrationStage = undefined;
    }
  } finally {
    releaseRegistration();
    if (registrationQueue === queuedRegistration) registrationQueue = Promise.resolve();
  }
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

function injectStyles(
  pluginId: string,
  styleUrls: string[] | undefined,
  apiBaseUrl: string,
  generation: number,
): void {
  if (!styleUrls) return;
  for (const href of styleUrls) {
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = resolvePluginUrl(href, apiBaseUrl);
    link.dataset.pluginId = pluginId;
    link.dataset.pluginGeneration = String(generation);
    document.head.appendChild(link);
  }
}

/** Removes one generation's styles, or every style during explicit unload. */
function removeStyles(pluginId: string, generation?: number): void {
  const selector =
    generation === undefined
      ? `link[data-plugin-id="${pluginId}"]`
      : `link[data-plugin-id="${pluginId}"][data-plugin-generation="${generation}"]`;
  document.querySelectorAll(selector).forEach((link) => link.remove());
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
export function unloadPlugin(
  id: string,
  options?: { evictCache?: boolean; transition?: "reload" | "removed" },
): void {
  const generation = claimLoadGeneration(id);
  if (options?.transition === "reload") {
    pluginRegistry.markPluginLoading(id, generation);
  } else {
    pluginRegistry.markPluginRemoved(id, generation);
  }
  // Supersede any in-flight load so a plugin whose initialize() is still
  // awaiting can't re-register after we've disabled/uninstalled it.
  deactivatePluginRuntime(id);
  if (options?.evictCache) {
    registeredPlugins.delete(id);
  }
}
